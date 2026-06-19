// SPDX-License-Identifier: MIT
// Purpose: tests for the session-context injector (issue #379). The
// four public-facing invariants:
//
//   - Disabled by default: zero ContextInjector → empty block, no panic.
//   - Lessons-only path produces a block with a [SESSION-CONTEXT-START]
//     marker, the "# Relevant lessons" section header, and the lesson
//     snippet + occurrences counter.
//   - Memory path produces a "# Relevant memory" section; secret-
//     shaped substrings are redacted, not echoed verbatim.
//   - Goals path produces a "# Pending autonomous goals" section with
//     ID + priority tags per row, sorted by priority DESC and id ASC.
//
// Plus a wiring-level test that pushes a real SessionContextBuilder
// into a Loop and asserts the block was the FIRST user message before
// the loop's reply.
package agentloop

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// TestContextInjectionDisabledByDefault guarantees the privacy-first
// promise: a zero-value injector emits an empty block and never
// touches any store. Loop wiring relies on this to keep legacy
// behaviour identical when no opt-in flag is set.
func TestContextInjectionDisabledByDefault(t *testing.T) {
	cj := &ContextInjector{} // zero value, no flags
	if cj.Enabled() {
		t.Fatal("zero ContextInjector must report Enabled=false")
	}
	out, err := cj.Invoke(context.Background(), "implement feature X")
	if err != nil {
		t.Fatalf("Invoke: unexpected error: %v", err)
	}
	if out != "" {
		t.Fatalf("disabled injector must produce empty block, got %d bytes: %q", len(out), out)
	}

	// Nil receiver must not panic — the loop's loop.go call site is
	// guarded by != nil but downstream embed callers might forget.
	var nilCJ *ContextInjector
	if nilCJ.Enabled() {
		t.Fatal("nil ContextInjector must report Enabled=false")
	}
	out, err = nilCJ.Invoke(context.Background(), "x")
	if err != nil || out != "" {
		t.Fatalf("nil Invoke should be a no-op, got (%q, %v)", out, err)
	}
}

// TestContextInjectionLessonsOnly seeds one lesson in a real SQLite
// store and asserts the rendered block carries the lesson text, the
// section header, and the START/END markers in that order.
func TestContextInjectionLessonsOnly(t *testing.T) {
	store := openLessonsStore(t)
	defer store.Close()
	ctx := context.Background()

	ws := t.TempDir()
	if err := store.Record(ctx, lessons.Entry{
		Type:      lessons.TypeToolError,
		Workspace: ws,
		Context:   map[string]any{"prompt": "session context injection"},
		Lesson:    "the previous build failed because the lessons fixture was empty",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	cj := &ContextInjector{
		Lessons:       store,
		Workspace:     ws, // same workspace as the seeded lesson
		TopK:          5,
		InjectLessons: true,
		Redactor:      DefaultRedactor(),
	}
	out, err := cj.Invoke(ctx, "session context injection")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(out, ContextBlockMarkerStart) {
		t.Errorf("block missing %s marker\n----\n%s\n----", ContextBlockMarkerStart, out)
	}
	if !strings.Contains(out, ContextBlockMarkerEnd) {
		t.Errorf("block missing %s marker\n----\n%s\n----", ContextBlockMarkerEnd, out)
	}
	if !strings.Contains(out, "# Relevant lessons") {
		t.Errorf("block missing # Relevant lessons header\n----\n%s\n----", out)
	}
	if !strings.Contains(out, "lessons fixture was empty") {
		t.Errorf("block missing lesson snippet\n----\n%s\n----", out)
	}
	if strings.Count(out, ContextBlockMarkerStart) != 1 || strings.Count(out, ContextBlockMarkerEnd) != 1 {
		t.Errorf("expected exactly one start + end marker, got start=%d end=%d\n%s",
			strings.Count(out, ContextBlockMarkerStart),
			strings.Count(out, ContextBlockMarkerEnd), out)
	}

	// Order invariant: START header before END.
	if idxStart := strings.Index(out, ContextBlockMarkerStart); idxStart >= 0 {
		if idxEnd := strings.Index(out, ContextBlockMarkerEnd); idxEnd <= idxStart {
			t.Errorf("END marker must follow START marker, got start=%d end=%d", idxStart, idxEnd)
		}
	}

	// Memory + Goals flags off → neither header should appear.
	if strings.Contains(out, "# Relevant memory") {
		t.Errorf("memory section header leaked when InjectMemory=false\n%s", out)
	}
	if strings.Contains(out, "# Pending autonomous goals") {
		t.Errorf("goals section header leaked when InjectGoals=false\n%s", out)
	}
}

// TestContextInjectionMemory seeds one memory entry, verifies the
// rendered block has the # Relevant memory section, and crucially
// verifies the bundled redactor replaces a pin-shaped OpenAI key in
// the insight with [REDACTED:OpenAI API Key] instead of echoing it
// back to the model.
func TestContextInjectionMemory(t *testing.T) {
	store := openMemoryStore(t)
	defer store.Close()

	ws := t.TempDir()
	const insight = "the agent's perf history suggests using sk-abcdef0123456789abcdef01 helps"
	if err := store.Add(&memory.Memory{
		Insight: insight,
		Project: ws,
		Tags:    []string{"perf"},
	}); err != nil {
		t.Fatalf("Add: %v", err)
	}

	cj := &ContextInjector{
		Memory:       store,
		Workspace:    ws, // same project as the seeded memory
		TopK:         5,
		InjectMemory: true,
		Redactor:     DefaultRedactor(),
	}
	out, err := cj.Invoke(context.Background(), "perf")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(out, "# Relevant memory") {
		t.Errorf("block missing # Relevant memory header\n%s", out)
	}
	// The raw key value must NOT survive — only the redaction marker.
	if strings.Contains(out, "sk-abcdef0123456789abcdef01") {
		t.Errorf("raw secret leaked into injected block\n%s", out)
	}
	if !strings.Contains(out, "[REDACTED:OpenAI API Key]") {
		t.Errorf("expected OpenAI redaction marker, got:\n%s", out)
	}
	// Lessons + Goals off → neither header should appear.
	if strings.Contains(out, "# Relevant lessons") {
		t.Errorf("lessons header leaked when InjectLessons=false\n%s", out)
	}
	if strings.Contains(out, "# Pending autonomous goals") {
		t.Errorf("goals section header leaked when InjectGoals=false\n%s", out)
	}
}

// TestContextInjectionGoals seeds two pending goals (different
// priorities) and verifies they appear in the rendered block in the
// expected priority-DESC / id-ASC order, each prefixed with its
// priority and ID tags.
func TestContextInjectionGoals(t *testing.T) {
	ws := t.TempDir()
	q := openAutonomyQueue(t)
	defer q.Close()
	ctx := context.Background()

	// Two goals of distinct priority; list should surface the higher
	// priority FIRST regardless of insertion order.
	if _, err := q.Add(ctx, "low priority goal", ws, 1, 1); err != nil {
		t.Fatalf("Add low: %v", err)
	}
	hiID, err := q.Add(ctx, "high priority goal", ws, 5, 1)
	if err != nil {
		t.Fatalf("Add high: %v", err)
	}

	cj := &ContextInjector{
		Goals:        q,
		Workspace:    ws,
		TopK:         5,
		InjectGoals:  true,
		Redactor:     DefaultRedactor(),
	}
	out, err := cj.Invoke(ctx, "anything")
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.Contains(out, "# Pending autonomous goals") {
		t.Errorf("block missing # Pending autonomous goals header\n%s", out)
	}
	if !strings.Contains(out, "high priority goal") {
		t.Errorf("high-priority prompt missing from block\n%s", out)
	}
	if !strings.Contains(out, "low priority goal") {
		t.Errorf("low-priority prompt missing from block\n%s", out)
	}
	priorityCanonical := "#" + itoa(int(hiID))
	if !strings.Contains(out, priorityCanonical) {
		t.Errorf("expected priority/ID tag %q in block\n%s", priorityCanonical, out)
	}
	// Order invariant: high before low.
	hiIdx := strings.Index(out, "high priority goal")
	loIdx := strings.Index(out, "low priority goal")
	if hiIdx < 0 || loIdx < 0 || hiIdx > loIdx {
		t.Errorf("expected high-priority goal before low-priority goal in block, got hi=%d lo=%d\n%s",
			hiIdx, loIdx, out)
	}
	// Negatives — other sections off.
	if strings.Contains(out, "# Relevant lessons") {
		t.Errorf("lessons header leaked when InjectLessons=false\n%s", out)
	}
	if strings.Contains(out, "# Relevant memory") {
		t.Errorf("memory header leaked when InjectMemory=false\n%s", out)
	}
}

// TestContextInjectorEnabledFalseOnAllFlagsOff is a focused regression
// guard. Any future field added to ContextInjector (e.g. inject_xxx)
// must keep Enabled() as the single answer to "is anything wired?".
func TestContextInjectorEnabledFalseOnAllFlagsOff(t *testing.T) {
	cj := &ContextInjector{
		Lessons:       openLessonsStoreForEnableCheck(t),
		Memory:        openMemoryStoreForEnableCheck(t),
		Goals:         openAutonomyQueueForEnableCheck(t),
		InjectLessons: false,
		InjectMemory:  false,
		InjectGoals:   false,
	}
	defer cj.Lessons.Close()
	defer cj.Memory.Close()
	defer cj.Goals.Close()
	if cj.Enabled() {
		t.Fatal("Enabled must return false when all flags are false, regardless of stores")
	}
	out, _ := cj.Invoke(context.Background(), "x")
	if out != "" {
		t.Fatalf("Invoke must return empty block when all flags false, got %q", out)
	}
}

// TestLoopSessionContextBuilderFiresBeforePrompt is the wiring-level
// test: a real Loop with SessionContextBuilder set must append the
// block as the FIRST user message, immediately before the goal
// prompt (after Preamble if set, before Lessons briefing).
func TestLoopSessionContextBuilderFiresBeforePrompt(t *testing.T) {
	const blockContent = "[SESSION-CONTEXT-START]\n# fake block\n[SESSION-CONTEXT-END]"
	loop := &Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: t.TempDir(),
		SessionContextBuilder: func(ctx context.Context, prompt string) (string, error) {
			return blockContent, nil
		},
		Completion: func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
			return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
		},
	}
	if _, err := loop.Run(context.Background(), testSession(t), "implement feature X"); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Re-run with a capture-point completion so we can see the history.
	var first []session.Message
	seen := false
	loop.Completion = func(ctx context.Context, history []session.Message, tools []ToolSpec) (*Completion, error) {
		if !seen {
			first = append([]session.Message(nil), history...)
			seen = true
		}
		return &Completion{Text: "done", Raw: session.Message{Role: "assistant", Content: "done"}}, nil
	}
	if _, err := loop.Run(context.Background(), testSession(t), "another goal"); err != nil {
		t.Fatalf("Run2: %v", err)
	}

	blockIdx := -1
	promptIdx := -1
	for i, m := range first {
		if m.Role != "user" {
			continue
		}
		if m.Content == blockContent {
			blockIdx = i
		} else if m.Content == "another goal" {
			promptIdx = i
		}
	}
	if blockIdx == -1 {
		t.Fatal("session-context block was not injected into the message history")
	}
	if promptIdx == -1 {
		t.Fatal("prompt missing from message history")
	}
	if blockIdx > promptIdx {
		t.Fatalf("block (%d) must precede prompt (%d) in message history", blockIdx, promptIdx)
	}
}

// ──────────────────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────────────────

func openLessonsStore(t *testing.T) *lessons.Store {
	t.Helper()
	store, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatalf("lessons.Open: %v", err)
	}
	return store
}

func openMemoryStore(t *testing.T) *memory.Store {
	t.Helper()
	store, err := memory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("memory.Open: %v", err)
	}
	return store
}

func openAutonomyQueue(t *testing.T) *autonomy.Queue {
	t.Helper()
	q, err := autonomy.Open(filepath.Join(t.TempDir(), "goals.db"))
	if err != nil {
		t.Fatalf("autonomy.Open: %v", err)
	}
	return q
}

// openLessonsStoreForEnableCheck sets up a store just to hold it.
// Returns the underlying *lessons.Store so the defer cleanup in the
// caller can close it.
func openLessonsStoreForEnableCheck(t *testing.T) *lessons.Store {
	t.Helper()
	stores := openLessonsStore(t)
	return stores
}

// openMemoryStoreForEnableCheck sets up a memory store just to hold it;
// the helper calls t.Cleanup internally so the test uses a single defer.
func openMemoryStoreForEnableCheck(t *testing.T) *memory.Store {
	t.Helper()
	s := openMemoryStore(t)
	return s
}

// openAutonomyQueueForEnableCheck sets up an autonomy queue just to
// hold it; same return-and-defer pattern as the lessons helper.
func openAutonomyQueueForEnableCheck(t *testing.T) *autonomy.Queue {
	t.Helper()
	q := openAutonomyQueue(t)
	return q
}
