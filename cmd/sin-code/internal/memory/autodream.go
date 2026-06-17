// SPDX-License-Identifier: MIT
// Purpose: autoDream — background memory consolidation inspired by Claude
// Code's KAIROS/autoDream system. Deduplicates near-duplicate memories,
// detects contradictions, summarizes large tag-groups, decays stale
// memories, and promotes frequently accessed ones. Runs in a background
// goroutine or as a one-shot pass. Thread-safe (mandate M7).
package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

const (
	defaultDreamInterval    = 5 * time.Minute
	defaultMaxMemories      = 1000
	dedupeThreshold         = 0.8
	contradictionThreshold  = 0.3
	decayAgeThreshold       = 30 * 24 * time.Hour
	decayFactor             = 0.9
	promoteAccessThreshold  = 3
	promoteBoost            = 0.5
	summarizeGroupThreshold = 5
)

type DreamReport struct {
	Deduped        int           `json:"deduped"`
	Contradictions int           `json:"contradictions"`
	Summarized     int           `json:"summarized"`
	Decayed        int           `json:"decayed"`
	Promoted       int           `json:"promoted"`
	Duration       time.Duration `json:"duration"`
}

type DreamStats struct {
	TotalRuns           int       `json:"total_runs"`
	LastRun             time.Time `json:"last_run"`
	TotalDeduped        int       `json:"total_deduped"`
	TotalContradictions int       `json:"total_contradictions"`
	TotalSummarized     int       `json:"total_summarized"`
}

type AutoDream struct {
	store       *Store
	interval    time.Duration
	maxMemories int
	llmClient   *llm.Client

	mu      sync.Mutex
	stats   DreamStats
	cancel  context.CancelFunc
	running bool
	wg      sync.WaitGroup
}

type AutoDreamOption func(*AutoDream)

func WithInterval(d time.Duration) AutoDreamOption {
	return func(ad *AutoDream) {
		if d > 0 {
			ad.interval = d
		}
	}
}

func WithMaxMemories(n int) AutoDreamOption {
	return func(ad *AutoDream) {
		if n > 0 {
			ad.maxMemories = n
		}
	}
}

func WithLLMClient(c *llm.Client) AutoDreamOption {
	return func(ad *AutoDream) {
		ad.llmClient = c
	}
}

func NewAutoDream(store *Store, opts ...AutoDreamOption) *AutoDream {
	ad := &AutoDream{
		store:       store,
		interval:    defaultDreamInterval,
		maxMemories: defaultMaxMemories,
	}
	for _, opt := range opts {
		opt(ad)
	}
	return ad
}

func (ad *AutoDream) Start(ctx context.Context) {
	ad.mu.Lock()
	if ad.running {
		ad.mu.Unlock()
		return
	}
	innerCtx, cancel := context.WithCancel(ctx)
	ad.cancel = cancel
	ad.running = true
	ad.mu.Unlock()

	ad.wg.Add(1)
	go func() {
		defer ad.wg.Done()
		ticker := time.NewTicker(ad.interval)
		defer ticker.Stop()
		for {
			select {
			case <-innerCtx.Done():
				return
			case <-ticker.C:
				_, _ = ad.RunOnce(innerCtx)
			}
		}
	}()
}

func (ad *AutoDream) Stop() {
	ad.mu.Lock()
	if !ad.running {
		ad.mu.Unlock()
		return
	}
	ad.running = false
	cancel := ad.cancel
	ad.cancel = nil
	ad.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	ad.wg.Wait()
}

func (ad *AutoDream) RunOnce(ctx context.Context) (*DreamReport, error) {
	if ad.store == nil {
		return nil, fmt.Errorf("autodream: nil store")
	}
	start := time.Now()
	report := &DreamReport{}

	all, err := ad.store.List(ListFilter{Limit: ad.maxMemories})
	if err != nil {
		return nil, fmt.Errorf("autodream: list memories: %w", err)
	}
	if len(all) == 0 {
		report.Duration = time.Since(start)
		ad.recordStats(report)
		return report, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	report.Deduped = ad.dedupe(ctx, all)

	all, err = ad.store.List(ListFilter{Limit: ad.maxMemories})
	if err != nil {
		return nil, fmt.Errorf("autodream: list after dedupe: %w", err)
	}
	report.Contradictions = ad.detectContradictions(ctx, all)

	all, err = ad.store.List(ListFilter{Limit: ad.maxMemories})
	if err != nil {
		return nil, fmt.Errorf("autodream: list after contradictions: %w", err)
	}
	report.Summarized = ad.summarize(ctx, all)

	all, err = ad.store.List(ListFilter{Limit: ad.maxMemories})
	if err != nil {
		return nil, fmt.Errorf("autodream: list after summarize: %w", err)
	}
	report.Decayed = ad.decay(ctx, all)
	report.Promoted = ad.promote(ctx, all)

	report.Duration = time.Since(start)
	ad.recordStats(report)
	return report, nil
}

func (ad *AutoDream) Stats() DreamStats {
	ad.mu.Lock()
	defer ad.mu.Unlock()
	return ad.stats
}

func (ad *AutoDream) recordStats(r *DreamReport) {
	ad.mu.Lock()
	ad.stats.TotalRuns++
	ad.stats.LastRun = time.Now().UTC()
	ad.stats.TotalDeduped += r.Deduped
	ad.stats.TotalContradictions += r.Contradictions
	ad.stats.TotalSummarized += r.Summarized
	ad.mu.Unlock()
}

func (ad *AutoDream) dedupe(ctx context.Context, all []*Memory) int {
	merged := 0
	deleted := map[string]bool{}
	for i := 0; i < len(all); i++ {
		if deleted[all[i].ID] {
			continue
		}
		for j := i + 1; j < len(all); j++ {
			if deleted[all[j].ID] {
				continue
			}
			if err := ctx.Err(); err != nil {
				return merged
			}
			if !sameTags(all[i].Tags, all[j].Tags) {
				continue
			}
			sim := jaccardSimilarity(all[i].Insight, all[j].Insight)
			if sim >= dedupeThreshold {
				keeper, dupe := pickKeeper(all[i], all[j])
				if keeper.Importance < dupe.Importance {
					keeper.Importance = dupe.Importance
				}
				keeper.AccessCount += dupe.AccessCount
				_ = ad.store.Update(keeper)
				_ = ad.store.Delete(dupe.ID, true)
				deleted[dupe.ID] = true
				merged++
			}
		}
	}
	return merged
}

func (ad *AutoDream) detectContradictions(ctx context.Context, all []*Memory) int {
	found := 0
	linked := map[string]bool{}
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			if err := ctx.Err(); err != nil {
				return found
			}
			if !sameTags(all[i].Tags, all[j].Tags) {
				continue
			}
			pairKey := all[i].ID + "\x00" + all[j].ID
			if linked[pairKey] {
				continue
			}
			if isContradiction(all[i].Insight, all[j].Insight) {
				_ = ad.store.AddLink(Link{
					From: all[i].ID,
					To:   all[j].ID,
					Rel:  string(LinkContradicts),
				})
				linked[pairKey] = true
				found++
			}
		}
	}
	return found
}

func (ad *AutoDream) summarize(ctx context.Context, all []*Memory) int {
	groups := groupByTag(all)
	created := 0
	for tag, mems := range groups {
		if err := ctx.Err(); err != nil {
			return created
		}
		if len(mems) < summarizeGroupThreshold {
			continue
		}
		summary := ad.buildSummary(ctx, tag, mems)
		if summary == "" {
			continue
		}
		_ = ad.store.Add(&Memory{
			Insight:    summary,
			Tags:       []string{tag, "autodream-summary"},
			Importance: 0.5,
		})
		created++
	}
	return created
}

func (ad *AutoDream) buildSummary(ctx context.Context, tag string, mems []*Memory) string {
	if ad.llmClient != nil {
		if s := ad.tryLLMSummary(ctx, tag, mems); s != "" {
			return s
		}
	}
	return deterministicSummary(tag, mems)
}

func (ad *AutoDream) tryLLMSummary(ctx context.Context, tag string, mems []*Memory) string {
	var b strings.Builder
	for _, m := range mems {
		b.WriteString("- ")
		b.WriteString(m.Insight)
		b.WriteString("\n")
	}
	prompt := fmt.Sprintf("Summarize these memories tagged '%s' into one concise insight:\n%s", tag, b.String())
	resp, err := ad.llmClient.Chat(ctx, llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: prompt}},
	})
	if err != nil || resp == nil {
		return ""
	}
	text := resp.ExtractText()
	if text == "" {
		return ""
	}
	return fmt.Sprintf("[autodream summary: %s] %s", tag, strings.TrimSpace(text))
}

func (ad *AutoDream) decay(ctx context.Context, all []*Memory) int {
	now := time.Now().UTC()
	decayed := 0
	for _, m := range all {
		if err := ctx.Err(); err != nil {
			return decayed
		}
		if now.Sub(m.Created) < decayAgeThreshold {
			continue
		}
		if !m.LastAccessed.IsZero() && now.Sub(m.LastAccessed) < decayAgeThreshold {
			continue
		}
		if m.Importance <= 0 {
			continue
		}
		m.Importance *= decayFactor
		_ = ad.store.Update(m)
		decayed++
	}
	return decayed
}

func (ad *AutoDream) promote(ctx context.Context, all []*Memory) int {
	promoted := 0
	for _, m := range all {
		if err := ctx.Err(); err != nil {
			return promoted
		}
		if m.AccessCount < promoteAccessThreshold {
			continue
		}
		m.Importance += promoteBoost
		_ = ad.store.Update(m)
		promoted++
	}
	return promoted
}

func sameTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func jaccardSimilarity(a, b string) float64 {
	setA := wordSet(a)
	setB := wordSet(b)
	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}
	if len(setA) == 0 || len(setB) == 0 {
		return 0
	}
	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,!?;:\"'()[]{}")
		if w != "" {
			set[w] = true
		}
	}
	return set
}

var negationWords = []string{"not ", "never ", "don't ", "shouldn't ", "no ", "cannot ", "can't ", "wrong", "incorrect", "false"}

func isContradiction(a, b string) bool {
	sim := jaccardSimilarity(a, b)
	if sim < contradictionThreshold {
		return false
	}
	aHasNeg := hasNegation(a)
	bHasNeg := hasNegation(b)
	return aHasNeg != bHasNeg
}

func hasNegation(s string) bool {
	lower := strings.ToLower(s)
	for _, nw := range negationWords {
		if strings.Contains(lower, nw) {
			return true
		}
	}
	return false
}

func pickKeeper(a, b *Memory) (*Memory, *Memory) {
	if a.Importance > b.Importance {
		return a, b
	}
	if b.Importance > a.Importance {
		return b, a
	}
	if a.Updated.After(b.Updated) {
		return a, b
	}
	return b, a
}

func groupByTag(all []*Memory) map[string][]*Memory {
	groups := map[string][]*Memory{}
	for _, m := range all {
		for _, t := range m.Tags {
			if t == "autodream-summary" {
				continue
			}
			groups[t] = append(groups[t], m)
		}
	}
	return groups
}

func deterministicSummary(tag string, mems []*Memory) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[autodream summary: %s] ", tag)
	for i, m := range mems {
		if i >= 10 {
			break
		}
		if i > 0 {
			b.WriteString("; ")
		}
		b.WriteString(truncate(m.Insight, 60))
	}
	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
