// SPDX-License-Identifier: MIT
// Purpose: Backlog triage — read open issues via gh, score, group, and
// render. The CLI is `sin-code triage` (issue #162).
//
// The scoring heuristic is operator-tuned, not LLM-tuned. The signals
// were chosen so the v0 Agent's current focus (loop-system cluster)
// ranks high, and stale / deprioritized items (good-first-issue,
// last-updated-30d) sink. See score.go for the math.
//
// Docs: triage.doc.md
package triage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
)

// Issue is the subset of the GitHub issue record that triage cares
// about. Anything not on this list (e.g. milestone, assignees list)
// is intentionally not modelled — the heuristic has no use for it
// today, and adding fields invites YAML drift.
type Issue struct {
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	State     string   `json:"state"`
	Author    string   `json:"author"`
	Labels    []string `json:"labels"`
	UpdatedAt string   `json:"updatedAt"`
	CreatedAt string   `json:"createdAt"`
	URL       string   `json:"url"`
}

// Scored is the input to the renderer. Score is the final heuristic
// score; Reasons is the per-signal breakdown so operators can audit
// why an issue ranks where it does. GroupKey is the label bucket for
// the markdown renderer.
type Scored struct {
	Issue    Issue    `json:"issue"`
	Score    int      `json:"score"`
	Reasons  []string `json:"reasons"`
	GroupKey string   `json:"groupKey"`
}

// ScoredList is the result of scoring. Sorted descending by score.
// GroupKey is the human label bucket (epic, loop-system, fusion, ...)
// or "unlabeled" if no label matches.
type ScoredList struct {
	Items []Scored `json:"items"`
	// Total is len(Items) — duplicated for the JSON envelope so the
	// consumer does not need to read both fields.
	Total int `json:"total"`
	// GroupCounts is the count of items per group key, in the same
	// order as Items (which is the score-descending order, not the
	// group order). Useful for dashboards.
	GroupCounts map[string]int `json:"groupCounts"`
}

// UpdatedAt parses the GitHub ISO 8601 timestamp and returns a time.
// Returns the zero value if the input is empty or unparseable; callers
// should treat that as "stale" via the "Last updated > 30 days" rule.
func (i Issue) Updated() time.Time {
	if i.UpdatedAt == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, i.UpdatedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// CreatedAt parses the GitHub ISO 8601 timestamp. Same failure mode
// as UpdatedAt.
func (i Issue) Created() time.Time {
	if i.CreatedAt == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, i.CreatedAt)
	if err != nil {
		return time.Time{}
	}
	return t
}

// HasLabel returns true if the issue has the given label (case-sensitive).
// Most labels are lowercase ("epic", "loop-system"), but we don't
// normalize — labels are operator-defined, not user input.
func (i Issue) HasLabel(name string) bool {
	for _, l := range i.Labels {
		if l == name {
			return true
		}
	}
	return false
}

// BlocksCount returns the number of issues that reference this issue
// in their body. This is a coarse proxy for "blocks other issues" —
// the real signal would require a graph walk via the GitHub API.
// We count `#NNN` references in the body. Self-references are excluded.
func (i Issue) BlocksCount(all []Issue) int {
	tag := issueTag(i.Number)
	count := 0
	for _, other := range all {
		if other.Number == i.Number {
			continue
		}
		if containsRef(other.Body, tag) || containsRef(other.Title, tag) {
			count++
		}
	}
	return count
}

func issueTag(n int) string {
	const digits = "0123456789"
	if n == 0 {
		return "#0"
	}
	var buf [16]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	return "#" + string(buf[i:])
}

func containsRef(s, tag string) bool {
	if s == "" || tag == "" {
		return false
	}
	for i := 0; i+len(tag) <= len(s); i++ {
		if s[i:i+len(tag)] == tag {
			// Require word boundary: previous char is not a digit
			// (to avoid matching #123 inside #1234). Next char is
			// not a digit either.
			if i > 0 && isDigit(s[i-1]) {
				continue
			}
			if i+len(tag) < len(s) && isDigit(s[i+len(tag)]) {
				continue
			}
			return true
		}
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// ── Loader (merged from loader.go) ──────────────────────────────────────

// Loader fetches open issues. The function is a variable so tests
// can inject a fixture without spawning gh.
var Loader = loadFromGH

// ghExec is a package-level hook for the ghbridge call so the
// loadFromGH error and parsing paths can be exercised without a live
// `gh` binary.
var ghExec = ghbridge.New().Execute

// loadFromGH runs `gh issue list --state open --json ...` via the
// bridge. The JSON fields are deliberately a strict subset of what
// gh can return — see the Issue struct for what we need.
func loadFromGH(ctx context.Context, repo string) ([]Issue, error) {
	args := []string{
		"issue", "list",
		"--state", "open",
		"--limit", "200",
		"--json",
		"number,title,body,state,author,labels,updatedAt,createdAt,url",
	}
	if repo != "" {
		args = append(args, "--repo", repo)
	}
	out, _, err := ghExec(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("gh issue list: %w", err)
	}
	var raw []struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Body   string `json:"body"`
		State  string `json:"state"`
		Author struct {
			Login string `json:"login"`
		} `json:"author"`
		Labels []struct {
			Name string `json:"name"`
		} `json:"labels"`
		UpdatedAt string `json:"updatedAt"`
		CreatedAt string `json:"createdAt"`
		URL       string `json:"url"`
	}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		return nil, fmt.Errorf("gh issue list: parse json: %w", err)
	}
	issues := make([]Issue, 0, len(raw))
	for _, r := range raw {
		labels := make([]string, 0, len(r.Labels))
		for _, l := range r.Labels {
			labels = append(labels, l.Name)
		}
		issues = append(issues, Issue{
			Number:    r.Number,
			Title:     r.Title,
			Body:      r.Body,
			State:     r.State,
			Author:    r.Author.Login,
			Labels:    labels,
			UpdatedAt: r.UpdatedAt,
			CreatedAt: r.CreatedAt,
			URL:       r.URL,
		})
	}
	return issues, nil
}
