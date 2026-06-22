// SPDX-License-Identifier: MIT
// Purpose: autodev pipeline — in-Go issue-to-PR primitives that complement
// the subprocess bridge in autodev.go. The bridge in autodev.go is the
// runtime subprocess boundary to the external OpenSIN-Code/autodev-cli
// (Python, MIT, v0.4.0). The pipeline here is the deterministic, stdlib-only
// surface the bridge can call into: stage state machine, GitHub issue
// fetch, branch-name sanitization, Conventional-Commits composer, and PR
// body renderer. Stdlib-only per M2 (CGO_ENABLED=0, single static binary).
//
// ghbridge is the only intra-module dependency and is wired through a
// pluggable IssueFetcher (ghbridge.Runner is already a test injection
// seam, so production code stays hermetic — see NewGHBridgeFetcher).
// Tests inject a stub via SetFetcher; production defaults to a
// ghbridge-backed fetcher installed via UseGHBridgeFetcher.
// Docs: autodev_pipeline.doc.md
package autodev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
)

// ── Pipeline: stage state machine for one issue ──────────────────────

// Stage enumerates the autodev lifecycle. The progression is
// queued → in_progress → done. Any other transition is rejected.
type Stage string

const (
	StageQueued     Stage = "queued"
	StageInProgress Stage = "in_progress"
	StageDone       Stage = "done"
)

// Event is a single stage-transition observation emitted by the
// Pipeline. Wired to hooks/fire via LogFunc in production.
type Event struct {
	Issue int       // GitHub issue number
	From  Stage     // stage before the transition ("" for the initial Enqueue)
	To    Stage     // stage after the transition
	At    time.Time // observed wall-clock time (UTC)
}

// LogFunc receives every Event the Pipeline emits. nil is a no-op.
type LogFunc func(Event)

// Pipeline drives an ordered set of issue→PR transitions. State per
// issue is in-memory only — the durable record belongs to autodev-cli's
// on-disk goal queue. The fields are guarded by mu so the type is safe
// under concurrent Advance from the daemon worker pool (M7).
type Pipeline struct {
	mu     sync.Mutex    // protects cur + events + log + clock
	cur    map[int]Stage // issue number → current stage
	events []Event       // accumulated log
	log    LogFunc       // observer sink (fanout for hooks/trace)
	clock  func() time.Time
}

// NewPipeline constructs an empty Pipeline with UTC clock and a no-op
// logger. Use SetLogger and SetClock to wire production sinks/clocks in
// tests and at caller sites.
func NewPipeline() *Pipeline {
	return &Pipeline{
		cur:   map[int]Stage{},
		clock: func() time.Time { return time.Now().UTC() },
		log:   func(Event) {},
	}
}

// SetLogger wires a LogFunc. nil reverts to no-op.
func (p *Pipeline) SetLogger(l LogFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if l == nil {
		p.log = func(Event) {}
		return
	}
	p.log = l
}

// SetClock wires the time source (test seam for byte-stability).
func (p *Pipeline) SetClock(f func() time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if f == nil {
		p.clock = func() time.Time { return time.Now().UTC() }
		return
	}
	p.clock = f
}

// Advance transitions issueNum from `from` to `to`. If from is "" this
// is the initial transition into StageQueued and the issue must not
// already be tracked. Returns ErrInvalidTransition if the current stage
// does not match `from`. Each successful transition emits one Event
// into the LogFunc AND appends to the internal events slice.
func (p *Pipeline) Advance(issueNum int, from, to Stage) error {
	if issueNum <= 0 {
		return fmt.Errorf("autodev: invalid issue number: %d", issueNum)
	}
	switch to {
	case StageQueued, StageInProgress, StageDone:
	default:
		return fmt.Errorf("autodev: invalid target stage: %q", to)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	cur, ok := p.cur[issueNum]
	if from == "" {
		if ok {
			return fmt.Errorf("%w: issue %d is already in %q (re-Enqueue refused)", ErrInvalidTransition, issueNum, cur)
		}
	} else {
		if !ok {
			return fmt.Errorf("%w: issue %d is not yet tracked (want from=%q)", ErrInvalidTransition, issueNum, from)
		}
		if cur != from {
			return fmt.Errorf("%w: issue %d is in %q, not %q", ErrInvalidTransition, issueNum, cur, from)
		}
	}
	p.cur[issueNum] = to
	evt := Event{Issue: issueNum, From: from, To: to, At: p.clock()}
	p.events = append(p.events, evt)
	p.log(evt)
	return nil
}

// Events returns a copy of the accumulated log in append order.
func (p *Pipeline) Events() []Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Event, len(p.events))
	copy(out, p.events)
	return out
}

// ErrInvalidTransition is returned by Advance on any stage-transition
// rule violation (re-enqueue, wrong `from`, unknown target).
var ErrInvalidTransition = errors.New("autodev: invalid transition")

// ── Issue + IssueFetcher + GitHub bridge ─────────────────────────────

// Issue is the parsed form of a GitHub issue fetched for the pipeline.
type Issue struct {
	Number int      // GitHub issue number
	Owner  string   // repository owner (e.g. "OpenSIN-Code")
	Repo   string   // repository name (e.g. "SIN-Code")
	Title  string   // issue title (trimmed)
	Body   string   // issue body (trimmed)
	Labels []string // normalized to lowercase
}

// IssueFetcher is the pluggable source of GitHub issues. Production
// uses NewGHBridgeFetcher so `gh issue view` does the work; tests
// install a stub via SetFetcher.
type IssueFetcher interface {
	Fetch(ctx context.Context, owner, repo string, num int) (*Issue, error)
}

// ghIssueViewJSON is the shape of `gh issue view --json title,body,labels`.
// We keep field names camelCase to match gh's wire format exactly.
type ghIssueViewJSON struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Labels []struct {
		Name string `json:"name"`
	} `json:"labels"`
}

// BridgeFetcher is the production IssueFetcher. It wraps any
// ghbridge.Runner (real `gh` for production; injected for hermetic tests
// that still want ghbridge classification coverage).
type BridgeFetcher struct {
	Run     ghbridge.Runner // runner required; nil returns ErrFetcherNotConfigured
	Timeout time.Duration   // per-call timeout; 0 means rely on ctx only
}

// NewGHBridgeFetcher composes a BridgeFetcher backed by the supplied
// ghbridge.Runner and timeout. Returns ErrFetcherNotConfigured when r
// is nil.
func NewGHBridgeFetcher(r ghbridge.Runner, timeout time.Duration) (*BridgeFetcher, error) {
	if r == nil {
		return nil, ErrFetcherNotConfigured
	}
	return &BridgeFetcher{Run: r, Timeout: timeout}, nil
}

// Fetch invokes `gh issue view <owner>/<repo>#<num> --json title,body,labels`
// through the injected runner, then JSON-decodes the result into an
// Issue. The runner classifies read-only `gh issue view` so this call
// never crosses the TierMutating line — callers must keep it that way.
func (f *BridgeFetcher) Fetch(ctx context.Context, owner, repo string, num int) (*Issue, error) {
	if f == nil || f.Run == nil {
		return nil, ErrFetcherNotConfigured
	}
	runCtx := ctx
	if f.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, f.Timeout)
		defer cancel()
	}
	ref := fmt.Sprintf("%s/%s#%d", owner, repo, num)
	stdout, _, err := f.Run(runCtx, []string{"issue", "view", ref, "--json", "title,body,labels"})
	if err != nil {
		return nil, fmt.Errorf("autodev: gh issue view %s: %w", ref, err)
	}
	var raw ghIssueViewJSON
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		return nil, fmt.Errorf("autodev: decode gh issue view JSON for %s: %w", ref, err)
	}
	labels := make([]string, 0, len(raw.Labels))
	for _, l := range raw.Labels {
		labels = append(labels, strings.ToLower(strings.TrimSpace(l.Name)))
	}
	return &Issue{
		Number: num,
		Owner:  owner,
		Repo:   repo,
		Title:  strings.TrimSpace(raw.Title),
		Body:   strings.TrimSpace(raw.Body),
		Labels: labels,
	}, nil
}

// ErrFetcherNotConfigured is returned by NewGHBridgeFetcher when the
// supplied ghbridge.Runner is nil (production wire-up mistake) and by
// NewIssueFromGitHub when no fetcher is installed for the current
// process (typical for tests that forgot to call SetFetcher).
var ErrFetcherNotConfigured = errors.New("autodev: issue fetcher not configured")

// defaultFetcher is the package-level fetcher. Production installs a
// BridgeFetcher via UseGHBridgeFetcher at process start; tests inject
// a stub via SetFetcher. Guarded by fetcherMu so M7's race contract is
// not silently broken under read-only access patterns.
var (
	fetcherMu      sync.RWMutex
	defaultFetcher IssueFetcher
)

// SetFetcher installs an IssueFetcher for NewIssueFromGitHub. Pass nil
// to clear it (tests should restore the previous value).
func SetFetcher(f IssueFetcher) {
	fetcherMu.Lock()
	defer fetcherMu.Unlock()
	defaultFetcher = f
}

// UseGHBridgeFetcher wires the default fetcher to a ghbridge-backed
// one. Convenience for autodev_cmd.go's startup path.
func UseGHBridgeFetcher(r ghbridge.Runner, timeout time.Duration) error {
	f, err := NewGHBridgeFetcher(r, timeout)
	if err != nil {
		return err
	}
	SetFetcher(f)
	return nil
}

// NewIssueFromGitHub fetches an issue using the currently-installed
// default fetcher. Returns ErrFetcherNotConfigured when no fetcher is
// installed.
func NewIssueFromGitHub(ctx context.Context, owner, repo string, num int) (*Issue, error) {
	fetcherMu.RLock()
	f := defaultFetcher
	fetcherMu.RUnlock()
	if f == nil {
		return nil, ErrFetcherNotConfigured
	}
	return f.Fetch(ctx, owner, repo, num)
}

// ── Branch-name sanitization ─────────────────────────────────────────

// branchForbidden matches any char not in git's safe-branch alphabet.
// We lowercase, collapse runs to '-', and trim leading/trailing '-'
// before prepending the autodev/<num>- prefix.
var branchForbidden = regexp.MustCompile(`[^a-z0-9._-]+`)

// branchSlug sanitizes a free-form string (issue title or short slug)
// into a git-safe token. Byte-stable: same input always emits the same
// output. Reserved-keyword "branch" is returned for empty/all-stripped
// inputs so the prefix never produces a malformed 'autodev/<num>-'.
func branchSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = branchForbidden.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "branch"
	}
	return s
}

// branchNameFromIssue composes a byte-stable branch name shaped as:
//
//	autodev/issue-<num>-<slug>
//
// where <slug> is the sanitized `title` argument. num must be > 0;
// callers without a sensible title may pass "" and get the literal
// "branch" token.
func branchNameFromIssue(num int, title string) string {
	if num <= 0 {
		num = 0
	}
	return fmt.Sprintf("autodev/issue-%d-%s", num, branchSlug(title))
}

// ── Conventional Commits composer ────────────────────────────────────

// CommitType is the Conventional Commits prefix. Lower-case only so
// the wire-format prefix matches the spec.
type CommitType string

const (
	CommitFeat     CommitType = "feat"
	CommitFix      CommitType = "fix"
	CommitChore    CommitType = "chore"
	CommitDocs     CommitType = "docs"
	CommitRefactor CommitType = "refactor"
	CommitTest     CommitType = "test"
)

// labelToType maps GitHub issue labels (lowercase) to commit types.
// The first match wins; if no label matches, CommitChore is the safe
// default. Adding entries here MUST preserve the byte-stable prefix
// list (the four-arm comparator in evalharness pins to it).
var labelToType = map[string]CommitType{
	"bug":           CommitFix,
	"fix":           CommitFix,
	"defect":        CommitFix,
	"regression":    CommitFix,
	"enhancement":   CommitFeat,
	"feature":       CommitFeat,
	"documentation": CommitDocs,
	"docs":          CommitDocs,
	"refactor":      CommitRefactor,
	"refactoring":   CommitRefactor,
	"test":          CommitTest,
	"tests":         CommitTest,
	"chore":         CommitChore,
	"maintenance":   CommitChore,
	"dependencies":  CommitChore,
}

// commitMessageFromIssue produces a Conventional Commits subject of the
// shape `<type>(issue-<n>): <title>` with the type inferred from the
// issue's labels. When no label matches, type defaults to "chore". The
// title is trimmed; if empty, "<issue n>" is used so the commit message
// is never blank.
func commitMessageFromIssue(iss Issue) string {
	typ := CommitChore
	for _, lbl := range iss.Labels {
		if v, ok := labelToType[strings.ToLower(strings.TrimSpace(lbl))]; ok {
			typ = v
			break
		}
	}
	title := strings.TrimSpace(iss.Title)
	if title == "" {
		title = fmt.Sprintf("issue %d", iss.Number)
	}
	if iss.Number > 0 {
		return fmt.Sprintf("%s(issue-%d): %s", typ, iss.Number, title)
	}
	return fmt.Sprintf("%s: %s", typ, title)
}

// ── PR body rendering ────────────────────────────────────────────────

// VerificationStatus is one of the three completion states the
// verification gate may report (mirrors agentloop's verify.report).
type VerificationStatus string

const (
	VerificationNone   VerificationStatus = "none"
	VerificationPassed VerificationStatus = "passed"
	VerificationFailed VerificationStatus = "failed"
)

// Goal is the high-level objective the PR addresses. Summary is the
// line that ends up inline in the PR body; Title is wrapped in italics
// if present.
type Goal struct {
	ID      string // e.g. "goal-<sha12>" (matches autonomy package)
	Title   string // short prose title
	Summary string // 1-3 sentence Markdown summary
}

// Result is the verification outcome the agent loop reported.
type Result struct {
	Status   VerificationStatus
	Evidence string // PoC/Oracle stdout or log excerpt
	IssueRef int    // GitHub issue number (mirrors Issue.Number)
	RunID    string // e.g. "verify-<runid>" for cross-references
}

// prBodyFromGoal renders a PR body with two H2 sections (Goal /
// Verification) plus a tail of issue / goal / run anchors. The output
// is byte-stable per (Goal, Result) pair (printer is a pure function).
func prBodyFromGoal(g Goal, r Result) string {
	var b strings.Builder
	b.WriteString("## Goal\n\n")
	summary := strings.TrimSpace(g.Summary)
	if summary != "" {
		b.WriteString(summary)
		b.WriteString("\n")
	}
	if t := strings.TrimSpace(g.Title); t != "" {
		b.WriteString("\n(" + t + ")\n")
	}
	b.WriteString("\n## Verification\n\n")
	b.WriteString("Status: **" + string(r.Status) + "**\n")
	if e := strings.TrimSpace(r.Evidence); e != "" {
		b.WriteString("\nEvidence:\n\n```\n")
		b.WriteString(e)
		b.WriteString("\n```\n")
	}
	b.WriteString("\n## Links\n")
	if r.IssueRef > 0 {
		b.WriteString(fmt.Sprintf("\n- Issue: #%d", r.IssueRef))
	}
	if g.ID != "" {
		b.WriteString(fmt.Sprintf("\n- Goal: `%s`", g.ID))
	}
	if r.RunID != "" {
		b.WriteString(fmt.Sprintf("\n- Verify run: `%s`", r.RunID))
	}
	b.WriteString("\n")
	return b.String()
}
