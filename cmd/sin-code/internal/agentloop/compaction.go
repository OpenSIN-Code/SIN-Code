// SPDX-License-Identifier: MIT
// Purpose: context compaction for the agent loop (issue #278). When the
// conversation approaches the context window limit, the loop compacts the
// message history using one of 5 strategies inspired by Claude Code's leak:
//
//  1. Summarize    — LLM-generated summary of the conversation so far
//  2. Truncate     — keep only the last N turns
//  3. Selective    — keep tool calls/results, drop prose
//  4. SlidingWindow— keep first turn (system prompt) + last N turns
//  5. Hybrid       — summarize old turns + keep recent turns verbatim
//
// Plus three Context Compaction Modes introduced by the compaction-modes PR:
//   - deterministic — evidence-preserving selective compaction
//   - llm           — single summarizer-driven system message
//   - hybrid        — evidence preservation + summarization of the middle
//
// The compactor is called proactively when len(messages) > maxTurns * 0.8,
// not reactively after hitting the limit.
//
// Thread-safe (mandate M7): all stats mutations are guarded by sync.Mutex.
package agentloop

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// evidenceMarkers are the substrings a message must contain to be
// classified as verification evidence. Matches are case-sensitive.
var evidenceMarkers = []string{
	"VERIFICATION PASSED",
	"VERIFICATION FAILED",
	"NOT DONE",
	"Open acceptance criteria",
}

type CompactionStrategy int

const (
	CompactionSummarize CompactionStrategy = iota
	CompactionTruncate
	CompactionSelective
	CompactionSlidingWindow
	CompactionHybrid
)

const (
	DefaultCompactionThreshold = 0.8
	charsPerToken              = 4
	summaryPrefix              = "[COMPACTED SUMMARY]\n"
)

type CompactionStats struct {
	Compactions  int64
	TokensBefore int64
	TokensAfter  int64
	LastStrategy string
	LastMode     string
}

type SummarizerFunc func(ctx context.Context, msgs []session.Message) (string, error)

type Compactor struct {
	Summarizer SummarizerFunc
	Threshold  float64

	mu     sync.Mutex
	stats  CompactionStats
	Config CompactorConfig
}

func NewCompactor(summarizer SummarizerFunc) *Compactor {
	return &Compactor{
		Summarizer: summarizer,
		Threshold:  DefaultCompactionThreshold,
	}
}

func DefaultCompactionStrategy() CompactionStrategy {
	return CompactionHybrid
}

func ParseCompactionStrategy(s string) (CompactionStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "summarize", "summarising", "summarizing":
		return CompactionSummarize, nil
	case "truncate":
		return CompactionTruncate, nil
	case "selective":
		return CompactionSelective, nil
	case "sliding", "slidingwindow", "sliding-window":
		return CompactionSlidingWindow, nil
	case "hybrid":
		return CompactionHybrid, nil
	default:
		return CompactionHybrid, fmt.Errorf("unknown compaction strategy: %q", s)
	}
}

func (s CompactionStrategy) String() string {
	switch s {
	case CompactionSummarize:
		return "summarize"
	case CompactionTruncate:
		return "truncate"
	case CompactionSelective:
		return "selective"
	case CompactionSlidingWindow:
		return "sliding-window"
	case CompactionHybrid:
		return "hybrid"
	default:
		return "unknown"
	}
}

func ShouldCompact(msgCount, maxTurns int, threshold float64) bool {
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	if maxTurns <= 0 {
		return false
	}
	return float64(msgCount) > float64(maxTurns)*threshold
}

func estimateTokens(msgs []session.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content) / charsPerToken
		if len(m.ToolCalls) > 0 {
			total += len(m.ToolCalls) / charsPerToken
		}
	}
	return total
}

func (c *Compactor) Compact(ctx context.Context, messages []session.Message, strategy CompactionStrategy, maxTokens int) []session.Message {
	if len(messages) <= 2 {
		return messages
	}

	tokensBefore := estimateTokens(messages)

	var result []session.Message
	switch strategy {
	case CompactionSummarize:
		result = c.compactSummarize(ctx, messages, maxTokens)
	case CompactionTruncate:
		result = compactTruncate(messages, maxTokens)
	case CompactionSelective:
		result = compactSelective(messages, maxTokens)
	case CompactionSlidingWindow:
		result = compactSlidingWindow(messages, maxTokens)
	case CompactionHybrid:
		result = c.compactHybrid(ctx, messages, maxTokens)
	default:
		result = c.compactHybrid(ctx, messages, maxTokens)
	}

	tokensAfter := estimateTokens(result)

	c.mu.Lock()
	c.stats.Compactions++
	c.stats.TokensBefore += int64(tokensBefore)
	c.stats.TokensAfter += int64(tokensAfter)
	c.stats.LastStrategy = strategy.String()
	c.mu.Unlock()

	return result
}

func (c *Compactor) Stats() CompactionStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

func (c *Compactor) Configure(cfg CompactorConfig) {
	cfg.Normalize()
	if cfg.Mode == ContextCompactionOff {
		cfg.Threshold = 0
	}
	c.mu.Lock()
	c.Config = cfg
	if cfg.Mode == ContextCompactionOff {
		c.Threshold = 0
	} else if cfg.Threshold > 0 {
		c.Threshold = cfg.Threshold
	}
	c.mu.Unlock()
}

func (c *Compactor) config() CompactorConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.Config
	cfg.Normalize()
	return cfg
}

func (c *Compactor) Compact2(ctx context.Context, input CompactInput) (CompactResult, error) {
	tokensBefore := estimateTokens(input.Messages)
	res := CompactResult{
		TokensBefore: tokensBefore,
		Mode:         input.Mode,
	}

	if input.Mode == "" {
		strat := input.Strategy
		if strat == 0 {
			strat = DefaultCompactionStrategy()
		}
		kept := c.Compact(ctx, input.Messages, strat, input.MaxTokens)
		res.Kept = kept
		res.TokensAfter = estimateTokens(kept)
		res.Dropped = diffMessages(input.Messages, kept)
		return res, nil
	}

	if input.Mode == ContextCompactionOff {
		res.Kept = input.Messages
		res.TokensAfter = tokensBefore
		res.Dropped = nil
		c.recordRun("", res.Mode.String(), tokensBefore, res.TokensAfter)
		return res, nil
	}

	cfg := c.config()
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = input.MaxTokens
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 8000
	}
	res.Mode = cfg.Mode

	switch cfg.Mode {
	case ContextCompactionDeterministic:
		res.Kept = compactDeterministicMode(input, cfg)
	case ContextCompactionLLM:
		kept, summary, err := c.compactLLMMode(ctx, input, cfg)
		if err != nil {
			return res, err
		}
		res.Kept = kept
		res.Summary = summary
	case ContextCompactionHybrid:
		kept, summary, err := c.compactHybridMode(ctx, input, cfg)
		if err != nil {
			return res, err
		}
		res.Kept = kept
		res.Summary = summary
	default:
		res.Kept = input.Messages
		res.TokensAfter = tokensBefore
		c.recordRun("", res.Mode.String(), tokensBefore, res.TokensAfter)
		return res, fmt.Errorf("agentloop: unsupported compaction mode %q", cfg.Mode)
	}

	res.TokensAfter = estimateTokens(res.Kept)
	res.Dropped = diffMessages(input.Messages, res.Kept)
	c.recordRun("", res.Mode.String(), tokensBefore, res.TokensAfter)
	return res, nil
}

func (c *Compactor) recordRun(strategyName, modeName string, tokensBefore, tokensAfter int) {
	c.mu.Lock()
	c.stats.Compactions++
	c.stats.TokensBefore += int64(tokensBefore)
	c.stats.TokensAfter += int64(tokensAfter)
	if strategyName != "" {
		c.stats.LastStrategy = strategyName
	}
	if modeName != "" {
		c.stats.LastMode = modeName
	}
	c.mu.Unlock()
}

func ShouldCompactTokens(tokens, ctxWin int, threshold float64) bool {
	if ctxWin <= 0 {
		return false
	}
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	if threshold > 1 {
		threshold = 1
	}
	return tokens >= int(float64(ctxWin)*threshold)
}

func containsEvidence(content string) bool {
	for _, marker := range evidenceMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

func (c *Compactor) compactSummarize(ctx context.Context, messages []session.Message, maxTokens int) []session.Message {
	summary := c.generateSummary(ctx, messages)
	return []session.Message{
		{Role: "system", Content: summaryPrefix + summary},
	}
}

func compactTruncate(messages []session.Message, maxTokens int) []session.Message {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	result := make([]session.Message, 0, len(messages))
	totalTokens := 0
	for i := len(messages) - 1; i >= 0; i-- {
		tokens := len(messages[i].Content) / charsPerToken
		if totalTokens+tokens > maxTokens {
			break
		}
		result = append([]session.Message{messages[i]}, result...)
		totalTokens += tokens
	}
	if len(result) == 0 && len(messages) > 0 {
		result = []session.Message{messages[len(messages)-1]}
	}
	return result
}

func compactSelective(messages []session.Message, maxTokens int) []session.Message {
	result := make([]session.Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == "system" || m.Role == "tool" || len(m.ToolCalls) > 0 {
			result = append(result, m)
			continue
		}
		if m.Role == "user" && len(m.Content) > 10000 {
			truncated := m.Content[:10000] + "\n... [truncated by selective compaction]"
			result = append(result, session.Message{Role: m.Role, Content: truncated, ToolCallID: m.ToolCallID, ToolCalls: m.ToolCalls})
			continue
		}
		if m.Role == "assistant" && len(m.Content) > 5000 {
			firstLine := strings.SplitN(m.Content, "\n", 2)[0]
			if len(firstLine) > 100 {
				firstLine = firstLine[:100]
			}
			result = append(result, session.Message{Role: m.Role, Content: firstLine + "\n... [prose dropped by selective compaction]", ToolCalls: m.ToolCalls})
			continue
		}
		result = append(result, m)
	}
	return result
}

func compactSlidingWindow(messages []session.Message, maxTokens int) []session.Message {
	if maxTokens <= 0 {
		maxTokens = 8000
	}
	if len(messages) <= 1 {
		return messages
	}

	first := messages[0]
	result := []session.Message{first}
	totalTokens := len(first.Content) / charsPerToken

	for i := len(messages) - 1; i >= 1; i-- {
		tokens := len(messages[i].Content) / charsPerToken
		if totalTokens+tokens > maxTokens {
			break
		}
		result = append([]session.Message{first}, append([]session.Message{messages[i]}, result[1:]...)...)
		totalTokens += tokens
	}

	if len(result) <= 1 && len(messages) > 1 {
		result = append(result, messages[len(messages)-1])
	}
	return result
}

func (c *Compactor) compactHybrid(ctx context.Context, messages []session.Message, maxTokens int) []session.Message {
	if maxTokens <= 0 {
		maxTokens = 8000
	}

	recentTokens := 0
	splitIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		tokens := len(messages[i].Content) / charsPerToken
		if recentTokens+tokens > maxTokens/2 {
			break
		}
		recentTokens += tokens
		splitIdx = i
	}

	if splitIdx <= 0 {
		return messages
	}

	oldMessages := messages[:splitIdx]
	recentMessages := messages[splitIdx:]

	summary := c.generateSummary(ctx, oldMessages)

	result := make([]session.Message, 0, len(recentMessages)+1)
	result = append(result, session.Message{Role: "system", Content: summaryPrefix + summary})
	result = append(result, recentMessages...)

	return result
}

func (c *Compactor) generateSummary(ctx context.Context, msgs []session.Message) string {
	if c.Summarizer != nil {
		if summary, err := c.Summarizer(ctx, msgs); err == nil && summary != "" {
			return summary
		}
	}
	return deterministicSummary(msgs)
}

func deterministicSummary(msgs []session.Message) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Summary of %d previous messages:\n", len(msgs)))
	for i, m := range msgs {
		if i >= 20 {
			b.WriteString(fmt.Sprintf("... and %d more messages\n", len(msgs)-20))
			break
		}
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		role := m.Role
		if role == "" {
			role = "unknown"
		}
		b.WriteString(fmt.Sprintf("  [%s] %s\n", role, content))
	}
	return b.String()
}

func retainEvidence(msgs []session.Message, cfg CompactorConfig) []session.Message {
	if len(msgs) == 0 {
		return nil
	}
	seen := make(map[int]bool)
	recent := cfg.RecentTurns
	if recent <= 0 {
		recent = 4
	}
	humanTurns := 0
	out := make([]session.Message, 0, len(msgs))
	if msgs[0].Role == "system" {
		out = append(out, msgs[0])
		seen[0] = true
	}
	for _, idx := range evidenceIndices(msgs) {
		if !seen[idx] {
			out = append(out, msgs[idx])
			seen[idx] = true
		}
	}
	for i := len(msgs) - 1; i >= 0 && humanTurns < recent; i-- {
		if seen[i] {
			continue
		}
		if msgs[i].Role == "user" {
			out = append(out, msgs[i])
			seen[i] = true
			humanTurns++
		}
	}
	return out
}

func evidenceIndices(msgs []session.Message) []int {
	out := make([]int, 0)
	for i, m := range msgs {
		if containsEvidence(m.Content) {
			out = append(out, i)
		}
	}
	return out
}

func compactDeterministicMode(input CompactInput, cfg CompactorConfig) []session.Message {
	if len(input.Messages) == 0 {
		return input.Messages
	}
	retained := retainEvidence(input.Messages, cfg)
	if len(retained) == 0 {
		return input.Messages
	}
	if cfg.PreserveEvidence {
		out := make([]session.Message, 0, len(input.Messages))
		for _, m := range input.Messages {
			if isInRetained(m, retained) || m.Role == "tool" || len(m.ToolCalls) > 0 || m.Role == "system" {
				out = append(out, m)
			}
		}
		return out
	}
	out := make([]session.Message, 0, len(input.Messages))
	for _, m := range input.Messages {
		if isInRetained(m, retained) {
			out = append(out, m)
		}
	}
	return out
}

func isInRetained(m session.Message, retain []session.Message) bool {
	for _, r := range retain {
		if r.Role == m.Role && r.Content == m.Content && r.ToolCallID == m.ToolCallID {
			return true
		}
	}
	return false
}

func (c *Compactor) compactLLMMode(ctx context.Context, input CompactInput, cfg CompactorConfig) ([]session.Message, string, error) {
	if len(input.Messages) == 0 {
		return input.Messages, "", nil
	}
	var summary string
	if c.Summarizer != nil {
		s, err := c.Summarizer(ctx, input.Messages)
		if err != nil {
			return nil, "", err
		}
		summary = s
	} else {
		summary = deterministicSummary(input.Messages)
	}
	if cfg.PreserveEvidence {
		for _, idx := range evidenceIndices(input.Messages) {
			summary += "\n[EVIDENCE " + input.Messages[idx].Role + "] " + input.Messages[idx].Content
		}
	}
	kept := []session.Message{{Role: "system", Content: summaryPrefix + summary}}
	return kept, summary, nil
}

func (c *Compactor) compactHybridMode(ctx context.Context, input CompactInput, cfg CompactorConfig) ([]session.Message, string, error) {
	if len(input.Messages) == 0 {
		return input.Messages, "", nil
	}
	recent := cfg.RecentTurns
	if recent <= 0 {
		recent = 4
	}
	recentCount := recent * 2
	if recentCount >= len(input.Messages) {
		summary := deterministicSummary(input.Messages)
		return []session.Message{{Role: "system", Content: summaryPrefix + summary}}, summary, nil
	}
	old := input.Messages[:len(input.Messages)-recentCount]
	recentMsgs := input.Messages[len(input.Messages)-recentCount:]
	var summary string
	if c.Summarizer != nil {
		s, err := c.Summarizer(ctx, old)
		if err != nil {
			return nil, "", err
		}
		summary = s
	} else {
		summary = deterministicSummary(old)
	}
	if cfg.PreserveEvidence {
		for _, idx := range evidenceIndices(old) {
			summary += "\n[EVIDENCE " + old[idx].Role + "] " + old[idx].Content
		}
	}
	out := make([]session.Message, 0, recentCount+1)
	out = append(out, session.Message{Role: "system", Content: summaryPrefix + summary})
	out = append(out, recentMsgs...)
	return out, summary, nil
}

func diffMessages(all, retain []session.Message) []session.Message {
	if len(all) == 0 {
		return nil
	}
	dropped := make([]session.Message, 0)
	for _, m := range all {
		found := false
		for _, r := range retain {
			if r.Role == m.Role && r.Content == m.Content && r.ToolCallID == m.ToolCallID {
				found = true
				break
			}
		}
		if !found {
			dropped = append(dropped, m)
		}
	}
	return dropped
}
