// SPDX-License-Identifier: MIT
// Purpose: tests for the triage scoring heuristic and renderer.
// No network, no gh — fixture data only.
package triage

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func mkIssue(num int, title string, labels []string, body string, updatedDaysAgo int) Issue {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	t := now.Add(-time.Duration(updatedDaysAgo) * 24 * time.Hour)
	return Issue{
		Number:    num,
		Title:     title,
		Body:      body,
		State:     "OPEN",
		Author:    "test",
		Labels:    labels,
		UpdatedAt: t.Format(time.RFC3339),
		CreatedAt: t.Format(time.RFC3339),
		URL:       "https://example.com/" + itoa(num),
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
}

func TestScore_EpicLabel(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "Epic test", []string{"epic"}, "", 1)
	s := Score(i, now, []Issue{i})
	if s.Score < wEpic {
		t.Fatalf("expected score >= %d for epic, got %d", wEpic, s.Score)
	}
}

func TestScore_AcceptanceCriteria(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "X", nil, "## Acceptance criteria\n- works\n", 1)
	s := Score(i, now, []Issue{i})
	if s.Score < wAcceptance {
		t.Fatalf("expected +%d for acceptance, got %d", wAcceptance, s.Score)
	}
}

func TestScore_Stale(t *testing.T) {
	now := fixedNow()
	fresh := mkIssue(1, "fresh", nil, "", 1)
	stale := mkIssue(2, "stale", nil, "", 60)
	sf := Score(fresh, now, []Issue{fresh, stale})
	ss := Score(stale, now, []Issue{fresh, stale})
	if ss.Score >= sf.Score {
		t.Fatalf("stale (%d) should score < fresh (%d)", ss.Score, sf.Score)
	}
}

func TestScore_BlocksCount(t *testing.T) {
	now := fixedNow()
	target := mkIssue(1, "target", nil, "", 1)
	referrer1 := mkIssue(2, "r1", nil, "depends on #1", 1)
	referrer2 := mkIssue(3, "r2", nil, "see #1 also", 1)
	all := []Issue{target, referrer1, referrer2}
	s := Score(target, now, all)
	if s.Score < 2*wBlocked {
		t.Fatalf("expected 2*%d from blocks, got %d", wBlocked, s.Score)
	}
}

func TestScore_BlocksCountExcludesSelf(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "x", nil, "self-ref #1 also #1", 1)
	s := Score(i, now, []Issue{i})
	// Self-refs must not count.
	for _, r := range s.Reasons {
		if strings.HasPrefix(r, "blocks") {
			t.Fatalf("unexpected self-block: %s", r)
		}
	}
}

func TestIssueTag(t *testing.T) {
	cases := map[int]string{
		0:  "#0",
		1:  "#1",
		42: "#42",
		99: "#99",
	}
	for n, want := range cases {
		if got := issueTag(n); got != want {
			t.Errorf("issueTag(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestContainsRef(t *testing.T) {
	cases := []struct {
		s, tag string
		want   bool
	}{
		{"see #1", "#1", true},
		{"#1 wins", "#1", true},
		{"x#1y", "#1", true}, // adjacent non-digit both sides
		{"#12 contains #1", "#1", true},    // #1 at end after space
		{"#12", "#1", false},               // #1 followed by digit
		{"prefix0#1", "#1", false},         // #1 preceded by digit
		{"#1234", "#123", false},           // #123 in #1234: digit boundary rejects prefix
		{"#1234", "#1234", true},
		{"#1234", "#12345", false},
		{"", "#1", false},
		{"no refs", "#1", false},
	}
	for _, c := range cases {
		got := containsRef(c.s, c.tag)
		if got != c.want {
			t.Errorf("containsRef(%q,%q) = %v, want %v", c.s, c.tag, got, c.want)
		}
	}
}

func TestScoreAll_StableOrder(t *testing.T) {
	now := fixedNow()
	high := mkIssue(1, "high", []string{"epic"}, "## Acceptance criteria", 1)
	mid := mkIssue(2, "mid", nil, "", 1)
	low := mkIssue(3, "low", []string{"good first issue"}, "", 60)
	list := ScoreAll([]Issue{low, high, mid}, now)
	if list.Items[0].Issue.Number != 1 {
		t.Errorf("expected #1 first, got #%d", list.Items[0].Issue.Number)
	}
	if list.Items[len(list.Items)-1].Issue.Number != 3 {
		t.Errorf("expected #3 last, got #%d", list.Items[len(list.Items)-1].Issue.Number)
	}
	if list.Total != 3 {
		t.Errorf("expected total=3, got %d", list.Total)
	}
}

func TestRender_Text(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "Sample", []string{"epic"}, "## Acceptance", 1)
	list := ScoreAll([]Issue{i}, now)
	var buf bytes.Buffer
	if err := Render(&buf, list, FormatText); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Sample") {
		t.Error("expected title in text output")
	}
	if !strings.Contains(out, "Backlog:") {
		t.Error("expected header in text output")
	}
}

func TestRender_MD(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "Sample", []string{"epic"}, "## Acceptance criteria", 1)
	list := ScoreAll([]Issue{i}, now)
	var buf bytes.Buffer
	if err := Render(&buf, list, FormatMD); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "# Backlog") {
		t.Error("expected # Backlog header")
	}
	if !strings.Contains(out, "## epic") {
		t.Error("expected epic section")
	}
	if !strings.Contains(out, "| Score |") {
		t.Error("expected score table")
	}
}

func TestRender_JSON(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "Sample", []string{"epic"}, "", 1)
	list := ScoreAll([]Issue{i}, now)
	var buf bytes.Buffer
	if err := Render(&buf, list, FormatJSON); err != nil {
		t.Fatal(err)
	}
	var back ScoredList
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, buf.String())
	}
	if back.Total != 1 {
		t.Errorf("expected total=1, got %d", back.Total)
	}
}

func TestGroupKey_FirstLabelWins(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "x", []string{"loop-system", "enhancement"}, "", 1)
	s := Score(i, now, []Issue{i})
	if s.GroupKey != "loop-system" {
		t.Errorf("expected loop-system group, got %q", s.GroupKey)
	}
}

func TestGroupKey_Unlabeled(t *testing.T) {
	now := fixedNow()
	i := mkIssue(1, "x", nil, "", 1)
	s := Score(i, now, []Issue{i})
	if s.GroupKey != "unlabeled" {
		t.Errorf("expected unlabeled, got %q", s.GroupKey)
	}
}

func TestHasLabel(t *testing.T) {
	i := Issue{Labels: []string{"epic", "loop-system"}}
	if !i.HasLabel("epic") {
		t.Error("HasLabel(epic) should be true")
	}
	if i.HasLabel("EPIC") {
		t.Error("HasLabel is case-sensitive")
	}
	if i.HasLabel("nope") {
		t.Error("HasLabel(nope) should be false")
	}
}

func TestUpdated_FallsBackToZero(t *testing.T) {
	i := Issue{}
	if !i.Updated().IsZero() {
		t.Error("expected zero time for empty UpdatedAt")
	}
	i.UpdatedAt = "garbage"
	if !i.Updated().IsZero() {
		t.Error("expected zero time for garbage UpdatedAt")
	}
}
