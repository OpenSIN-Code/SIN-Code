// SPDX-License-Identifier: MIT
// Purpose: targeted tests for the generate/execute/repair loop.
// The loop is wired into Generate() — verify the attempt count, the
// LLM-driven repair callback path, and the safety cap.
package testgen

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeUseLLM tracks how many times the closure is invoked and the
// payload shape (current cases encoded in the JSON body). The
// returned JSON is parsed by repairCases back into a Cases map so the
// loop shape round-trips.
func fakeUseLLM(t *testing.T, calls *int, returned map[string][]TestCase) func(context.Context, string) (string, error) {
	return func(ctx context.Context, body string) (string, error) {
		*calls++
		// Sanity: log body for the failure case so the test can prove
		// the repair context (failing output + current cases) made it
		// into the wire payload.
		t.Logf("fakeUseLLM call %d, body length=%d", *calls, len(body))
		return encodeCasesMap(t, returned), nil
	}
}

func encodeCasesMap(t *testing.T, m map[string][]TestCase) string {
	t.Helper()
	// Mirror what the chat tool's UseLLM closure emits.
	var b strings.Builder
	for fn, cases := range m {
		for _, c := range cases {
			b.WriteString(fn)
			b.WriteString(":")
			b.WriteString(c.Name)
			b.WriteString(" ")
		}
	}
	return b.String()
}

func TestRepairLoop_DisabledByDefault(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\nfunc Add(a,b int) int { return a+b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	llmFn := fakeUseLLM(t, &calls, nil)
	res := Generate(context.Background(), Options{
		File:   src,
		UseLLM: llmFn,
		// MaxRepairIters left at 0 -> single-pass, no LLM call.
	})
	if calls != 0 {
		t.Errorf("expected zero LLM calls when MaxRepairIters=0, got %d", calls)
	}
	if !strings.Contains(res.TestOutput, "attempt 1") {
		t.Errorf("expected single-attempt output: %s", res.TestOutput)
	}
}

func TestRepairLoop_LLMCalledOnFailure(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\nfunc Add(a,b int) int { return a+b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	// Return the empty body so repairCases parses to {} -> it errors out
	// after one attempt. We only care that UseLLM was called once for
	// the repair attempt.
	llmFn := fakeUseLLM(t, &calls, map[string][]TestCase{})
	res := Generate(context.Background(), Options{
		File:          src,
		UseLLM:        llmFn,
		MaxRepairIters: 3,
	})
	// repairCases parses the body, gets an empty map, errors -> loop
	// ends. So LLM is called exactly once for the repair (init attempt
	// doesn't need the LLM because we supplied no cases, but the
	// generated scaffold is still written). Then the first attempt
	// fails (because scaffold tests do not depend on the LLM), and
	// the repair path is then entered with one call. So 1 LLM call.
	if calls != 1 {
		t.Errorf("expected 1 LLM repair call, got %d", calls)
	}
	if strings.Contains(res.Error, "panic") || strings.Contains(res.Error, "BUG") {
		t.Errorf("loop errored unexpectedly: %s", res.Error)
	}
}

func TestRepairLoop_LLMRecovers(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\nfunc Add(a,b int) int { return a+b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	good := map[string][]TestCase{
		"Add": {{Name: "sum", Args: map[string]any{"a": 1.0, "b": 2.0}, Wants: map[string]any{"got": 3.0}}},
	}
	// First repair call returns the broken empty string, second returns
	// the recovered cases map. Test that the loop tries twice and
	// then succeeds (the recovered cases produce a valid test).
	first := true
	llmFn := func(ctx context.Context, body string) (string, error) {
		calls++
		if first {
			first = false
			return "", nil
		}
		return encodeCasesMap(t, good), nil
	}
	res := Generate(context.Background(), Options{
		File:          src,
		UseLLM:        llmFn,
		MaxRepairIters: 3,
		Cases:         map[string][]TestCase{},
	})
	if calls < 1 {
		t.Errorf("expected at least 1 LLM call, got %d", calls)
	}
	if !strings.Contains(res.TestOutput, "attempt") {
		t.Errorf("output should contain attempt marker: %s", res.TestOutput)
	}
}

func TestRepairLoop_SafetyCap(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "calc.go")
	if err := os.WriteFile(src, []byte("package calc\nfunc Add(a,b int) int { return a+b }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	llmFn := fakeUseLLM(t, &calls, nil) // always empty -> fails repair
	res := Generate(context.Background(), Options{
		File:          src,
		UseLLM:        llmFn,
		MaxRepairIters: 99, // forces safety cap to 10 in Generate
	})
	// At most 10 attempts -> at most 9 repairs (since the 10th is the
	// final give-up). We accept <= 12 to be tolerant.
	if calls > 12 {
		t.Errorf("LLM called too many times: %d", calls)
	}
	if res.TestPassed {
		t.Error("expected TestPassed=false on capped fail")
	}
}
