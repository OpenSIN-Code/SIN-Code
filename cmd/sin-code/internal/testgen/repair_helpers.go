// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when testgen is refactored
package testgen

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// repairErrorIfLoop returns a non-empty error string only when the
// repair loop ran at least once and exhausted without success. Empty
// otherwise so CLI/test callers that only read res.Error see no
// regression versus the pre-loop behaviour.
func repairErrorIfLoop(maxIters int, numOutputs int) string {
	if maxIters <= 1 {
		return ""
	}
	return fmt.Sprintf("sin_test_generate: %d attempts failed; LLM repair exhausted. Review Result.TestOutput", maxIters)
}

// repairCases re-prompts the LLM with the failing `go test` output
// and expects a fresh Cases map (keyed as testgen expects). The
// signature mirrors Options.UseLLM (`func(ctx, code string) (string, error)`).
// Re-using the same closure is intentional: it already returns the
// Cases JSON the chat tool wired; we treat the failing output as
// context-injection by encoding it in the request body via the
// second argument to leverage caller-specific prompts.
func repairCases(ctx context.Context, file string, useLLM func(context.Context, string) (string, error), current map[string][]TestCase, failing string) (map[string][]TestCase, error) {
	if useLLM == nil {
		return current, nil
	}
	// Compose a "repair" prompt: snippet of the failing tail (last 4 KB is
	// plenty) glued to the current scaffold so the LLM knows what broke.
	cur, _ := json.Marshal(current)
	body := strings.Join([]string{
		string(cur),
		"# failing test output:",
		failing,
	}, "\n")
	raw, err := useLLM(ctx, body)
	if err != nil {
		return nil, err
	}
	// Caller's UseLLM returns either:
	// - a Cases map (JSON object keyed by function name), OR
	// - opaque text that is not a Cases map.
	// First try the map parse; on failure, treat the empty / non-JSON
	// reply as "LLM has nothing to suggest" (return nil so the caller
	// can break out of the loop early).
	var fresh map[string][]TestCase
	if uerr := json.Unmarshal([]byte(raw), &fresh); uerr != nil {
		return nil, nil
	}
	if len(fresh) == 0 {
		return nil, nil
	}
	return fresh, nil
}

// joinAttemptOutputs concatenates attempt log sections with a single
// newline between them. Empty sections are skipped so a single-pass
// run produces the same bytes as the pre-loop version.
func joinAttemptOutputs(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(p)
	}
	return b.String()
}
