// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the pipeline surface added in pipeline.go.
// Stdlib-only — no subprocess, no real gh bridge. The 5 tests are
// hermetic and assertion-clear:
//  1. TestPipeline_StageTransitions         — queued → in_progress → done
//  2. TestNewIssueFromGitHub_FetchesWithGH   — stub IssueFetcher → parses title/body/labels
//  3. TestBranchNaming_ByteStable           — branchNameFromIssue byte-stable + sanitizes
//  4. TestCommitMessage_ConventionalCommits  — commitMessageFromIssue Conventional Commits
//  5. TestPRBody_RendersGoalAndVerification  — prBodyFromGoal includes summary, status, links
//
// Docs: autodev_pipeline.doc.md
package autodev

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

// fakeFetcher is the minimal IssueFetcher stub. Tests dial it through
// SetFetcher. The RunArgs field tracks every (owner, repo, num) tuple
// the production shim passed through so the test for fetching can assert
// the GH wire args without needing a real gh classification.
type fakeFetcher struct {
	Issue   *Issue
	Err     error
	RunArgs []struct {
		Owner string
		Repo  string
		Num   int
	}
}

func (f *fakeFetcher) Fetch(_ context.Context, owner, repo string, num int) (*Issue, error) {
	f.RunArgs = append(f.RunArgs, struct {
		Owner string
		Repo  string
		Num   int
	}{Owner: owner, Repo: repo, Num: num})
	if f.Err != nil {
		return nil, f.Err
	}
	if f.Issue == nil {
		return nil, errors.New("fake: no issue installed")
	}
	// Defensive copy so the test can mutate the original without
	// disturbing previous fetches.
	cp := *f.Issue
	return &cp, nil
}

// ── 1. Stage transitions ─────────────────────────────────────────────

func TestPipeline_StageTransitions(t *testing.T) {
	p := NewPipeline()

	// Pin the clock so the At field is byte-stable per the spec; this
	// keeps any future four-arm comparator free to diff Events().
	frozen := time.Date(2026, 6, 19, 12, 0, 0, 0, time.UTC)
	p.SetClock(func() time.Time { return frozen })

	var observed []Event
	p.SetLogger(func(e Event) { observed = append(observed, e) })

	// Initial Enqueue (from "" → Queued).
	if err := p.Advance(4242, "", StageQueued); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// Queued → InProgress.
	if err := p.Advance(4242, StageQueued, StageInProgress); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// InProgress → Done.
	if err := p.Advance(4242, StageInProgress, StageDone); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	got := p.Events()
	if len(got) != 3 {
		t.Fatalf("Events len = %d, want 3; observed log len = %d", len(got), len(observed))
	}
	if len(observed) != 3 {
		t.Fatalf("LogFunc fired %d times, want 3 (one per transition)", len(observed))
	}

	wantTransitions := []struct {
		from, to Stage
	}{
		{"", StageQueued},
		{StageQueued, StageInProgress},
		{StageInProgress, StageDone},
	}
	for i, want := range wantTransitions {
		if got[i].From != want.from || got[i].To != want.to {
			t.Errorf("evt[%d] = %q→%q, want %q→%q", i, got[i].From, got[i].To, want.from, want.to)
		}
		if got[i].Issue != 4242 {
			t.Errorf("evt[%d].Issue = %d, want 4242", i, got[i].Issue)
		}
		if !got[i].At.Equal(frozen) {
			t.Errorf("evt[%d].At = %v, want frozen clock %v", i, got[i].At, frozen)
		}
	}

	// log + Events() return the same sequence (LogFunc isn't a parallel
	// observer; it shares the same source).
	if !reflect.DeepEqual(observed, got) {
		t.Errorf("LogFunc slice vs Events() differ:\nlog=%vevents=%v", observed, got)
	}

	// Defensive: invalid `from` is rejected, no event emitted.
	if err := p.Advance(4242, StageQueued, StageInProgress); !errors.Is(err, ErrInvalidTransition) {
		t.Errorf("Advance with wrong from: err = %v, want ErrInvalidTransition", err)
	}
	if len(p.Events()) != 3 {
		t.Errorf("rejected Advance still appended; events len = %d, want 3", len(p.Events()))
	}
}

// ── 2. NewIssueFromGitHub with stub fetcher ───────────────────────────

func TestNewIssueFromGitHub_FetchesWithGH(t *testing.T) {
	want := &Issue{
		Number: 173,
		Owner:  "OpenSIN-Code",
		Repo:   "SIN-Code",
		Title:  "compress-tools support Serve",
		Body:   "Make PONYTAIL tags the wire-format for sin-code serve --compress-tools.\n\nCloses #173.",
		Labels: []string{"enhancement", "Pipeline"},
	}
	fake := &fakeFetcher{Issue: want}

	prev := defaultFetcher // snapshot for restore
	t.Cleanup(func() { SetFetcher(prev) })
	SetFetcher(fake)

	got, err := NewIssueFromGitHub(context.Background(), "OpenSIN-Code", "SIN-Code", 173)
	if err != nil {
		t.Fatalf("NewIssueFromGitHub: %v", err)
	}

	if got.Title != want.Title {
		t.Errorf("Title = %q, want %q", got.Title, want.Title)
	}
	if got.Body != want.Body {
		t.Errorf("Body = %q, want %q", got.Body, want.Body)
	}
	// Labels are parsed by the BridgeFetcher (production path) and
	// lowercased there; the stub is pass-through. We assert exactly the
	// labels the test installed — the normalization property of the
	// BridgeFetcher is covered separately.
	wantLabels := []string{"enhancement", "Pipeline"}
	if !reflect.DeepEqual(got.Labels, wantLabels) {
		t.Errorf("Labels = %v, want %v (stub pass-through)", got.Labels, wantLabels)
	}
	if got.Number != 173 || got.Owner != "OpenSIN-Code" || got.Repo != "SIN-Code" {
		t.Errorf("envelope fields wrong: got %+v", got)
	}
	if len(fake.RunArgs) != 1 {
		t.Fatalf("fetcher invoked %d times, want 1", len(fake.RunArgs))
	}
	arg := fake.RunArgs[0]
	if arg.Owner != "OpenSIN-Code" || arg.Repo != "SIN-Code" || arg.Num != 173 {
		t.Errorf("fetcher args = %+v, want (OpenSIN-Code, SIN-Code, 173)", arg)
	}

	// Negative path: fetcher error is propagated as-is.
	SetFetcher(&fakeFetcher{Err: errors.New("gh: rate limit")})
	if _, err := NewIssueFromGitHub(context.Background(), "x", "y", 1); err == nil || !strings.Contains(err.Error(), "rate limit") {
		t.Errorf("fetcher error not propagated: %v", err)
	}
}

// ── 3. Branch-name byte stability ─────────────────────────────────────

func TestBranchNaming_ByteStable(t *testing.T) {
	// Same input, three times → identical bytes (the contract).
	a := branchNameFromIssue(173, "Add SOTA --compress-tools flag!")
	b := branchNameFromIssue(173, "Add SOTA --compress-tools flag!")
	c := branchNameFromIssue(173, "Add SOTA --compress-tools flag!")
	if a != b || b != c {
		t.Fatalf("not byte-stable:\n  a=%q\n  b=%q\n  c=%q", a, b, c)
	}
	// Sanitized: uppercase → lowercase, forbidden runs collapse to a
	// single '-'. The kept '-' chars from the source survive intact
	// (they're in the safe class `[^a-z0-9._-]+`'s negation). Two
	// forbidden runs on either side of the kept "--" therefore produce
	// 2+2 = 4 dashes before Trim. Trim strips only leading/trailing.
	wantSlug := "autodev/issue-173-add-sota---compress-tools-flag"
	if a != wantSlug {
		t.Errorf("BranchName = %q, want %q", a, wantSlug)
	}
	// No leading or trailing '-' on the slug itself after the numeric.
	if strings.HasSuffix(strings.TrimPrefix(a, fmtPrefix(173)), "-") {
		t.Errorf("slug has trailing '-': %q", a)
	}
	if strings.HasPrefix(strings.TrimPrefix(a, fmtPrefix(173)), "-") {
		t.Errorf("slug has leading '-': %q", a)
	}
	// Source special chars ('!') are dropped — slug (after the slash)
	// must not contain any source-special char.
	slug := strings.TrimPrefix(a, fmtPrefix(173))
	if strings.ContainsAny(slug, "!@#$%^&*()={}[]|\\:;\"'<>,?~") {
		t.Errorf("BranchName slug still contains a source special char: %q (slug=%q)", a, slug)
	}
	// Slash-depth must be exactly 1.
	if strings.Count(a, "/") != 1 {
		t.Errorf("BranchName = %q, want exactly one '/'", a)
	}

	// Empty / all-stripped title → deterministic "branch" token so the
	// prefix never produces 'autodev/issue-N-'.
	if got := branchNameFromIssue(7, "   "); got != "autodev/issue-7-branch" {
		t.Errorf("whitespace-only slug = %q, want %q", got, "autodev/issue-7-branch")
	}
	if got := branchNameFromIssue(7, "!!!@@@"); got != "autodev/issue-7-branch" {
		t.Errorf("all-stripped slug = %q, want %q", got, "autodev/issue-7-branch")
	}
	// Numbers / dots / underscores preserved (RFC 1123 git-safe).
	if got := branchNameFromIssue(42, "v1.2.3_rc1"); got != "autodev/issue-42-v1.2.3_rc1" {
		t.Errorf("numeric-safe slug = %q, want %q", got, "autodev/issue-42-v1.2.3_rc1")
	}
}

// fmtPrefix returns the literal `autodev/issue-N-` prefix for an issue
// number, deduped against pipeline.go's own composition so a routing
// change there produces a single test-site update.
func fmtPrefix(n int) string { return "autodev/issue-" + itoa(n) + "-" }

// itoa is a stdlib-only zero-allocation int conversion for small n.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// ── 4. Conventional Commits composer ──────────────────────────────────

func TestCommitMessage_ConventionalCommits(t *testing.T) {
	cases := []struct {
		name  string
		issue Issue
		want  string
	}{
		{
			name:  "bug label → fix",
			issue: Issue{Number: 173, Title: "compress tool missing", Labels: []string{"bug"}},
			want:  "fix(issue-173): compress tool missing",
		},
		{
			name:  "enhancement label → feat",
			issue: Issue{Number: 200, Title: "Add --compress-tools", Labels: []string{"enhancement"}},
			want:  "feat(issue-200): Add --compress-tools",
		},
		{
			name:  "feature label wins over bug (first match)",
			issue: Issue{Number: 7, Title: "two labels", Labels: []string{"feature", "bug"}},
			want:  "feat(issue-7): two labels",
		},
		{
			name:  "docs label",
			issue: Issue{Number: 9, Title: "Update README", Labels: []string{"docs"}},
			want:  "docs(issue-9): Update README",
		},
		{
			name:  "refactor label",
			issue: Issue{Number: 11, Title: "Collapse helpers", Labels: []string{"refactor"}},
			want:  "refactor(issue-11): Collapse helpers",
		},
		{
			name:  "test label",
			issue: Issue{Number: 13, Title: "Cover branch sanitization", Labels: []string{"test"}},
			want:  "test(issue-13): Cover branch sanitization",
		},
		{
			name:  "no labels → chore",
			issue: Issue{Number: 17, Title: "Bump deps"},
			want:  "chore(issue-17): Bump deps",
		},
		{
			name:  "unknown label → chore (safe default)",
			issue: Issue{Number: 19, Title: "Random noise", Labels: []string{"help-wanted", "good-first-issue"}},
			want:  "chore(issue-19): Random noise",
		},
		{
			name:  "empty title → fallback",
			issue: Issue{Number: 23, Title: "  ", Labels: []string{"enhancement"}},
			want:  "feat(issue-23): issue 23",
		},
		{
			name:  "labels normalized to lowercase before lookup",
			issue: Issue{Number: 29, Title: "X", Labels: []string{"BUG"}},
			want:  "fix(issue-29): X",
		},
		{
			name:  "issue number 0 → no scope suffix",
			issue: Issue{Number: 0, Title: "ad-hoc"},
			want:  "chore: ad-hoc",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := commitMessageFromIssue(tc.issue)
			if got != tc.want {
				t.Errorf("commitMessageFromIssue = %q, want %q", got, tc.want)
			}
		})
	}

	// Byte-stability: same Issue → same bytes.
	first := commitMessageFromIssue(cases[1].issue)
	for i := 0; i < 3; i++ {
		if got := commitMessageFromIssue(cases[1].issue); got != first {
			t.Fatalf("commitMessageFromIssue not byte-stable: %q vs %q", got, first)
		}
	}
}

// ── 5. PR body rendering ─────────────────────────────────────────────

func TestPRBody_RendersGoalAndVerification(t *testing.T) {
	g := Goal{
		ID:      "goal-abcd1234efgh",
		Title:   "Compress tool manifest",
		Summary: "Shrink the 47-tool wire manifest via the 5 ponytail tags.",
	}
	r := Result{
		Status:   VerificationPassed,
		Evidence: "sin-code serve --compress-tools: 47 → 31 tool entries (-34% bytes)",
		IssueRef: 173,
		RunID:    "verify-7f3a9c2e",
	}

	body := prBodyFromGoal(g, r)

	// Goal summary & title.
	if !strings.Contains(body, g.Summary) {
		t.Errorf("body missing goal summary:\n%s", body)
	}
	if !strings.Contains(body, g.Title) {
		t.Errorf("body missing goal title:\n%s", body)
	}
	// Verification section + status keyword.
	if !strings.Contains(body, "## Verification") {
		t.Errorf("body missing ## Verification:\n%s", body)
	}
	if !strings.Contains(body, "Status: **passed**") {
		t.Errorf("body missing Status: **passed**:\n%s", body)
	}
	// Evidence block.
	if !strings.Contains(body, r.Evidence) {
		t.Errorf("body missing evidence block:\n%s", body)
	}
	if !strings.Contains(body, "```") {
		t.Errorf("body missing code fence around evidence:\n%s", body)
	}
	// Links.
	if !strings.Contains(body, "## Links") {
		t.Errorf("body missing ## Links section:\n%s", body)
	}
	if !strings.Contains(body, "Issue: #173") {
		t.Errorf("body missing issue link:\n%s", body)
	}
	if !strings.Contains(body, "goal-abcd1234efgh") {
		t.Errorf("body missing goal ID:\n%s", body)
	}
	if !strings.Contains(body, "verify-7f3a9c2e") {
		t.Errorf("body missing verify run link:\n%s", body)
	}

	// Byte-stable: same (Goal, Result) twice → identical bytes.
	body2 := prBodyFromGoal(g, r)
	if body != body2 {
		t.Errorf("prBodyFromGoal not byte-stable:\n  a=%q\n  b=%q", body, body2)
	}

	// Status variants render the right status keyword.
	failed := prBodyFromGoal(g, Result{Status: VerificationFailed})
	if !strings.Contains(failed, "Status: **failed**") {
		t.Errorf("verification=failed body missing 'Status: **failed**':\n%s", failed)
	}
	none := prBodyFromGoal(g, Result{Status: VerificationNone})
	if !strings.Contains(none, "Status: **none**") {
		t.Errorf("verification=none body missing 'Status: **none**':\n%s", none)
	}

	// Optional fields omitted gracefully: with no IssueRef, no Goal ID,
	// and no RunID, the Links section must not emit any list-item
	// bullet. We tolerate the header itself (it documents the section).
	minimal := prBodyFromGoal(Goal{Summary: "x"}, Result{Status: VerificationPassed})
	if strings.Contains(minimal, "\n- ") {
		t.Errorf("minimal body should not emit list items when no link sources:\n%s", minimal)
	}
}
