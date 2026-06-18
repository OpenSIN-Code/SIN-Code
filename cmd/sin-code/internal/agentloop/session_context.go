// SPDX-License-Identifier: MIT
// Purpose: privacy-first session-context injection (issue #379).
// ContextInjector collects top-K relevant entries from the lessons
// store, the long-term memory store, and the pending-autonomy-goals
// queue, runs every snippet through a SecretRedactor (so an API key
// accidentally recorded during a prior run never becomes the first
// thing a fresh agent reads), and emits a single markdown block that
// the agent loop prepends as the FIRST user message of a session.
//
// All three sources are OFF by default — privacy-first, opt-in only,
// never reaches into a user's history without an explicit
// `inject_*` flag. The Redactor is mandatory in practice (the loop
// sets a default one) unless a caller intentionally leaves it nil.
// Mandate M7: the injector holds no mutable init-time state and is
// safe for concurrent use; the underlying stores serialize their own
// access (modernc.org/sqlite, go.etcd.io/bbolt).
package agentloop

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/autonomy"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
)

// DefaultContextTopK is the per-source bound when ContextInjector.TopK
// is unset. Mirrors the documented config default (5).
const DefaultContextTopK = 5

// ContextBlockMarkerStart / End delimit the injected block in the
// first user message so downstream tools (loop transcripts, ledger
// snapshots, eval harnesses) can grep for it deterministically.
const (
	ContextBlockMarkerStart = "[SESSION-CONTEXT-START]"
	ContextBlockMarkerEnd   = "[SESSION-CONTEXT-END]"
)

// SecretRedactor replaces known-secret substrings with safe markers.
// Implementations must be pure (no observable state) and stable per
// (content) → byte-stable output so the eval harness can pin
// golden snapshots.
type SecretRedactor interface {
	Redact(content string) string
}

// passingRedactor is the zero-config Redactor returned when callers
// explicitly opt out of redaction. It is the documented escape hatch
// for tests that want raw content; production wiring must NOT use it.
type passingRedactor struct{}

func (passingRedactor) Redact(s string) string { return s }

// defaultSecretPatterns are the canonical regexes that the bundled
// redactor searches for. Compiled once at package init. Order:
// most-specific first so a partial match does not shadow a longer
// target (e.g. AWS access key before "key=..."). Common with the
// memory.GovernanceCapture set; duplicated here so this package does
// not import the memory package (avoids any future cycle risk).
var defaultSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`(?i)\b(sk-)[a-zA-Z0-9]{20,}\b`),
	regexp.MustCompile(`(?i)\b(ghp_|gho_|ghs_|ghr_)[a-zA-Z0-9]{36,}\b`),
	regexp.MustCompile(`(?i)\b(xox[bpsa]-)[a-zA-Z0-9-]{10,}\b`),
	regexp.MustCompile(`(?i)\b(sk-ant-)[a-zA-Z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)\b(vck_)[a-zA-Z0-9]{20,}\b`),
	regexp.MustCompile(`(?i)-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\b(eyJ)[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\.[a-zA-Z0-9_-]*\b`),
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[=:]\s*["']?[^\s"']{4,}`),
	regexp.MustCompile(`(?i)\b(api[_-]?key)\s*[=:]\s*["']?[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`(?i)\b(token)\s*[=:]\s*["']?[a-zA-Z0-9]{20,}`),
	regexp.MustCompile(`(?i)\b(bearer)\s+[a-zA-Z0-9_\-.=]{20,}`),
}

// defaultSecretLabels pairs each defaultSecretPatterns entry with a
// human-readable redaction label. Same index as defaultSecretPatterns.
var defaultSecretLabels = []string{
	"AWS Access Key ID",
	"OpenAI API Key",
	"GitHub Token",
	"Slack Token",
	"Anthropic API Key",
	"Vercel AI Gateway Key",
	"Private Key Block",
	"JWT Token",
	"Password Assignment",
	"API Key Assignment",
	"Token Assignment",
	"Bearer Token",
}

// defaultRedactor is the package-wide redactor wired by the loop
// unless the caller overrides it. Init-order safe: secretPatterns plus
// labels are package-level vars, populated at init.
type defaultRedactor struct{}

// DefaultRedactor returns the package-wide SecretRedactor. It is
// safe to share across goroutines — the regex set is read-only after
// init and the matchers never mutate shared state.
func DefaultRedactor() SecretRedactor { return defaultRedactor{} }

// Redact replaces every regex match with [REDACTED:<label>] so the
// agent reads a safe marker instead of a live credential. Behaviour
// matches memory.GovernanceCapture.Redact so the two layers never
// disagree when memory is wired alongside the injector.
func (defaultRedactor) Redact(content string) string {
	if content == "" {
		return content
	}
	for i, re := range defaultSecretPatterns {
		label := defaultSecretLabels[i]
		content = re.ReplaceAllString(content, "[REDACTED:"+label+"]")
	}
	return content
}

// ContextInjector is the concrete type wired by the loop. It is the
// single answer to "where does the context block come from?" — the
// loop calls its Invoke at session start when any opt-in flag is
// set. A zero-value ContextInjector is safe; Enabled() reports false.
type ContextInjector struct {
	// Lessons, Memory, Goals are the source stores. Any of them may be
	// nil; the corresponding InjectX flag should stay false in practice
	// (but Build() short-circuits nil stores anyway so a true flag with
	// a nil store is a soft no-op, not a panic).
	Lessons *lessons.Store
	Memory  *memory.Store
	Goals   *autonomy.Queue

	// Workspace scopes the Lessons and Goals queries (lesson rows are
	// keyed on workspace; goal rows include their workspace path) and is
	// passed to Memory.Prime as the project filter.
	Workspace string

	// TopK bounds the number of entries pulled from each source. Zero
	// is replaced with DefaultContextTopK at Invoke time so the
	// manifest in the rendered block is always N <= TopK.
	TopK int

	// InjectX flags. ALL three default false. Privacy-first — the
	// block is empty unless the user explicitly opts in per source.
	InjectLessons bool
	InjectMemory  bool
	InjectGoals   bool

	// Redactor re-secrets secret-shaped substrings before injection.
	// nil -> DefaultRedactor().
	Redactor SecretRedactor
}

// Enabled reports whether any InjectX flag is set. The loop uses it
// as a short-circuit — when false, Invoke is never called so the loop
// stays exactly as it was before this feature.
func (c *ContextInjector) Enabled() bool {
	if c == nil {
		return false
	}
	return c.InjectLessons || c.InjectMemory || c.InjectGoals
}

// Invoke produces the markdown block. Returns the empty string when
// the injector is disabled, when every source returns empty, or when
// an error short-circuits a single source (the other sources are
// still attempted; errors are surfaced as a single in-band note so
// the block is never silently truncated without trace).
//
// Output is byte-stable per (config, sources, prompt):
//   - marker ordering: START -> sections -> END
//   - section headers are exactly one of:
//     # Relevant lessons | # Relevant memory | # Pending autonomous goals
//   - lessons sorted by occurrences DESC then last_seen DESC
//   - memory sorted by Prime's internal order (already score-ranked)
//   - goals sorted by priority DESC then id ASC for ties
func (c *ContextInjector) Invoke(ctx context.Context, prompt string) (string, error) {
	if c == nil || !c.Enabled() {
		return "", nil
	}
	topK := c.TopK
	if topK <= 0 {
		topK = DefaultContextTopK
	}
	now := time.Now().UTC()

	var sections []string
	var errs []string

	if c.InjectLessons {
		s, err := c.buildLessonsSection(ctx, prompt, topK)
		if err != nil {
			errs = append(errs, "lessons: "+err.Error())
		} else if s != "" {
			sections = append(sections, s)
		}
	}
	if c.InjectMemory {
		s, err := c.buildMemorySection(ctx, prompt, topK)
		if err != nil {
			errs = append(errs, "memory: "+err.Error())
		} else if s != "" {
			sections = append(sections, s)
		}
	}
	if c.InjectGoals {
		s, err := c.buildGoalsSection(ctx, topK)
		if err != nil {
			errs = append(errs, "goals: "+err.Error())
		} else if s != "" {
			sections = append(sections, s)
		}
	}

	if len(sections) == 0 && len(errs) == 0 {
		return "", nil
	}

	var b strings.Builder
	b.WriteString(ContextBlockMarkerStart)
	b.WriteString("\n")
	b.WriteString("# Session context (auto-injected at session start, " +
		now.Format(time.RFC3339) + ")\n")
	for _, s := range sections {
		b.WriteString("\n")
		b.WriteString(s)
		b.WriteString("\n")
	}
	if len(errs) > 0 {
		sort.Strings(errs)
		b.WriteString("\n> Note: ")
		b.WriteString(strings.Join(errs, "; "))
		b.WriteString("\n")
	}
	b.WriteString(ContextBlockMarkerEnd)
	return b.String(), nil
}

// redactor returns the effective Redactor — never nil in production
// because DefaultRedactor is wired by the loopbuilder.
func (c *ContextInjector) redactor() SecretRedactor {
	if c.Redactor != nil {
		return c.Redactor
	}
	return DefaultRedactor()
}

// buildLessonsSection runs lessons.Store.QueryTopK which falls back
// internally to Query when the index is empty.
func (c *ContextInjector) buildLessonsSection(ctx context.Context, prompt string, topK int) (string, error) {
	if c.Lessons == nil {
		return "", nil
	}
	briefCtx := map[string]any{"prompt": prompt}
	entries, err := c.Lessons.QueryTopK(ctx, c.Workspace, briefCtx, topK)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	var b strings.Builder
	b.WriteString("# Relevant lessons\n")
	for _, e := range entries {
		b.WriteString("- [")
		b.WriteString(string(e.Type))
		b.WriteString("] ")
		b.WriteString(c.redactor().Redact(e.Lesson))
		b.WriteString(" (occurrences=")
		b.WriteString(itoa(e.Occurrences))
		b.WriteString(")\n")
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// buildMemorySection calls Memory.Prime, then re-renders so the
// redactor runs. Prime already returns a formatted block; we strip
// its own header so our section header is the only "#" line in the
// rendered block — keeps the manifest byte-stable.
func (c *ContextInjector) buildMemorySection(ctx context.Context, prompt string, topK int) (string, error) {
	if c.Memory == nil {
		return "", nil
	}
	primeBlock, err := c.Memory.Prime(prompt, c.Workspace, topK)
	if err != nil {
		return "", err
	}
	if primeBlock == "" {
		return "", nil
	}
	red := c.redactor().Redact(primeBlock)
	red = strings.TrimLeft(red, " \t\r\n")
	if i := strings.Index(red, "\n"); i > 0 && strings.HasPrefix(red, "#") {
		red = strings.TrimLeft(red[i+1:], " \t\r\n")
	}
	var b strings.Builder
	b.WriteString("# Relevant memory\n")
	b.WriteString(red)
	return strings.TrimRight(b.String(), "\n"), nil
}

// buildGoalsSection enumerates pending autonomous goals, sorted by
// priority then id so the order is stable across queries with equal
// priorities (issue #379 byte-stability requirement).
func (c *ContextInjector) buildGoalsSection(ctx context.Context, topK int) (string, error) {
	if c.Goals == nil {
		return "", nil
	}
	goals, err := c.Goals.List(ctx, autonomy.StatusPending)
	if err != nil {
		return "", err
	}
	if len(goals) == 0 {
		return "", nil
	}
	sort.SliceStable(goals, func(i, j int) bool {
		if goals[i].Priority != goals[j].Priority {
			return goals[i].Priority > goals[j].Priority
		}
		return goals[i].ID < goals[j].ID
	})
	if len(goals) > topK {
		goals = goals[:topK]
	}
	var b strings.Builder
	b.WriteString("# Pending autonomous goals\n")
	for _, g := range goals {
		b.WriteString(fmt.Sprintf("- [p=%d #%d] %s\n",
			g.Priority, g.ID, c.redactor().Redact(g.Prompt)))
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// itoa is a zero-allocation int formatter for short numbers — used
// in the block so json.Marshal is never pulled into a print path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
