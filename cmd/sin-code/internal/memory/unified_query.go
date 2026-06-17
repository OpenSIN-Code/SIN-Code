// SPDX-License-Identifier: MIT
// Purpose: Unified memory query across all 5 stores (issue #346).
// memory.Store (bbolt), lessons.Store (SQLite), session.Store (SQLite),
// ledger.Store (SQLite), and the orchestrator episode store (SQLite/FTS5).
//
// NOTE on the episode store: the orchestrator package imports the memory
// package (nim_agent.go), so memory cannot import orchestrator without
// creating an import cycle. The episode store is therefore accepted as
// an EpisodeSearcher interface rather than *orchestrator.EpisodeStore.
package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type StoreLabel string

const (
	StoreMemory   StoreLabel = "memory"
	StoreLessons  StoreLabel = "lessons"
	StoreSessions StoreLabel = "sessions"
	StoreLedger   StoreLabel = "ledger"
	StoreEpisodes StoreLabel = "episodes"
)

type UnifiedResult struct {
	Store     StoreLabel `json:"store"`
	ID        string     `json:"id"`
	Content   string     `json:"content"`
	Score     float64    `json:"score"`
	Timestamp time.Time  `json:"timestamp"`
}

type EpisodeHit struct {
	ID        int64
	TaskTitle string
	Score     float64
	Passed    bool
	CreatedAt time.Time
}

type EpisodeSearcher interface {
	Similar(ctx context.Context, taskTitle string, k int) ([]EpisodeHit, error)
}

type memorySearcher interface {
	Search(query string, project string, limit int) ([]ScoredMemory, error)
}

type lessonsSearcher interface {
	Query(ctx context.Context, workspace string, limit int) ([]lessons.Entry, error)
}

type sessionSearcher interface {
	List() ([]session.Info, error)
}

type ledgerSearcher interface {
	Sessions(ctx context.Context, limit int) ([]string, error)
	List(ctx context.Context, sessionID string, limit int) ([]ledger.Entry, error)
}

type UnifiedStore struct {
	mem      memorySearcher
	lessons  lessonsSearcher
	sess     sessionSearcher
	ledger   ledgerSearcher
	episodes EpisodeSearcher
	mu       sync.RWMutex
}

func NewUnifiedStore(
	mem *Store,
	lessonsStore *lessons.Store,
	sess *session.Store,
	ledgerStore *ledger.Store,
	episodes EpisodeSearcher,
) *UnifiedStore {
	u := &UnifiedStore{}
	if mem != nil {
		u.mem = mem
	}
	if lessonsStore != nil {
		u.lessons = lessonsStore
	}
	if sess != nil {
		u.sess = sess
	}
	if ledgerStore != nil {
		u.ledger = ledgerStore
	}
	if episodes != nil {
		u.episodes = episodes
	}
	return u
}

func (u *UnifiedStore) HasStore(label StoreLabel) bool {
	if u == nil {
		return false
	}
	u.mu.RLock()
	defer u.mu.RUnlock()
	switch label {
	case StoreMemory:
		return u.mem != nil
	case StoreLessons:
		return u.lessons != nil
	case StoreSessions:
		return u.sess != nil
	case StoreLedger:
		return u.ledger != nil
	case StoreEpisodes:
		return u.episodes != nil
	}
	return false
}

func (u *UnifiedStore) Query(ctx context.Context, query string, topK int) ([]UnifiedResult, error) {
	if u == nil {
		return nil, nil
	}
	if topK <= 0 {
		topK = 10
	}
	type storeResult struct {
		results []UnifiedResult
		err     error
	}
	ch := make(chan storeResult, 5)
	u.mu.RLock()
	mem, ls, sess, ldg, eps := u.mem, u.lessons, u.sess, u.ledger, u.episodes
	u.mu.RUnlock()

	go func() {
		if mem == nil {
			ch <- storeResult{}
			return
		}
		scored, err := mem.Search(query, "", topK*2)
		if err != nil {
			ch <- storeResult{err: err}
			return
		}
		out := make([]UnifiedResult, 0, len(scored))
		for _, s := range scored {
			score := s.Score
			if score <= 0 {
				score = 0.1
			}
			out = append(out, UnifiedResult{
				Store: StoreMemory, ID: s.ID, Content: s.Insight,
				Score:     clamp01(score)*0.6 + recencyScore(s.Updated)*0.3 + importanceScore(s.Importance)*0.1,
				Timestamp: s.Updated,
			})
		}
		ch <- storeResult{results: out}
	}()

	go func() {
		if ls == nil {
			ch <- storeResult{}
			return
		}
		entries, err := ls.Query(ctx, "*", topK*2)
		if err != nil {
			ch <- storeResult{err: err}
			return
		}
		out := make([]UnifiedResult, 0, len(entries))
		for _, e := range entries {
			out = append(out, UnifiedResult{
				Store: StoreLessons, ID: e.ID, Content: e.Lesson,
				Score:     substringScore(query, e.Lesson)*0.5 + recencyScore(e.LastSeen)*0.3 + occurrenceScore(float64(e.Occurrences))*0.2,
				Timestamp: e.LastSeen,
			})
		}
		ch <- storeResult{results: out}
	}()

	go func() {
		if sess == nil {
			ch <- storeResult{}
			return
		}
		infos, err := sess.List()
		if err != nil {
			ch <- storeResult{err: err}
			return
		}
		out := make([]UnifiedResult, 0, len(infos))
		for _, info := range infos {
			rel := substringScore(query, info.Title)
			if rel == 0 && query != "" {
				continue
			}
			ts := parseTime(info.UpdatedAt)
			out = append(out, UnifiedResult{
				Store: StoreSessions, ID: info.ID, Content: info.Title,
				Score: rel*0.6 + recencyScore(ts)*0.4, Timestamp: ts,
			})
		}
		ch <- storeResult{results: out}
	}()

	go func() {
		if ldg == nil {
			ch <- storeResult{}
			return
		}
		sids, err := ldg.Sessions(ctx, topK*4)
		if err != nil {
			ch <- storeResult{err: err}
			return
		}
		out := make([]UnifiedResult, 0, len(sids))
		for _, sid := range sids {
			entries, err := ldg.List(ctx, sid, topK*2)
			if err != nil {
				continue
			}
			for _, e := range entries {
				rel := substringScore(query, e.Summary)
				if rel == 0 && query != "" {
					continue
				}
				out = append(out, UnifiedResult{
					Store: StoreLedger, ID: e.ID, Content: e.Summary,
					Score: rel*0.5 + recencyScore(e.CreatedAt)*0.3 + 0.2, Timestamp: e.CreatedAt,
				})
			}
		}
		ch <- storeResult{results: out}
	}()

	go func() {
		if eps == nil {
			ch <- storeResult{}
			return
		}
		hits, err := eps.Similar(ctx, query, topK*2)
		if err != nil {
			ch <- storeResult{err: err}
			return
		}
		out := make([]UnifiedResult, 0, len(hits))
		for _, ep := range hits {
			rel := ep.Score
			if rel <= 0 {
				rel = substringScore(query, ep.TaskTitle)
			}
			boost := 0.0
			if ep.Passed {
				boost = 0.1
			}
			out = append(out, UnifiedResult{
				Store: StoreEpisodes, ID: fmt.Sprintf("ep-%d", ep.ID), Content: ep.TaskTitle,
				Score: clamp01(rel)*0.5 + recencyScore(ep.CreatedAt)*0.3 + boost, Timestamp: ep.CreatedAt,
			})
		}
		ch <- storeResult{results: out}
	}()

	var all []UnifiedResult
	var errs []error
	for i := 0; i < 5; i++ {
		r := <-ch
		if r.err != nil {
			errs = append(errs, r.err)
		}
		all = append(all, r.results...)
	}
	all = dedupeByContent(all)
	sort.Slice(all, func(i, j int) bool {
		if all[i].Score != all[j].Score {
			return all[i].Score > all[j].Score
		}
		return all[i].Timestamp.After(all[j].Timestamp)
	})
	if len(all) > topK {
		all = all[:topK]
	}
	if len(all) == 0 && len(errs) > 0 {
		msgs := make([]string, len(errs))
		for i, e := range errs {
			msgs[i] = e.Error()
		}
		return nil, fmt.Errorf("unified query: all stores failed: %s", strings.Join(msgs, "; "))
	}
	return all, nil
}

func dedupeByContent(in []UnifiedResult) []UnifiedResult {
	seen := map[string]int{}
	out := make([]UnifiedResult, 0, len(in))
	for _, r := range in {
		h := contentHash(r.Content)
		if idx, ok := seen[h]; ok {
			if r.Score > out[idx].Score {
				out[idx] = r
			}
			continue
		}
		seen[h] = len(out)
		out = append(out, r)
	}
	return out
}

func contentHash(s string) string {
	h := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(s))))
	return hex.EncodeToString(h[:8])
}

func substringScore(needle, haystack string) float64 {
	if needle == "" {
		return 0.5
	}
	if strings.Contains(strings.ToLower(haystack), strings.ToLower(needle)) {
		return 1.0
	}
	nw := strings.Fields(strings.ToLower(needle))
	hw := strings.ToLower(haystack)
	hits := 0
	for _, w := range nw {
		if strings.Contains(hw, w) {
			hits++
		}
	}
	if len(nw) > 0 && hits > 0 {
		return float64(hits) / float64(len(nw))
	}
	return 0.0
}

func recencyScore(t time.Time) float64 {
	if t.IsZero() {
		return 0.0
	}
	age := time.Since(t)
	if age <= 0 {
		return 1.0
	}
	const halfLife = 7 * 24 * time.Hour
	return math.Exp(-float64(age) / float64(halfLife))
}

func importanceScore(imp float64) float64 {
	if imp <= 0 {
		return 0.0
	}
	if imp > 1 {
		return 1.0
	}
	return imp
}

func occurrenceScore(occ float64) float64 {
	if occ <= 0 {
		return 0.0
	}
	return 1.0 - 1.0/(1.0+occ/5.0)
}

func clamp01(x float64) float64 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

func parseTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
