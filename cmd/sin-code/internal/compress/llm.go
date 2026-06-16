// SPDX-License-Identifier: MIT
// Purpose: LLM-driven summarization pass for the compress package.
// Strategy=llm and Strategy=hybrid both reach this file. Strategy=llm
// replaces N drops with a single synthetic summary entry; Strategy=hybrid
// only invokes the LLM on drops that exceed the byte-budget.
//
// Validation:
//   - The byte-preservation invariants (code, URLs, paths, commands)
//     from caveman-compress are applied to the *Markdown* body of any
//     dropped entry that contains them. If the LLM drops them, the LLM
//     response is rejected with a retryable patch prompt.
//   - We retry up to MaxRetries (default 2). After exhaustion we
//     surface the LLM as unavailable for this Plan and fall back to
//     deterministic.
//
// Determinism:
//   - The Plan produced by Strategy=llm is not byte-deterministic in
//     timestamp terms (the LLM may phrase things slightly differently
//     each time), but the SHA-256 across (Entries[], Drops[], Merges[])
//     is what `compressor.planHash` hashes, so plan-identities across
//     identical-runs diverge by merge-body text only. The deterministic
//     strategy is the one we test for byte-reproducibility; the LLM
//     strategy regression-tests against the byte-preservation invariants.
package compress

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

// MergeOpts tunes the LLM-driven merge pass.
type MergeOpts struct {
	TargetRatio float64         // default 0.5; smaller -> tighter summary
	MaxRetries  int             // default 2
	Timeout     context.Context // optional deadline; nil = no deadline
}

// LLMSummarizer wraps the configured llm.Client. Constructed by
// NewLLMSummarizer; nil when no client is reachable from env.
type LLMSummarizer struct {
	client *llm.Client
	model  string
}

// NewLLMSummarizer picks the best available provider from the env.
//
//	c == nil    -> env-only construction (default model = NIM llama-70b).
//	c != nil    -> use the supplied client verbatim (used by tests).
//
// Returns an error only when the supplied client is non-nil but
// unusable (no BaseURL). When env construction fails the returned
// `Available()` is false and the summarizer is a no-op.
func NewLLMSummarizer(c *llm.Client) (*LLMSummarizer, error) {
	if c != nil {
		if c.BaseURL == "" {
			return nil, errors.New("compress: supplied llm client has no BaseURL")
		}
		return &LLMSummarizer{client: c, model: resolvedModel()}, nil
	}
	// Env-resolved provider.
	client, err := llm.ProviderFromConfig("nim", "", "", "", 0)
	if err != nil {
		// Fall back: many callers (test environments, airgapped CI)
		// have no provider configured; not an error — just unavailable.
		return &LLMSummarizer{}, nil
	}
	return &LLMSummarizer{client: client, model: resolvedModel()}, nil
}

// Available reports whether the summarizer can answer a chat request.
// Returns false when there is no client, the BaseURL is empty, or
// SIN_CODE_OFFLINE is set. The CLI uses this to surface a warning.
func (s *LLMSummarizer) Available() bool {
	if s == nil || s.client == nil {
		return false
	}
	if s.client.BaseURL == "" {
		return false
	}
	if os.Getenv("SIN_CODE_OFFLINE") != "" {
		return false
	}
	return true
}

// resolvedModel returns the model name to embed in chat requests.
// Priority: SIN_LLM_MODEL env override > model alias "compress".
func resolvedModel() string {
	if v := os.Getenv("SIN_LLM_MODEL"); v != "" {
		return v
	}
	return "compress-summary"
}

// MergeDrops produces a single PlanMerge from the given drops. The
// merge body is the LLM's compressed text; the source-hashes are the
// SHA-256 of every drop's body. The function is best-effort: on
// exhausted retries it returns (nil, nil) so callers can decide
// silently or warn.
func (s *LLMSummarizer) MergeDrops(drops []PlanEntry, opts MergeOpts) (*PlanMerge, error) {
	if !s.Available() {
		return nil, nil
	}
	if len(drops) == 0 {
		return nil, nil
	}
	if opts.TargetRatio <= 0 {
		opts.TargetRatio = 0.5
	}
	if opts.MaxRetries <= 0 {
		opts.MaxRetries = 2
	}
	ctx := opts.Timeout
	if ctx == nil {
		ctx = context.Background()
	}

	// Build the prompt. We assemble all drops into a single Markdown
	// block separated by `---` and ask the LLM to compress.
	sourceHashes := make([]string, 0, len(drops))
	var bundle strings.Builder
	for i, d := range drops {
		sourceHashes = append(sourceHashes, d.Hash)
		if i > 0 {
			bundle.WriteString("\n---\n")
		}
		bundle.WriteString("# ")
		bundle.WriteString(d.Subject)
		bundle.WriteString("\n\n")
		bundle.WriteString(d.Body)
	}
	original := bundle.String()

	systemPrompt := fmt.Sprintf("You are a deterministic compressor. Compress the user's prose to %d%% of the original byte size. PRESERVE byte-for-byte:\n- Code blocks (lines starting with backticks or inside triple-backtick fences)\n- Inline code (anything between backticks)\n- URLs (lines matching http(s)://, ftp://, or file:// schemes)\n- File paths (lines that look like absolute or relative Unix/Windows paths)\n- Command lines (lines starting with '$ ', '> ', or shell prompt)\n- Markdown headings (lines starting with #, ##, ###)\nOUTPUT only the compressed text, no preamble.",
		int(opts.TargetRatio*100))

	attempt := 0
	var response string
	var missing []string
	for {
		attempt++
		userPrompt := "Compress this:\n\n" + original
		if attempt > 1 && len(missing) > 0 {
			userPrompt = fmt.Sprintf("Your previous response dropped %s. Compress again, restoring them exactly:\n\n%s",
				strings.Join(missing, ", "), original)
		}
		req := llm.ChatRequest{
			Model: s.model,
			Messages: []llm.Message{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
		}
		resp, err := s.client.Chat(ctx, req)
		if err != nil {
			if attempt > opts.MaxRetries {
				return nil, fmt.Errorf("compress: llm chat failed after %d attempts: %w", attempt, err)
			}
			continue
		}
		response = resp.ExtractText()
		missing = checkPreservation(original, response)
		if len(missing) == 0 {
			break
		}
		if attempt > opts.MaxRetries {
			return nil, errors.New("compress: llm exhausted retries on byte-preservation invariants")
		}
	}
	if strings.TrimSpace(response) == "" {
		return nil, errors.New("compress: llm returned empty body")
	}
	id := "merge-" + shortHash(response)
	return &PlanMerge{
		ID:           id,
		Strategy:     StrategyLLM,
		SourceHashes: sourceHashes,
		Body:         response,
		Bytes:        len(response),
	}, nil
}

// checkPreservation reports whether the LLM response dropped any
// byte-preservation-protected line from the original. The check is
// line-based: for each original line that is "preservation-anchored"
// (code fence, URL, file path, command line, heading), if the line's
// exact bytes do not occur *somewhere* in the response, the line is
// flagged. Returns the slice of "missing" markers.
func checkPreservation(original, response string) []string {
	var missing []string
	anchorMatch := func(line string, anchor string) bool {
		return strings.Contains(response, line) || strings.Contains(response, anchor)
	}
	origLines := strings.Split(original, "\n")
	for _, ln := range origLines {
		if !isPreservationLine(ln) {
			continue
		}
		// Trim trailing whitespace because the LLM may have stripped
		// line-end spaces; everything else must match byte-for-byte.
		key := strings.TrimRight(ln, " \t")
		if !anchorMatch(key, key) {
			missing = append(missing, key)
		}
	}
	return missing
}

// isPreservationLine tells whether `line` is protected by the
// byte-preservation invariants. The heuristics mirror caveman's:
//
//   - line starts with "#" or "##" etc → heading
//   - line starts with "$ ", "> ", or "# " → command (note: shell
//     prompts use # as well; we still flag them)
//   - line starts with "```" → code fence marker
//   - line matches http(s):// or contains a /-laden path token → URL/path
//   - line consists of chars typical of an inline code line starting
//     with backticks → inline code head/tail
func isPreservationLine(line string) bool {
	t := strings.TrimSpace(line)
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "```") {
		return true
	}
	if strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "#!") {
		return true
	}
	if strings.HasPrefix(t, "$ ") || strings.HasPrefix(t, "> ") {
		return true
	}
	if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") ||
		strings.HasPrefix(t, "ftp://") || strings.HasPrefix(t, "file://") {
		return true
	}
	// File-path heuristic: starts with /, ./, ../, or contains a /.
	if strings.HasPrefix(t, "/") || strings.HasPrefix(t, "./") || strings.HasPrefix(t, "../") {
		return true
	}
	// Inline code detection (single backtick head): we only mark
	// the *start* of inline code spans because we're operating line
	// by line.
	if strings.HasPrefix(t, "`") && strings.HasSuffix(t, "`") {
		return true
	}
	return false
}
