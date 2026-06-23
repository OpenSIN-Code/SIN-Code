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

// evidenceMarkers are the substrings a message must contain to be classified
// as verification evidence. Matches are case-sensitive (the verify subsystem
// always emits them capitalised).
var evidenceMarkers = []string{
	"VERIFICATION PASSED",
	"VERIFICATION FAILED",
	"NOT DONE",
	"Open acceptance criteria",
}

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
		Config:     DefaultCompactorConfig(),
	}
}

// Configure writes the given config into the Compactor, normalising it
// first so downstream code never sees zero-thresholds or empty modes.
// Concurrency-safe (mandate M7).
//
// When the user explicitly requests ContextCompactionOff we also reset
// the legacy Threshold field so the legacy turns-trigger heuristic in
// the loop does not fire — otherwise configuring any compactor would
// re-introduce legacy issue #278 compaction even though the user opted
// out of the new mode-based path.
func (c *Compactor) Configure(cfg CompactorConfig) {
	cfg.Normalize()
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Config = cfg
	if cfg.Mode == ContextCompactionOff {
		c.Threshold = 0
	} else if cfg.Threshold > 0 {
		c.Threshold = cfg.Threshold
	}
}

// config returns a snapshot of the current CompactorConfig. Concurrency-safe.
func (c *Compactor) config() CompactorConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Config
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
	case "off", "", "default":
		return CompactionHybrid, nil
	}
	return CompactionHybrid, fmt.Errorf("unknown compaction strategy: %q", s)
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

// ShouldCompactTokens reports whether reqTokens exceeds the effective
// context window * threshold. effective = max(1, ctxWindow), so callers
// passing 0 = "unbounded" get a sane signal only if reqTokens > 0.
func ShouldCompactTokens(reqTokens, ctxWindow int, threshold float64) bool {
	if threshold <= 0 {
		threshold = DefaultCompactionThreshold
	}
	if ctxWindow <= 0 {
		return false
	}
	return float64(reqTokens) > float64(ctxWindow)*threshold
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

// Compact is the legacy entry point retained for backward compatibility
// (issue #278). It delegates to Compact2, which honours CompactorConfig.Mode
// when set; otherwise the explicit strategy drives the result.
func (c *Compactor) Compact(ctx context.Context, messages []session.Message, strategy CompactionStrategy, maxTokens int) []session.Message {
	in := CompactInput{
		Messages:  messages,
		Strategy:  strategy,
		MaxTokens: maxTokens,
	}
	if cfg := c.config(); cfg.Mode != "" && cfg.Mode != ContextCompactionOff {
		in.Mode = cfg.Mode
	}
	r, _ := c.Compact2(ctx, in)
	return r.Kept
}

// Compact2 is the canonical entry point for context compaction modes. It
// honours the mode in input.Mode (off|deterministic|llm|hybrid). When
// input.Mode is empty (zero-value), it falls back to the legacy strategy
// path so existing call-sites remain byte-stable.
//
// The result is byte-stable per (input, c.Config) pair so the four-arm
// comparator (issue #171) can pin snapshots deterministically.
func (c *Compactor) Compact2(ctx context.Context, in CompactInput) (CompactResult, error) {
	tokensBefore := estimateTokens(in.Messages)

	res := CompactResult{
		Kept:         in.Messages,
		Dropped:      nil,
		TokensBefore: tokensBefore,
		TokensAfter:  tokensBefore,
		Mode:         in.Mode,
	}

	if len(in.Messages) == 0 {
		c.recordRun(int64(tokensBefore), int64(tokensBefore), "", in.Mode)
		return res, nil
	}

	kept, summary, dropped := c.runMode(ctx, in)

	res.Kept = kept
	res.Summary = summary
	res.Dropped = dropped
	res.TokensAfter = estimateTokens(kept)

	c.recordRun(int64(tokensBefore), int64(res.TokensAfter), lastStrategyLabel(in.Strategy, in.Mode), in.Mode)
	return res, nil
}

// runMode dispatches to the per-mode compaction routine and returns the
// kept slice + LLM summary + dropped slice. Empty Mode falls back to the
// legacy explicit-strategy path so callers that pass only a
// CompactionStrategy see identical semantics to the original issue #278
// dispatch. Explicit ContextCompactionOff is a true no-op.
func (c *Compactor) runMode(ctx context.Context, in CompactInput) ([]session.Message, string, []session.Message) {
	switch in.Mode {
	case ContextCompactionLLM:
		return c.compactLLMMode(ctx, in)
	case ContextCompactionDeterministic:
		return c.compactDeterministicMode(in)
	case ContextCompactionHybrid:
		return c.compactHybridMode(ctx, in)
	case ContextCompactionOff:
		return in.Messages, "", nil
	case "":
		kept := c.CompactStrategy(ctx, in.Messages, in.Strategy, in.MaxTokens)
		dropped := diffMessages(in.Messages, kept)
		return kept, "", dropped
	}
	kept := c.CompactStrategy(ctx, in.Messages, in.Strategy, in.MaxTokens)
	dropped := diffMessages(in.Messages, kept)
	return kept, "", dropped
}

// compactLLMMode collapses the conversation into a single system message
// containing the LLM-generated (or deterministic fallback) summary.
// Loses: ordered turns. Preserves: the summary string itself.
func (c *Compactor) compactLLMMode(ctx context.Context, in CompactInput) ([]session.Message, string, []session.Message) {
	summary := c.generateSummary(ctx, in.Messages)
	kept := []session.Message{{Role: "system", Content: summaryPrefix + summary}}
	return kept, summary, in.Messages
}

// compactDeterministicMode applies the evidence-preserving retain rules
// and drops everything that does not pass them. The output order mirrors
// the input order, so the system prompt (if present) comes first and any
// evidence messages surface in their original position.
func (c *Compactor) compactDeterministicMode(in CompactInput) ([]session.Message, string, []session.Message) {
	recent := c.config().RecentTurns
	if recent <= 0 {
		recent = 4
	}
	kept := c.retainEvidence(in.Messages, recent, in.EvidenceIndices)
	dropped := diffMessages(in.Messages, kept)
	return kept, "", dropped
}

// compactHybridMode combines evidence-preserving retain with an LLM
// summary of the *dropped* middle. The output begins with a system
// summary message followed by the evidence-retained slice verbatim.
func (c *Compactor) compactHybridMode(ctx context.Context, in CompactInput) ([]session.Message, string, []session.Message) {
	recent := c.config().RecentTurns
	if recent <= 0 {
		recent = 4
	}
	kept := c.retainEvidence(in.Messages, recent, in.EvidenceIndices)
	dropped := diffMessages(in.Messages, kept)
	summary := c.generateSummary(ctx, dropped)
	if summary == "" {
		return kept, "", dropped
	}
	out := make([]session.Message, 0, len(kept)+1)
	out = append(out, session.Message{Role: "system", Content: summaryPrefix + summary})
	out = append(out, kept...)
	return out, summary, dropped
}

// retainEvidence keeps:
//  1. The first system message (system prompt).
//  2. The first user message (the goal/problem statement).
//  3. The last `recent` turns (approximated as the last 2*recent
//     messages of any role so user/assistant/tool breadcrumbs
//     together stay attached to the model's last attention window).
//  4. Any role: tool message anywhere in history.
//  5. Any message whose Content matches one of the evidenceMarkers OR
//     whose index is present in input.EvidenceIndices.
//
// Output preserves input ordering.
func (c *Compactor) retainEvidence(msgs []session.Message, recent int, evidenceIdx map[int]bool) []session.Message {
	if len(msgs) == 0 {
		return nil
	}
	keep := make([]bool, len(msgs))

	for i, m := range msgs {
		if m.Role == "system" {
			keep[i] = true
			break
		}
	}

	for i, m := range msgs {
		if m.Role == "user" {
			keep[i] = true
			break
		}
	}

	tailStart := len(msgs) - 2*recent
	if tailStart < 0 {
		tailStart = 0
	}
	for i := tailStart; i < len(msgs); i++ {
		keep[i] = true
	}

	preserveEvidence := c.config().PreserveEvidence
	for i, m := range msgs {
		if preserveEvidence && (m.Role == "tool" || containsEvidence(m.Content)) {
			keep[i] = true
		}
	}

	for idx := range evidenceIdx {
		if idx >= 0 && idx < len(msgs) {
			keep[idx] = true
		}
	}

	out := make([]session.Message, 0, len(msgs))
	for i, m := range msgs {
		if keep[i] {
			out = append(out, m)
		}
	}
	return out
}

// containsEvidence returns true if content contains any of the
// evidenceMarkers substrings (case-sensitive substrings).
func containsEvidence(content string) bool {
	if content == "" {
		return false
	}
	for _, marker := range evidenceMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

// diffMessages returns the messages in `all` whose pointer-identity (or
// by index, since msgs is a slice of values, by content+toolcalls) is
// not present in `kept`. The comparison uses a canonical fingerprint
// because session.Message is a value-type slice element.
func diffMessages(all, kept []session.Message) []session.Message {
	keptKeys := make(map[string]struct{}, len(kept))
	for _, m := range kept {
		keptKeys[messageKey(m)] = struct{}{}
	}
	var dropped []session.Message
	for _, m := range all {
		if _, ok := keptKeys[messageKey(m)]; !ok {
			dropped = append(dropped, m)
		}
	}
	return dropped
}

// messageKey is a stable fingerprint for a session.Message used for
// dropped-set membership. Two messages with identical role+content+
// toolcalls are considered equal for diffing purposes.
func messageKey(m session.Message) string {
	return m.Role + "\x00" + m.Content + "\x00" + string(m.ToolCalls)
}

func (c *Compactor) recordRun(tokensBefore, tokensAfter int64, label string, mode ContextCompactionMode) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stats.Compactions++
	c.stats.TokensBefore += tokensBefore
	c.stats.TokensAfter += tokensAfter
	c.stats.LastStrategy = label
	c.stats.LastMode = mode.String()
}

func lastStrategyLabel(s CompactionStrategy, mode ContextCompactionMode) string {
	if mode != "" && mode != ContextCompactionOff {
		return mode.String()
	}
	return s.String()
}

func (c *Compactor) Stats() CompactionStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
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

// CompactStrategy is the explicit-strategy dispatch used when the
// compactor is invoked via the legacy Compact() path with no Mode. It
// mirrors the original switch statement byte-for-byte so existing
// callers stay stable.
func (c *Compactor) CompactStrategy(ctx context.Context, messages []session.Message, strategy CompactionStrategy, maxTokens int) []session.Message {
	if len(messages) <= 2 {
		return messages
	}

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

// Configure sets the compactor's CompactorConfig snapshot from the loop.
// When Mode is Off, Threshold is reset to 0 so the legacy turns-trigger
// never fires for opted-out callers.
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

// config returns the effective CompactorConfig (zero-value-aware).
func (c *Compactor) config() CompactorConfig {
	c.mu.Lock()
	defer c.mu.Unlock()
	cfg := c.Config
	cfg.Normalize()
	return cfg
}

// Compact2 dispatches on input.Mode:
//   - empty           → fall back to legacy Strategy dispatch
//   - off             → no-op (returns input unchanged)
//   - deterministic   → evidence-preserving selective compaction
//   - llm             → single LLM summary as system message
//   - hybrid          → LLM summary + recent turns + evidence
//
// Stats are updated under c.mu (M7). Deterministic and hybrid are
// byte-stable per (input, cfg) pair.
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

// ShouldCompactTokens is the trigger predicate for the tokens-fired path.
// Returns true when the current token estimate exceeds threshold fraction
// of ctxWin (tokens cap). threshold clamped to [0, 1].
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

// containsEvidence reports whether the message content matches at least
// one of the canonical verification-evidence markers. Matches are
// case-sensitive (the verify subsystem always emits capitalised headers).
func containsEvidence(content string) bool {
	for _, marker := range evidenceMarkers {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}

// retainEvidence builds the preservation subset: the first system
// message, every evidence-flagged message, and the last RecentTurns
// human turns. Used by deterministic and hybrid modes.
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

// evidenceIndices returns the indexes of messages that contain any of
// the canonical evidence markers.
func evidenceIndices(msgs []session.Message) []int {
	out := make([]int, 0)
	for i, m := range msgs {
		if containsEvidence(m.Content) {
			out = append(out, i)
		}
	}
	return out
}

// compactDeterministicMode applies selective compaction but ALWAYS
// retains evidence-flagged messages plus the first system prompt plus
// the trailing RecentTurns user messages (mandate M3). Dropped
// messages are NOT summarized — a deterministic mode is lossless.
func compactDeterministicMode(input CompactInput, cfg CompactorConfig) []session.Message {
	if len(input.Messages) == 0 {
		return input.Messages
	}
	retained := retainEvidence(input.Messages, cfg)
	if cfg.PreserveEvidence {
		seen := make(map[int]bool, len(retained))
		for _, m := range retained {
			for i, src := range input.Messages {
				if !seen[i] && src.Role == m.Role && src.Content == m.Content {
					seen[i] = true
				}
			}
		}
		out := make([]session.Message, 0, len(retained))
		for i, m := range input.Messages {
			if seen[i] {
				out = append(out, m)
			}
		}
		return out
	}
	out := make([]session.Message, 0, len(input.Messages))
	for _, m := range input.Messages {
		retain := false
		for _, k := range retained {
			if k.Role == m.Role && k.Content == m.Content {
				retain = true
				break
			}
		}
		retain = retain || m.Role == "tool" || len(m.ToolCalls) > 0 || m.Role == "system"
		if retain {
			out = append(out, m)
		}
	}
	return out
}

// compactLLMMode collapses every message into a single summary system
// message. The summarizer is forced (c.Summarizer); when nil, falls back
// to deterministicSummary. This mode is lossy and writes a sidecar
// snapshot when the loop calls writeCompactionSidecar.
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

// compactHybridMode produces a deterministic summary of the older
// conversations + the trailing RecentTurns verbatim + every
// evidence-flagged message. The summary is inserted as a system
// message at the head. This mode is lossy.
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
	recent_ := input.Messages[len(input.Messages)-recentCount:]
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
	out = append(out, recent_...)
	return out, summary, nil
}

// diffMessages returns the entries that exist in all but not in retain.
// Order is preserved from `all`.
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
