// SPDX-License-Identifier: MIT
// Purpose: tests for context compaction strategies (issue #278, M7).
package agentloop

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func makeTestMessages(n int) []session.Message {
	msgs := make([]session.Message, n)
	for i := range msgs {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		msgs[i] = session.Message{
			Role:    role,
			Content: fmt.Sprintf("message %d: %s", i, strings.Repeat("x", 1000)),
		}
	}
	return msgs
}

func makeToolMessages(n int) []session.Message {
	msgs := make([]session.Message, n)
	for i := range msgs {
		switch i % 4 {
		case 0:
			msgs[i] = session.Message{Role: "user", Content: fmt.Sprintf("user msg %d", i)}
		case 1:
			msgs[i] = session.Message{Role: "assistant", Content: "", ToolCalls: []byte(`[{"id":"tc_1","type":"function","function":{"name":"sin_read","arguments":"{}"}}]`)}
		case 2:
			msgs[i] = session.Message{Role: "tool", Content: "tool output " + strings.Repeat("y", 500), ToolCallID: "tc_1"}
		case 3:
			msgs[i] = session.Message{Role: "assistant", Content: "assistant prose " + strings.Repeat("z", 6000)}
		}
	}
	return msgs
}

func TestCompactionSummarize(t *testing.T) {
	c := NewCompactor(func(ctx context.Context, msgs []session.Message) (string, error) {
		return "LLM summary of conversation", nil
	})
	msgs := makeTestMessages(20)
	result := c.Compact(context.Background(), msgs, CompactionSummarize, 4000)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if !strings.Contains(result[0].Content, "LLM summary") {
		t.Errorf("summary content: %s", result[0].Content)
	}
	if result[0].Role != "system" {
		t.Errorf("expected system role, got %s", result[0].Role)
	}
}

func TestCompactionSummarizeFallback(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(10)
	result := c.Compact(context.Background(), msgs, CompactionSummarize, 4000)

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
	if !strings.Contains(result[0].Content, "Summary of 10") {
		t.Errorf("fallback summary should mention message count: %s", result[0].Content)
	}
}

func TestCompactionTruncate(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(20)
	result := c.Compact(context.Background(), msgs, CompactionTruncate, 3000)

	if len(result) >= len(msgs) {
		t.Fatalf("truncate should reduce message count: got %d from %d", len(result), len(msgs))
	}
	if len(result) == 0 {
		t.Fatal("truncate should keep at least one message")
	}
	lastOriginal := msgs[len(msgs)-1].Content
	if result[len(result)-1].Content != lastOriginal {
		t.Error("truncate should keep the most recent messages")
	}
}

func TestCompactionSelective(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeToolMessages(16)
	result := c.Compact(context.Background(), msgs, CompactionSelective, 0)

	hasTool := false
	for _, m := range result {
		if m.Role == "tool" {
			hasTool = true
		}
	}
	if !hasTool {
		t.Error("selective compaction should preserve tool messages")
	}

	for _, m := range result {
		if m.Role == "assistant" && len(m.Content) > 5000 && len(m.ToolCalls) == 0 {
			t.Error("selective compaction should drop long assistant prose")
		}
	}
}

func TestCompactionSlidingWindow(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(20)
	msgs[0].Role = "system"
	msgs[0].Content = "SYSTEM PROMPT"
	result := c.Compact(context.Background(), msgs, CompactionSlidingWindow, 3000)

	if len(result) < 1 {
		t.Fatal("sliding window should keep at least the system message")
	}
	if result[0].Content != "SYSTEM PROMPT" {
		t.Errorf("sliding window should preserve first (system) message: got %s", result[0].Content)
	}
	if len(result) >= len(msgs) {
		t.Fatalf("sliding window should reduce total messages: got %d from %d", len(result), len(msgs))
	}
}

func TestCompactionHybrid(t *testing.T) {
	c := NewCompactor(func(ctx context.Context, msgs []session.Message) (string, error) {
		return "hybrid summary of old messages", nil
	})
	msgs := makeTestMessages(20)
	msgs[0].Role = "system"
	result := c.Compact(context.Background(), msgs, CompactionHybrid, 3000)

	if len(result) == 0 {
		t.Fatal("hybrid should return at least one message")
	}
	if len(result) >= len(msgs) {
		t.Fatalf("hybrid should reduce message count: got %d from %d", len(result), len(msgs))
	}
	hasSummary := false
	for _, m := range result {
		if strings.Contains(m.Content, "hybrid summary") {
			hasSummary = true
		}
	}
	if !hasSummary {
		t.Error("hybrid should contain a summary of old messages")
	}
}

func TestCompactionHybridFallback(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(30)
	result := c.Compact(context.Background(), msgs, CompactionHybrid, 1000)

	if len(result) == 0 {
		t.Fatal("hybrid fallback should return messages")
	}
	if len(result) >= len(msgs) {
		t.Fatalf("hybrid fallback should reduce: got %d from %d", len(result), len(msgs))
	}
}

func TestCompactionEmptyMessages(t *testing.T) {
	c := NewCompactor(nil)
	result := c.Compact(context.Background(), nil, CompactionHybrid, 4000)
	if len(result) != 0 {
		t.Errorf("empty input should return empty, got %d", len(result))
	}
}

func TestCompactionSingleMessage(t *testing.T) {
	c := NewCompactor(nil)
	msgs := []session.Message{{Role: "user", Content: "hello"}}
	result := c.Compact(context.Background(), msgs, CompactionHybrid, 4000)
	if len(result) != 1 {
		t.Errorf("single message should return as-is, got %d", len(result))
	}
}

func TestCompactionTwoMessages(t *testing.T) {
	c := NewCompactor(nil)
	msgs := []session.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
	}
	result := c.Compact(context.Background(), msgs, CompactionHybrid, 4000)
	if len(result) != 2 {
		t.Errorf("two messages should return as-is, got %d", len(result))
	}
}

func TestCompactionStatsTracking(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(20)

	c.Compact(context.Background(), msgs, CompactionTruncate, 500)
	c.Compact(context.Background(), msgs, CompactionSelective, 0)

	stats := c.Stats()
	if stats.Compactions != 2 {
		t.Errorf("expected 2 compactions, got %d", stats.Compactions)
	}
	if stats.TokensBefore == 0 {
		t.Error("expected non-zero tokensBefore")
	}
	if stats.LastStrategy != "selective" {
		t.Errorf("expected last strategy 'selective', got %s", stats.LastStrategy)
	}
}

func TestCompactionConcurrentRaceFree(t *testing.T) {
	c := NewCompactor(nil)
	msgs := makeTestMessages(20)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			strategy := CompactionStrategy(n % 5)
			c.Compact(context.Background(), msgs, strategy, 2000)
			_ = c.Stats()
		}(i)
	}
	wg.Wait()

	stats := c.Stats()
	if stats.Compactions != 50 {
		t.Errorf("expected 50 compactions, got %d", stats.Compactions)
	}
}

func TestParseCompactionStrategy(t *testing.T) {
	cases := []struct {
		input string
		want  CompactionStrategy
	}{
		{"summarize", CompactionSummarize},
		{"truncate", CompactionTruncate},
		{"selective", CompactionSelective},
		{"sliding", CompactionSlidingWindow},
		{"sliding-window", CompactionSlidingWindow},
		{"hybrid", CompactionHybrid},
		{"HYBRID", CompactionHybrid},
	}
	for _, tc := range cases {
		got, err := ParseCompactionStrategy(tc.input)
		if err != nil {
			t.Errorf("parse %q: %v", tc.input, err)
		}
		if got != tc.want {
			t.Errorf("parse %q: got %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestParseCompactionStrategyUnknown(t *testing.T) {
	_, err := ParseCompactionStrategy("nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown strategy")
	}
}

func TestCompactionStrategyString(t *testing.T) {
	cases := []struct {
		strat CompactionStrategy
		want  string
	}{
		{CompactionSummarize, "summarize"},
		{CompactionTruncate, "truncate"},
		{CompactionSelective, "selective"},
		{CompactionSlidingWindow, "sliding-window"},
		{CompactionHybrid, "hybrid"},
	}
	for _, tc := range cases {
		if got := tc.strat.String(); got != tc.want {
			t.Errorf("strategy String(): got %q, want %q", got, tc.want)
		}
	}
}

func TestDefaultCompactionStrategy(t *testing.T) {
	if got := DefaultCompactionStrategy(); got != CompactionHybrid {
		t.Errorf("default strategy: got %v, want %v", got, CompactionHybrid)
	}
}

func TestShouldCompact(t *testing.T) {
	cases := []struct {
		msgCount  int
		maxTurns  int
		threshold float64
		want      bool
	}{
		{10, 10, 0.8, true},
		{8, 10, 0.8, false},
		{7, 10, 0.8, false},
		{9, 10, 0.8, true},
		{100, 80, 0.8, true},
		{50, 80, 0.8, false},
	}
	for _, tc := range cases {
		got := ShouldCompact(tc.msgCount, tc.maxTurns, tc.threshold)
		if got != tc.want {
			t.Errorf("ShouldCompact(%d, %d, %.1f): got %v, want %v",
				tc.msgCount, tc.maxTurns, tc.threshold, got, tc.want)
		}
	}
}

func TestShouldCompactDefaultThreshold(t *testing.T) {
	if !ShouldCompact(17, 20, 0) {
		t.Error("ShouldCompact with threshold=0 should use default 0.8")
	}
}

func TestShouldCompactZeroMaxTurns(t *testing.T) {
	if ShouldCompact(100, 0, 0.8) {
		t.Error("ShouldCompact with maxTurns=0 should return false")
	}
}

func TestDeterministicSummary(t *testing.T) {
	msgs := makeTestMessages(25)
	summary := deterministicSummary(msgs)
	if !strings.Contains(summary, "Summary of 25") {
		t.Errorf("summary should mention count: %s", summary)
	}
	if !strings.Contains(summary, "and 5 more messages") {
		t.Errorf("summary should truncate at 20: %s", summary)
	}
}

func TestCompactionSelectivePreservesSystemPrompt(t *testing.T) {
	c := NewCompactor(nil)
	msgs := []session.Message{
		{Role: "system", Content: "IMPORTANT SYSTEM PROMPT"},
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
		{Role: "tool", Content: "tool result", ToolCallID: "tc1"},
	}
	result := c.Compact(context.Background(), msgs, CompactionSelective, 0)
	if result[0].Content != "IMPORTANT SYSTEM PROMPT" {
		t.Errorf("selective should preserve system prompt: got %s", result[0].Content)
	}
}

// Context Compaction Modes (first PR — compaction-modes) coverage.

func stubSummarizerCM(tag string) SummarizerFunc {
	return func(ctx context.Context, msgs []session.Message) (string, error) {
		return tag + "(" + strconv.Itoa(len(msgs)) + ")", nil
	}
}

func TestCompact2_Off_NoOp(t *testing.T) {
	c := NewCompactor(stubSummarizerCM("LLM"))
	msgs := []session.Message{{Role: "system", Content: "sys"}, {Role: "user", Content: "u1"}, {Role: "assistant", Content: "a1"}}
	r, _ := c.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: ContextCompactionOff, MaxTokens: 100})
	if len(r.Kept) != len(msgs) {
		t.Errorf("off no-op; got %d want %d", len(r.Kept), len(msgs))
	}
}

func TestCompact2_Deterministic_EvidencePreserved(t *testing.T) {
	c := NewCompactor(stubSummarizerCM("LLM"))
	c.Configure(CompactorConfig{Mode: ContextCompactionDeterministic, Trigger: CompactionTriggerTokens, Threshold: 0.8, MaxTokens: 8000, PreserveEvidence: true, RecentTurns: 4})
	msgs := make([]session.Message, 30)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: "noise"}
		if i%2 == 1 {
			msgs[i].Role = "assistant"
		}
	}
	msgs[0] = session.Message{Role: "system", Content: "SYSTEM"}
	msgs[15] = session.Message{Role: "assistant", Content: "VERIFY: VERIFICATION PASSED"}
	r, _ := c.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: ContextCompactionDeterministic, MaxTokens: 8000})
	if len(r.Kept) >= len(msgs) {
		t.Errorf("deterministic reduce; got %d from %d", len(r.Kept), len(msgs))
	}
	found := false
	for _, m := range r.Kept {
		if m.Content == "VERIFY: VERIFICATION PASSED" {
			found = true
		}
	}
	if !found {
		t.Error("missing VERIFICATION PASSED")
	}
	if r.Summary != "" {
		t.Errorf("deterministic summary non-empty: %q", r.Summary)
	}
}

func TestCompact2_LLM_SingleSummary(t *testing.T) {
	c := NewCompactor(stubSummarizerCM("LLM-SUMMARY"))
	c.Configure(CompactorConfig{Mode: ContextCompactionLLM, MaxTokens: 8000, PreserveEvidence: true, RecentTurns: 4})
	msgs := []session.Message{{Role: "system", Content: "S"}, {Role: "user", Content: "g"}, {Role: "assistant", Content: "p"}}
	r, _ := c.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: ContextCompactionLLM, MaxTokens: 8000})
	if len(r.Kept) != 1 || r.Kept[0].Role != "system" {
		t.Error("llm single system message expected")
	}
	if !strings.Contains(r.Summary, "LLM-SUMMARY") {
		t.Errorf("summary missing tag: %q", r.Summary)
	}
	if !r.Mode.IsLossy() {
		t.Error("llm should be lossy")
	}
}

func TestCompact2_Hybrid_EvidencePlusSummary(t *testing.T) {
	c := NewCompactor(stubSummarizerCM("LLM-HYBRID"))
	c.Configure(CompactorConfig{Mode: ContextCompactionHybrid, RecentTurns: 2, PreserveEvidence: true, MaxTokens: 8000})
	msgs := make([]session.Message, 20)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: "m" + strconv.Itoa(i)}
		if i%2 == 1 {
			msgs[i].Role = "assistant"
		}
	}
	msgs[0].Role = "system"
	msgs[15].Content = "VERIFICATION PASSED"
	r, _ := c.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: ContextCompactionHybrid, MaxTokens: 8000})
	if !strings.Contains(r.Summary, "LLM-HYBRID") {
		t.Errorf("hybrid summary missing tag: %q", r.Summary)
	}
	if len(r.Kept) == 0 || r.Kept[0].Role != "system" {
		t.Error("hybrid Kept[0] should be system")
	}
}

func TestCompact2_LegacyStrategy_Unchanged(t *testing.T) {
	c := NewCompactor(stubSummarizerCM("LLM"))
	msgs := makeTestMessages(20)
	r, _ := c.Compact2(context.Background(), CompactInput{Messages: msgs, Strategy: CompactionTruncate, MaxTokens: 3000})
	if len(r.Kept) >= len(msgs) {
		t.Error("legacy truncate should reduce")
	}
}

func TestCompact2_ByteStableDeterministic(t *testing.T) {
	cfg := CompactorConfig{Mode: ContextCompactionDeterministic, RecentTurns: 4, PreserveEvidence: true, MaxTokens: 8000}
	msgs := []session.Message{{Role: "system", Content: "S"}, {Role: "user", Content: "U1"}, {Role: "assistant", Content: "A1"}, {Role: "tool", Content: "T1", ToolCallID: "tc1"}, {Role: "assistant", Content: "VERIFICATION PASSED"}}
	c1 := NewCompactor(stubSummarizerCM("FIXED"))
	c1.Configure(cfg)
	c2 := NewCompactor(stubSummarizerCM("FIXED"))
	c2.Configure(cfg)
	r1, _ := c1.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: cfg.Mode, MaxTokens: cfg.MaxTokens})
	r2, _ := c2.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: cfg.Mode, MaxTokens: cfg.MaxTokens})
	if len(r1.Kept) != len(r2.Kept) {
		t.Errorf("byte-stable r1=%d r2=%d", len(r1.Kept), len(r2.Kept))
	}
	for i := range r1.Kept {
		if r1.Kept[i].Role != r2.Kept[i].Role || r1.Kept[i].Content != r2.Kept[i].Content {
			t.Errorf("Kept[%d] differs", i)
		}
	}
}

func TestIdentifyEvidence_FindsMarkers(t *testing.T) {
	msgs := []session.Message{{Role: "system", Content: "s"}, {Role: "user", Content: "u"}, {Role: "assistant", Content: "VERIFICATION PASSED"}, {Role: "user", Content: "m"}, {Role: "assistant", Content: "Open acceptance criteria: yes"}}
	idx := evidenceIndices(msgs)
	if len(idx) != 2 {
		t.Errorf("evidenceIndices size: got %d want 2 (idx=%v)", len(idx), idx)
	}
}
