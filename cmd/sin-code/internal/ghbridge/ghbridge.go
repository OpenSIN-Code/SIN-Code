// SPDX-License-Identifier: MIT
// Purpose: gh-bridge — typed, allowlisted wrapper around the official
// GitHub CLI (gh). Owns (1) the 3-tier verb classifier
// (read-only / mutating / forbidden), (2) a Runner abstraction so tests
// can inject a fake exec, and (3) the Health + Execute primitives used by
// the MCP layer. The gh binary itself is NEVER vendored (M2: single
// static Go binary, CGo-free, stdlib-only) — it is a runtime dependency
// the user installs via Homebrew/apt/etc.
// Docs: ghbridge.doc.md
package ghbridge

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// ── Public configuration constants ─────────────────────────────────────

// ServerName is the MCP server name registered in mcp.json. Used by
// sin-code mcp status to surface the bridge without scraping paths.
const ServerName = "gh"

// HealthTimeout is the cap for the "gh auth status" probe inside
// Health(). Kept separate from DefaultExecuteTimeout because a hung
// interactive auth prompt must not block the agent loop for a full
// minute.
const HealthTimeout = 30 * time.Second

// DefaultExecuteTimeout caps a single gh invocation. gh is generally
// fast (sub-second) but `gh run watch` or `gh workflow run --watch` can
// legitimately take many seconds; 60s is the right floor for a coding
// agent tool slot.
const DefaultExecuteTimeout = 60 * time.Second

// ── Tier classification ───────────────────────────────────────────────

// Tier classifies a gh command by what it can mutate. The bridge uses
// this to route read-only calls into the cheap gh_query tool and
// mutating calls into the ask-policy-gated gh_execute tool.
type Tier int

const (
	// TierReadOnly commands only read state (gh issue list, gh pr view).
	// Safe to call without an interactive prompt.
	TierReadOnly Tier = iota
	// TierMutating commands change state (gh pr merge, gh issue create).
	// Require ask-policy confirmation in headless mode.
	TierMutating
	// TierForbidden commands must NEVER reach the gh binary — either
	// the verb is destructive (delete) or the group is out of scope
	// (api, auth, config, secret, ...).
	TierForbidden
)

// String renders a Tier for diagnostics and the AllowedSurface log.
func (t Tier) String() string {
	switch t {
	case TierReadOnly:
		return "read-only"
	case TierMutating:
		return "mutating"
	default:
		return "forbidden"
	}
}

// ── Runner abstraction ─────────────────────────────────────────────────

// Runner executes a gh subcommand and returns captured stdout/stderr.
// Production code uses ExecRunner (real exec.CommandContext); tests
// inject a fake so they can assert on classification and capture
// without spawning a process.
type Runner func(ctx context.Context, args []string) (stdout, stderr string, err error)

// ExecRunner is the production Runner: it spawns `gh <args...>` with
// the caller's ctx (so cancellation propagates) and captures stdout +
// stderr separately. The returned error is *exec.ExitError when gh
// exits non-zero — callers should treat that as "gh ran, but
// reported a problem" (surface stderr) and any other error as "gh
// could not be run at all".
func ExecRunner(ctx context.Context, args []string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "gh", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Decorate exit errors so callers can distinguish "gh said no"
		// from "gh missing". The original err is wrapped for errors.Is.
		return stdout.String(), stderr.String(), fmt.Errorf("gh: %w", err)
	}
	return stdout.String(), stderr.String(), nil
}

// ── Bridge ─────────────────────────────────────────────────────────────

// Bridge is the typed gh wrapper. Construct with New() for production
// or NewWithRunner(...) for tests. The zero value is NOT usable
// (Runner is nil → panic on first call).
type Bridge struct {
	// Run executes the gh binary. Injectable for tests.
	Run Runner
	// Timeout is the per-call ctx deadline. Health() uses HealthTimeout
	// regardless so a hung `gh auth status` cannot pin a tool slot.
	Timeout time.Duration
}

// New returns a production Bridge backed by ExecRunner and
// DefaultExecuteTimeout.
func New() *Bridge {
	return &Bridge{
		Run:     ExecRunner,
		Timeout: DefaultExecuteTimeout,
	}
}

// NewWithRunner is the test constructor: caller provides a Runner and
// a Timeout. timeout=0 means "no per-call deadline; rely on ctx".
func NewWithRunner(r Runner, timeout time.Duration) *Bridge {
	return &Bridge{Run: r, Timeout: timeout}
}

// Health verifies the gh binary is installed and authenticated. The
// 30s cap is independent of b.Timeout because auth probes can hang on
// interactive SSO prompts and we want a tight bound.
func (b *Bridge) Health(ctx context.Context) error {
	if b == nil || b.Run == nil {
		return errors.New("ghbridge: nil bridge or runner")
	}
	// LookPath catches "gh not installed" before we spend a ctx
	// building an exec.Cmd.
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("ghbridge: gh binary not found on PATH: %w", err)
	}
	hctx, cancel := context.WithTimeout(ctx, HealthTimeout)
	defer cancel()
	stdout, stderr, err := b.Run(hctx, []string{"auth", "status"})
	if err != nil {
		// `gh auth status` exits non-zero when unauthenticated. We
		// want to surface that distinctly so the agent can prompt
		// for `gh auth login` instead of treating it as "gh broken".
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("ghbridge: auth check failed: %s", msg)
	}
	return nil
}

// Execute runs `gh <args>` after classifying them. The returned Tier
// tells the caller whether the call was read-only, mutating, or
// forbidden. Forbidden commands NEVER reach b.Run — that invariant is
// the whole point of the bridge.
//
// Returns the gh stdout (possibly truncated) on success; on error
// returns stderr + a wrapped error so the caller can surface a useful
// message to the LLM.
func (b *Bridge) Execute(ctx context.Context, args []string) (string, Tier, error) {
	if b == nil || b.Run == nil {
		return "", TierForbidden, errors.New("ghbridge: nil bridge or runner")
	}
	tier, err := Classify(args)
	if err != nil {
		// Defense in depth: classification failures are hard-stops.
		// The runner MUST NOT be invoked.
		return "", TierForbidden, fmt.Errorf("ghbridge: classify: %w", err)
	}
	if tier == TierForbidden {
		// Classify() already returned an err for the forbidden case
		// (we never return TierForbidden with err==nil), so this
		// branch is technically unreachable. Belt + suspenders.
		return "", TierForbidden, errors.New("ghbridge: forbidden command rejected")
	}
	// Apply per-call timeout on top of ctx.
	runCtx := ctx
	if b.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, b.Timeout)
		defer cancel()
	}
	stdout, stderr, runErr := b.Run(runCtx, args)
	if runErr != nil {
		// Prefer stderr (gh writes diagnostics there) and truncate
		// to keep tool replies bounded.
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = strings.TrimSpace(stdout)
		}
		if msg == "" {
			msg = runErr.Error()
		}
		return "", tier, fmt.Errorf("ghbridge: gh %s: %s", strings.Join(args, " "), truncate(msg, 500))
	}
	return stdout, tier, nil
}

// ── Classifier (the security core) ────────────────────────────────────

// allowedGroups is the closed set of gh top-level groups the bridge
// permits. Anything outside this map is hard-rejected. Keep this list
// in lockstep with the documentation in ghbridge.doc.md.
var allowedGroups = map[string]bool{
	"issue":    true,
	"pr":       true,
	"release":  true,
	"run":      true,
	"workflow": true,
	"repo":     true,
	"search":   true,
	"label":    true,
}

// readOnlyVerbs is the set of verbs (group+verb) the bridge considers
// safe to call without an interactive prompt. Anything not in here is
// classified as TierMutating or TierForbidden.
var readOnlyVerbs = map[string]bool{
	"list":   true,
	"view":   true,
	"status": true,
	"checks": true,
	"diff":   true,
}

// mutatingVerbs is the set of verbs that CHANGE state. They run only
// via gh_execute, which is ask-policy-gated in the agent loop. Adding
// a verb here means the agent can call it — but the user must approve.
var mutatingVerbs = map[string]bool{
	"create":  true,
	"comment": true,
	"edit":    true,
	"close":   true,
	"reopen":  true,
	"merge":   true,
	"review":  true,
	"ready":   true,
	"checkout": true,
	"label":   true,
	"pin":     true,
	"unpin":   true,
	"download": true,
	"rerun":   true,
	"cancel":  true,
	"watch":   true,
}

// forbiddenTokens is a deny-list scanned across ALL positions of the
// args slice. A match in the tail (`["issue", "list", "delete"]`)
// still hard-blocks. This catches obfuscation attempts where a
// forbidden verb hides behind a permitted group prefix.
var forbiddenTokens = map[string]bool{
	"delete":     true,
	"auth":       true,
	"secret":     true,
	"ssh-key":    true,
	"gpg-key":    true,
	"api":        true,
	"alias":      true,
	"extension":  true,
	"config":     true,
	"codespace":  true,
	"fork":       true,
	"sync":       true,
	"archive":    true,
	"unarchive":  true,
	"transfer":   true,
}

// Classify inspects a gh arg list and returns its Tier plus an error
// describing WHY a call is rejected. The function is FAIL-CLOSED:
// unknown input → TierForbidden with an explanatory error. Callers
// MUST treat TierForbidden as a hard reject even if err is nil.
//
// The classification rules in order:
//  1. zero args → forbidden (no group specified)
//  2. any forbidden token in any position → forbidden
//  3. args[0] not in allowedGroups → forbidden (group not exposed)
//  4. group == "search" → read-only (search has no verb)
//  5. len(args) < 2 → forbidden ("group requires verb")
//  6. args[1] in forbiddenTokens → forbidden (defense in depth,
//     re-checked here even though step 2 would have caught it)
//  7. args[1] in readOnlyVerbs → read-only
//  8. args[1] in mutatingVerbs → mutating
//  9. otherwise → forbidden ("unknown verb, fail closed")
func Classify(args []string) (Tier, error) {
	if len(args) == 0 {
		return TierForbidden, errors.New("ghbridge: no args provided (need a group, e.g. 'issue', 'pr')")
	}
	// Step 2: scan every position for a forbidden token. This catches
	// `gh issue list delete` and `gh repo view --json api` alike.
	for _, a := range args {
		if forbiddenTokens[a] {
			return TierForbidden, fmt.Errorf("ghbridge: forbidden token %q in args", a)
		}
	}
	group := args[0]
	if !allowedGroups[group] {
		return TierForbidden, fmt.Errorf("ghbridge: group %q is not in the allowlist (allowed: %s)", group, AllowedSurface())
	}
	// Step 4: search is verb-less.
	if group == "search" {
		return TierReadOnly, nil
	}
	if len(args) < 2 {
		return TierForbidden, fmt.Errorf("ghbridge: group %q requires a verb (e.g. list, view, create)", group)
	}
	verb := args[1]
	// Step 6: defensive re-check on the verb slot.
	if forbiddenTokens[verb] {
		return TierForbidden, fmt.Errorf("ghbridge: forbidden verb %q for group %q", verb, group)
	}
	if readOnlyVerbs[verb] {
		return TierReadOnly, nil
	}
	if mutatingVerbs[verb] {
		return TierMutating, nil
	}
	return TierForbidden, fmt.Errorf("ghbridge: unknown verb %q for group %q (fail closed)", verb, group)
}

// AllowedSurface returns a sorted, comma-joined list of the group
// names the bridge permits. Useful for `--help` text, the AllowedSurface
// skill output, and for the `gh_query` tool description so the model
// knows what it can call.
func AllowedSurface() string {
	groups := make([]string, 0, len(allowedGroups))
	for g := range allowedGroups {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	return strings.Join(groups, ", ")
}

// ── Internal helpers ──────────────────────────────────────────────────

// truncate caps s to at most n bytes, appending "…" when truncation
// actually happened. Used to keep error messages bounded in size when
// gh emits a large auth-prompt or JSON error blob.
func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
