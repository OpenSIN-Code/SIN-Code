// SPDX-License-Identifier: MIT
// Purpose: tests for context compaction strategies (issue #278, M7) AND
// for the compaction-modes evidence-preserving layer (first PR).
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

// ──────────────────────────────────────────────────────────────────────────
// Context Compaction Modes (first PR) coverage.
// ──────────────────────────────────────────────────────────────────────────

func stubSummarizer(tag string) SummarizerFunc {
	return func(ctx context.Context, msgs []session.Message) (string, error) {
		return tag + "(" + strconv.Itoa(len(msgs)) + ")", nil
	}
}

func TestCompact2_Off_NoOp(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM"))
	msgs := []session.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u1"},
		{Role: "assistant", Content: "a1"},
	}
	r, err := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Mode:      ContextCompactionOff,
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("Compact2: %v", err)
	}
	if len(r.Kept) != len(msgs) {
		t.Errorf("off-mode should be a no-op; got Kept=%d want %d", len(r.Kept), len(msgs))
	}
	if r.TokensBefore != r.TokensAfter {
		t.Errorf("off-mode should not change token count: before=%d after=%d", r.TokensBefore, r.TokensAfter)
	}
}

func TestCompact2_Deterministic_EvidencePreserved(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM"))
	c.Configure(CompactorConfig{
		Mode:             ContextCompactionDeterministic,
		Trigger:          CompactionTriggerTokens,
		Threshold:        0.8,
		MaxTokens:        8000,
		PreserveEvidence: true,
		RecentTurns:      4,
	})

	msgs := make([]session.Message, 30)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: "noise"}
		if i%2 == 1 {
			msgs[i].Role = "assistant"
		}
	}
	msgs[0] = session.Message{Role: "system", Content: "SYSTEM-PROMPT"}
	msgs[1] = session.Message{Role: "user", Content: "fix the login flow"}
	msgs[10] = session.Message{Role: "tool", Content: "<<file_dump>>", ToolCallID: "tc1"}
	msgs[15] = session.Message{Role: "assistant", Content: "found the bug — VERIFICATION PASSED"}
	r, err := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Mode:      ContextCompactionDeterministic,
		MaxTokens: 8000,
	})
	if err != nil {
		t.Fatalf("Compact2: %v", err)
	}
	if len(r.Kept) >= len(msgs) {
		t.Errorf("deterministic mode should drop un-flagged messages; got len=%d input=%d", len(r.Kept), len(msgs))
	}
	for _, idx := range []int{0, 1, 10, 15} {
		if !containsMessage(r.Kept, msgs[idx]) {
			t.Errorf("evidence-preservation drop: missing message idx %d (content=%q)", idx, msgs[idx].Content)
		}
	}
	if r.Summary != "" {
		t.Errorf("deterministic mode should NOT produce an LLM summary; got %q", r.Summary)
	}
}

func TestCompact2_LLM_SingleSummary(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM-SUMMARY"))
	c.Configure(CompactorConfig{
		Mode:             ContextCompactionLLM,
		MaxTokens:        8000,
		PreserveEvidence: true,
		RecentTurns:      4,
	})

	msgs := []session.Message{
		{Role: "system", Content: "SYSTEM"},
		{Role: "user", Content: "the goal"},
		{Role: "assistant", Content: "assistant prose"},
		{Role: "tool", Content: "tool result"},
	}
	r, err := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Mode:      ContextCompactionLLM,
		MaxTokens: 8000,
	})
	if err != nil {
		t.Fatalf("Compact2: %v", err)
	}
	if len(r.Kept) != 1 {
		t.Errorf("llm mode should collapse to single system message; got len=%d", len(r.Kept))
	}
	if r.Kept[0].Role != "system" {
		t.Errorf("llm mode Kept should be system role; got %q", r.Kept[0].Role)
	}
	if !strings.Contains(r.Summary, "LLM-SUMMARY") {
		t.Errorf("llm mode Summary should contain stub tag; got %q", r.Summary)
	}
	if !r.Mode.IsLossy() {
		t.Errorf("llm mode should be lossy; Mode=%v IsLossy=%v", r.Mode, r.Mode.IsLossy())
	}
}

func TestCompact2_Hybrid_EvidencePlusSummary(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM-HYBRID"))
	c.Configure(CompactorConfig{
		Mode:             ContextCompactionHybrid,
		RecentTurns:      4,
		PreserveEvidence: true,
		MaxTokens:        8000,
	})

	msgs := []session.Message{
		{Role: "system", Content: "SYSTEM"},
		{Role: "user", Content: "fix bug"},
		{Role: "assistant", Content: "checking"},
		{Role: "user", Content: "any progress?"},
		{Role: "assistant", Content: "VERIFICATION FAILED on first attempt"},
		{Role: "assistant", Content: "retrying"},
		{Role: "tool", Content: "tool"},
		{Role: "assistant", Content: "VERIFICATION PASSED"},
	}
	r, err := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Mode:      ContextCompactionHybrid,
		MaxTokens: 8000,
	})
	if err != nil {
		t.Fatalf("Compact2: %v", err)
	}
	if !strings.Contains(r.Summary, "LLM-HYBRID") {
		t.Errorf("hybrid Summary should contain stub tag; got %q", r.Summary)
	}
	if len(r.Kept) == 0 {
		t.Fatal("hybrid should retain at least the summary message")
	}
	if r.Kept[0].Role != "system" {
		t.Errorf("hybrid Kept[0] should be the summary system message; got %q", r.Kept[0].Role)
	}
	for _, idx := range []int{0, 1, 7} {
		if !containsMessage(r.Kept, msgs[idx]) {
			t.Errorf("hybrid evidence-preservation drop: missing message idx %d", idx)
		}
	}
}

func TestCompact2_LegacyStrategy_Unchanged(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM"))
	msgs := makeTestMessages(20)
	r, err := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Strategy:  CompactionTruncate,
		Mode:      "",
		MaxTokens: 3000,
	})
	if err != nil {
		t.Fatalf("Compact2: %v", err)
	}
	if len(r.Kept) >= len(msgs) {
		t.Errorf("legacy truncate should still reduce; got Kept=%d input=%d", len(r.Kept), len(msgs))
	}
	if r.Kept[len(r.Kept)-1].Content != msgs[len(msgs)-1].Content {
		t.Errorf("legacy truncate should keep last message; got %q", r.Kept[len(r.Kept)-1].Content)
	}
}

func TestCompact2_ByteStableDeterministic(t *testing.T) {
	cfg := CompactorConfig{
		Mode:             ContextCompactionDeterministic,
		RecentTurns:      4,
		PreserveEvidence: true,
		MaxTokens:        8000,
	}
	c1 := NewCompactor(stubSummarizer("FIXED"))
	c1.Configure(cfg)
	c2 := NewCompactor(stubSummarizer("FIXED"))
	c2.Configure(cfg)
	msgs := []session.Message{
		{Role: "system", Content: "S"},
		{Role: "user", Content: "U1"},
		{Role: "assistant", Content: "A1"},
		{Role: "tool", Content: "T1", ToolCallID: "tc1"},
		{Role: "assistant", Content: "VERIFICATION PASSED"},
	}
	r1, _ := c1.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: cfg.Mode, MaxTokens: cfg.MaxTokens})
	r2, _ := c2.Compact2(context.Background(), CompactInput{Messages: msgs, Mode: cfg.Mode, MaxTokens: cfg.MaxTokens})
	if len(r1.Kept) != len(r2.Kept) {
		t.Errorf("deterministic mode should be byte-stable: r1=%d r2=%d", len(r1.Kept), len(r2.Kept))
	}
	for i := range r1.Kept {
		if !messagesEqual(r1.Kept[i], r2.Kept[i]) {
			t.Errorf("Kept[%d] differs: r1=%+v r2=%+v", i, r1.Kept[i], r2.Kept[i])
		}
	}
}

func messagesEqual(a, b session.Message) bool {
	if a.Role != b.Role || a.Content != b.Content || a.ToolCallID != b.ToolCallID {
		return false
	}
	if string(a.ToolCalls) != string(b.ToolCalls) {
		return false
	}
	return true
}

func TestCompact2_RecentTurnsConfigurable(t *testing.T) {
	c := NewCompactor(stubSummarizer("LLM"))
	msgs := make([]session.Message, 30)
	for i := range msgs {
		msgs[i] = session.Message{Role: "user", Content: "u"}
		if i%2 == 1 {
			msgs[i].Role = "assistant"
		}
	}
	msgs[0].Role = "system"
	msgs[0].Content = "SYSTEM-PROMPT"
	msgs[1].Role = "user"
	msgs[1].Content = "fix it"

	c.Configure(CompactorConfig{
		Mode:             ContextCompactionDeterministic,
		RecentTurns:      2,
		PreserveEvidence: false,
		MaxTokens:        8000,
	})
	r, _ := c.Compact2(context.Background(), CompactInput{
		Messages:  msgs,
		Mode:      ContextCompactionDeterministic,
		MaxTokens: 8000,
	})
	if want := 6; len(r.Kept) != want {
		t.Errorf("RecentTurns=2 retain count: got %d want %d", len(r.Kept), want)
	}
}

func TestIdentifyEvidence_FindsMarkers(t *testing.T) {
	msgs := []session.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "u"},
		{Role: "assistant", Content: "VERIFICATION PASSED"},
		{Role: "user", Content: "more"},
		{Role: "assistant", Content: "Open acceptance criteria: done"},
	}
	idx := evidenceIndices(msgs)
	wantSet := map[int]bool{2: true, 4: true}
	if len(idx) != len(wantSet) {
		t.Errorf("evidenceIndices set size: got %d want %d (idx=%v)", len(idx), len(wantSet), idx)
	}
	for _, k := range idx {
		if !wantSet[k] {
			t.Errorf("evidenceIndices unexpected index %d (idx=%v)", k, idx)
		}
	}
}

func containsMessage(s []session.Message, target session.Message) bool {
	for _, m := range s {
		if m.Role == target.Role && m.Content == target.Content {
			return true
		}
	}
	return false
}
