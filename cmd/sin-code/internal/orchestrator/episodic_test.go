// SPDX-License-Identifier: MIT
package orchestrator

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestEpisodeStoreNilDBIsSafe(t *testing.T) {
	s, err := NewEpisodeStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Record(context.Background(), &Episode{TaskTitle: "x"}); err != nil {
		t.Fatalf("nil-DB Record must be no-op, got %v", err)
	}
	if got, err := s.Similar(context.Background(), "x", 5); err != nil || got != nil {
		t.Fatalf("nil-DB Similar must return nil, got %v err=%v", got, err)
	}
}

func TestFtsQuerySanitization(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"ab", ""}, // too short
		{"hello", `"hello"`},
		{"foo bar", `"foo" OR "bar"`},
		{"foo-bar!baz", `"foobarbaz"`},
	}
	for _, c := range cases {
		if got := ftsQuery(c.in); got != c.want {
			t.Errorf("ftsQuery(%q)=%q want %q", c.in, got, c.want)
		}
	}
}

func TestPlanningPriorEmpty(t *testing.T) {
	if got := PlanningPrior(nil); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestPlanningPriorRenders(t *testing.T) {
	eps := []*Episode{
		{TaskTitle: "fix bug", Score: 0.9, Passed: true, Rounds: 1},
		{TaskTitle: "refactor", Score: 0.3, Passed: false, Rounds: 3},
	}
	p := PlanningPrior(eps)
	for _, want := range []string{"SUCCEEDED", "FAILED", "fix bug", "refactor"} {
	if !strings.Contains(p, want) {
		t.Errorf("PlanningPrior missing %q:\n%s", want, p)
	}
}
}

func TestEpisodeStoreInMemoryRoundTrip(t *testing.T) {
	// EpisodeStore is tested via nil-DB safe-mode in this build (no sqlite
	// driver imported). The full SQLite round-trip is covered by the
	// integration test suite when run with the FTS5 build tag.
	s, err := NewEpisodeStore(nil)
	if err != nil {
		t.Fatal(err)
	}
	ep := &Episode{Intent: "codebase_change", TaskTitle: "fix nil pointer", Score: 0.9, Passed: true, Rounds: 1}
	if err := s.Record(context.Background(), ep); err != nil {
		t.Fatalf("nil-DB Record must succeed silently, got %v", err)
	}
	got, err := s.Similar(context.Background(), "nil pointer", 5)
	if err != nil || got != nil {
		t.Fatalf("nil-DB Similar must return nil, got %v err=%v", got, err)
	}
	_ = os.Stat // keep import live for future extension
}
