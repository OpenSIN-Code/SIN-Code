// SPDX-License-Identifier: MIT
// Purpose: LLM-driven test case filling for sin_test_generate. Builds a
// deterministic prompt, calls the configured OpenAI-compatible chat client,
// and extracts the table-driven cases it returns. Up to MaxRepairIters
// retry-and-repair passes run when the generated code does not compile.
//
// This is the Phase 5 deliverable for sin-debt#medium upgrade: add LLM case
// filling in Phase 2 (the stub was the placeholder).
//
// The prompt asks for a fenced JSON block so a misbehaving model that
// wraps the JSON in prose still parses. The parser tolerates leading
// fences and trims them.
package testgen

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// LLMOpts controls case-filling for a single run.
type LLMOpts struct {
	// MaxTokens caps the chat completion (default 1024).
	MaxTokens int
	// Temperature defaults to 0.2 (deterministic enough for tests).
	Temperature float64
	// MaxRepairIters is the upper bound on compile-and-repair passes
	// the caller runs afterFillCases returns a candidate. The test
	// generator owns the retry loop; this only affects prompt
	// formatting (e.g. "this is attempt N of M").
	MaxRepairIters int
}

// TestCase is the JSON shape we ask the LLM to return for one row of a
// table-driven test. Field names match the fallback template so the two
// paths produce the same final bytes. Free-form, exported so the model
// can be instructed about the expected schema.
type TestCase struct {
	Name    string         `json:"name"`
	Args    map[string]any `json:"args"`
	Wants   map[string]any `json:"wants"`
	WantErr bool           `json:"want_err,omitempty"`
}

// LLMResult is what FillCases returns. Cases is the row data the caller
// must splice into the scaffold (caller is responsible for emitting the
// final `func TestXxx(t *testing.T)` body). Raw is the untouched model
// response for debugging.
type LLMResult struct {
	Cases []TestCase
	Raw   string
}

// FillCasesWithLLM sends a single chat completion asking the model for
// 3-7 realistic test cases for fn. The signature is what was extracted
// by the fallback generator (see FuncInfo). Returns the parsed cases.
//
// The caller may pass MaxRepairIters > 0; this function does NOT loop —
// loop ownership stays with the test generator for clean race-safety.
func FillCasesWithLLM(ctx context.Context, client *llm.Client, model string, fn FuncInfo, opts LLMOpts) (LLMResult, error) {
	if client == nil {
		return LLMResult{}, fmt.Errorf("testgen: LLM client is nil")
	}
	if opts.MaxTokens <= 0 {
		opts.MaxTokens = 1024
	}
	if opts.Temperature == 0 {
		opts.Temperature = 0.2
	}
	prompt := buildCaseFillPrompt(fn, opts)

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Model:       model,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Messages: []llm.Message{
			{Role: "system", Content: systemPromptForTestCases()},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return LLMResult{}, fmt.Errorf("testgen: LLM chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return LLMResult{}, fmt.Errorf("testgen: LLM returned no choices")
	}
	raw := resp.Choices[0].Message.Content
	cases, perr := parseCaseFillResponse(raw)
	if perr != nil {
		// Surface the raw text so the caller can decide whether to retry
		// through its repair loop.
		return LLMResult{Raw: raw}, perr
	}
	return LLMResult{Cases: cases, Raw: raw}, nil
}

// systemPromptForTestCases is the role that anchors the model's tone
// and contract. Keep it short and unambiguous.
func systemPromptForTestCases() string {
	return "You generate realistic Go table-driven test cases." +
		" Reply ONLY with a fenced ```json``` block containing a JSON array." +
		" Each element is {name, args, wants, want_err}." +
		" Do NOT include any Go source, prose, or commentary."
}

// buildCaseFillPrompt converts the function signature into a
// deterministic prompt. The attempt counter shows up in repair passes.
func buildCaseFillPrompt(fn FuncInfo, opts LLMOpts) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Function: %s\n", fn.Name)
	if fn.IsMethod {
		fmt.Fprintf(&b, "Receiver: %s\n", fn.Receiver)
	}
	if len(fn.Args) > 0 {
		fmt.Fprintln(&b, "Args:")
		for _, a := range fn.Args {
			fmt.Fprintf(&b, "  - %s %s\n", a.Name, a.Type)
		}
	} else {
		fmt.Fprintln(&b, "Args: (none)")
	}
	if len(fn.Returns) > 0 {
		fmt.Fprintln(&b, "Returns:")
		for _, r := range fn.Returns {
			fmt.Fprintf(&b, "  - %s %s\n", r.Name, r.Type)
		}
	}
	fmt.Fprintf(&b, "ReturnsError: %v\n", fn.HasError)
	if opts.MaxRepairIters > 0 {
		fmt.Fprintf(&b, "Attempt: %d of %d\n", opts.MaxRepairIters, opts.MaxRepairIters)
	}
	fmt.Fprintln(&b, "Generate 3-7 realistic test cases that exercise the happy path, common edges, and any error branches. Output a JSON array only.")
	return b.String()
}

var fencedJSONRe = regexp.MustCompile("(?s)```json\\s*\\n?(.*?)\\n?```")

// parseCaseFillResponse extracts the JSON array. Tolerates bare JSON
// without fences and leading prose.
func parseCaseFillResponse(raw string) ([]TestCase, error) {
	trimmed := strings.TrimSpace(raw)
	m := fencedJSONRe.FindStringSubmatch(trimmed)
	if len(m) >= 2 {
		trimmed = strings.TrimSpace(m[1])
	}
	if !strings.HasPrefix(trimmed, "[") {
		// Fall back to finding the first '[' and last ']'.
		start := strings.Index(trimmed, "[")
		end := strings.LastIndex(trimmed, "]")
		if start < 0 || end <= start {
			return nil, fmt.Errorf("testgen: response has no JSON array")
		}
		trimmed = trimmed[start : end+1]
	}
	var cases []TestCase
	if err := json.Unmarshal([]byte(trimmed), &cases); err != nil {
		return nil, fmt.Errorf("testgen: parse cases: %w", err)
	}
	if len(cases) == 0 {
		return nil, fmt.Errorf("testgen: empty case list")
	}
	return cases, nil
}
