// SPDX-License-Identifier: MIT
// Purpose: scoring heuristic for backlog triage (issue #162).
// See types.go for the data model; this file is the math.
package triage

import (
	"sort"
	"strings"
	"time"
)

// Signal weights are deliberately small integers. The total range
// after the table in the issue body is roughly [-5, +25]. Anything
// above +20 is "do this first"; below 0 is "ignore unless you have
// a reason." The numbers are operator-tuned and live in source control
// (not config) so they are auditable and reviewable like the rest of
// the agent.
const (
	wEpic          = 10
	wBlocked       = 5
	wAcceptance    = 3
	wNoV0          = 5
	wGoodFirst     = -3
	wStale30d      = -2
	wFresh7d       = 1
	wLoopSystem    = 4
	wFusion        = 2
	wMemoryOrSkill = 2
)

// Score runs the heuristic on a single issue, returning the Scored
// record. The all slice is used for BlocksCount (cross-references
// in bodies).
func Score(i Issue, now time.Time, all []Issue) Scored {
	s := Scored{Issue: i}
	add := func(pts int, reason string) {
		s.Score += pts
		s.Reasons = append(s.Reasons, reason)
	}

	if i.HasLabel("epic") {
		add(wEpic, "epic label")
	}
	if n := i.BlocksCount(all); n > 0 {
		add(wBlocked*n, blocksReason(n))
	}
	if hasAcceptanceSection(i.Body) {
		add(wAcceptance, "has acceptance criteria")
	}
	if !i.HasLabel("v0") {
		add(wNoV0, "not in v0 plan")
	}
	if i.HasLabel("good first issue") {
		add(wGoodFirst, "good first issue (deprioritize for operator)")
	}
	if ageDays(i.Updated(), now) > 30 {
		add(wStale30d, "stale (>30d)")
	} else if ageDays(i.Updated(), now) < 7 {
		add(wFresh7d, "fresh (<7d)")
	}
	if i.HasLabel("loop-system") {
		add(wLoopSystem, "loop-system label (current focus)")
	}
	if i.HasLabel("fusion") {
		add(wFusion, "fusion label (skill port)")
	}
	if i.HasLabel("memory") || i.HasLabel("v0") {
		add(wMemoryOrSkill, "memory/v0 label")
	}

	s.GroupKey = groupKey(i)
	return s
}

// ScoreAll scores every issue, sorts descending by score (stable), and
// groups by GroupKey. The grouping count is filled for dashboards.
func ScoreAll(in []Issue, now time.Time) ScoredList {
	all := make([]Scored, 0, len(in))
	for _, i := range in {
		all = append(all, Score(i, now, in))
	}
	sort.SliceStable(all, func(a, b int) bool {
		if all[a].Score != all[b].Score {
			return all[a].Score > all[b].Score
		}
		return all[a].Issue.Number < all[b].Issue.Number
	})
	counts := map[string]int{}
	for _, s := range all {
		counts[s.GroupKey]++
	}
	return ScoredList{Items: all, Total: len(all), GroupCounts: counts}
}

// groupKey picks the human-facing bucket for the markdown renderer.
// Order matters: the first matching label wins. Unlabeled issues go
// in "unlabeled".
func groupKey(i Issue) string {
	priority := []string{
		"epic",
		"loop-system",
		"fusion",
		"memory",
		"dx",
		"mcp",
		"v0",
		"enhancement",
		"bug",
		"documentation",
	}
	for _, p := range priority {
		if i.HasLabel(p) {
			return p
		}
	}
	// Fall through any non-priority label to its first label
	if len(i.Labels) > 0 {
		return i.Labels[0]
	}
	return "unlabeled"
}

func hasAcceptanceSection(body string) bool {
	// Cheap heuristic: presence of an "Acceptance criteria" heading
	// or the "## Acceptance" shorthand. Operators tend to write
	// "## Acceptance criteria" verbatim from the issue template.
	b := strings.ToLower(body)
	return strings.Contains(b, "acceptance criteria") ||
		strings.Contains(b, "## acceptance")
}

func ageDays(t, now time.Time) int {
	if t.IsZero() {
		return 1<<31 - 1 // effectively "stale" if missing
	}
	d := now.Sub(t).Hours() / 24
	if d < 0 {
		return 0
	}
	return int(d)
}

func blocksReason(n int) string {
	if n == 1 {
		return "blocks 1 other issue"
	}
	return blocksReasonN(n)
}

// blocksReasonN formats the count. Kept as its own function to make
// the pluralization rule explicit and testable.
func blocksReasonN(n int) string {
	// Standard English pluralization: "2 issues", "3 issues"...
	// No special-casing for 1 because Score() guards the singular.
	return "blocks " + itoa(n) + " other issues"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
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
