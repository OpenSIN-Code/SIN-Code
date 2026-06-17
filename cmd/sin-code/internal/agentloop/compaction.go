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

type CompactionStats struct {
	Compactions  int64
	TokensBefore int64
	TokensAfter  int64
	LastStrategy string
}

type SummarizerFunc func(ctx context.Context, msgs []session.Message) (string, error)

type Compactor struct {
	Summarizer SummarizerFunc
	Threshold  float64

	mu   sync.Mutex
	stats CompactionStats
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
