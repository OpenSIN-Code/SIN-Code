// SPDX-License-Identifier: MIT
// Purpose: unit tests for the tool-coverage enforcer (issue #248, M7).
package agentloop

import (
	"sort"
	"sync"
	"testing"
)

func TestToolCoverageEnforcer_NoConstraints_Passes(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, nil)
	ok, missing, forbidden := e.Check()
	if !ok || len(missing) != 0 || len(forbidden) != 0 {
		t.Fatalf("expected pass, got ok=%v missing=%v forbidden=%v", ok, missing, forbidden)
	}
}

func TestToolCoverageEnforcer_MissingRequired(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc", "sin_oracle"}, nil)
	e.Record("sin_poc")
	ok, missing, forbidden := e.Check()
	if ok {
		t.Fatal("expected fail when a required tool is missing")
	}
	if len(missing) != 1 || missing[0] != "sin_oracle" {
		t.Fatalf("expected missing sin_oracle, got %v", missing)
	}
	if len(forbidden) != 0 {
		t.Fatalf("expected no forbidden violations, got %v", forbidden)
	}
	fb := e.Feedback(missing, forbidden)
	if fb == "" {
		t.Fatal("expected non-empty feedback")
	}
}

func TestToolCoverageEnforcer_AllRequiredUsed(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc", "sin_oracle"}, nil)
	e.Record("sin_oracle")
	e.Record("sin_poc")
	ok, missing, forbidden := e.Check()
	if !ok || len(missing) != 0 || len(forbidden) != 0 {
		t.Fatalf("expected pass, got ok=%v missing=%v forbidden=%v", ok, missing, forbidden)
	}
}

func TestToolCoverageEnforcer_ForbiddenUsed(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, []string{"sin_bash"})
	e.Record("sin_bash")
	ok, missing, forbidden := e.Check()
	if ok {
		t.Fatal("expected fail when forbidden tool is used")
	}
	if len(missing) != 0 || len(forbidden) != 1 || forbidden[0] != "sin_bash" {
		t.Fatalf("expected forbidden sin_bash, got missing=%v forbidden=%v", missing, forbidden)
	}
}

func TestToolCoverageEnforcer_ForbiddenNotUsed(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, []string{"sin_bash"})
	e.Record("sin_poc")
	ok, missing, forbidden := e.Check()
	if !ok || len(missing) != 0 || len(forbidden) != 0 {
		t.Fatalf("expected pass, got ok=%v missing=%v forbidden=%v", ok, missing, forbidden)
	}
}

func TestToolCoverageEnforcer_BothRequiredAndForbidden(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc"}, []string{"sin_bash"})
	e.Record("sin_bash")
	ok, missing, forbidden := e.Check()
	if ok {
		t.Fatal("expected fail")
	}
	if len(missing) != 1 || len(forbidden) != 1 {
		t.Fatalf("expected 1 missing and 1 forbidden, got missing=%v forbidden=%v", missing, forbidden)
	}
}

func TestToolCoverageEnforcer_Used(t *testing.T) {
	e := NewToolCoverageEnforcer(nil, nil)
	e.Record("sin_read")
	e.Record("sin_edit")
	e.Record("sin_read") // duplicate record must not duplicate Used
	used := e.Used()
	sort.Strings(used)
	want := []string{"sin_edit", "sin_read"}
	if len(used) != len(want) {
		t.Fatalf("expected %v, got %v", want, used)
	}
	for i := range want {
		if used[i] != want[i] {
			t.Fatalf("expected %v, got %v", want, used)
		}
	}
}

func TestToolCoverageEnstructor_OpenCriteria(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc"}, []string{"sin_bash"})
	oc := e.OpenCriteria([]string{"sin_poc"}, []string{"sin_bash"})
	if len(oc) != 2 {
		t.Fatalf("expected 2 criteria, got %v", oc)
	}
	if oc[0] != "required tool not used: sin_poc" {
		t.Fatalf("unexpected first criterion: %q", oc[0])
	}
	if oc[1] != "forbidden tool used: sin_bash" {
		t.Fatalf("unexpected second criterion: %q", oc[1])
	}
}

func TestToolCoverageEnforcer_Feedback(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_oracle"}, []string{"sin_bash"})
	fb := e.Feedback([]string{"sin_oracle"}, []string{"sin_bash"})
	if fb == "" {
		t.Fatal("expected feedback")
	}
	if fb[0] != 'Y' {
		t.Fatalf("expected feedback to start with directive, got %q", fb)
	}
}

func TestToolCoverageEnforcer_RaceSafe(t *testing.T) {
	e := NewToolCoverageEnforcer([]string{"sin_poc", "sin_oracle"}, []string{"sin_bash"})
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if n%3 == 0 {
				e.Record("sin_poc")
			} else if n%3 == 1 {
				e.Record("sin_oracle")
			} else {
				e.Record("sin_read")
			}
			_, _, _ = e.Check()
		}(i)
	}
	wg.Wait()
	ok, missing, forbidden := e.Check()
	if !ok {
		t.Fatalf("race-safe recording failed: missing=%v forbidden=%v", missing, forbidden)
	}
}
