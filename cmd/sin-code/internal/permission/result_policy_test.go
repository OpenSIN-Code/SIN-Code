// SPDX-License-Identifier: MIT
// Purpose: tests for the reactive permission result scanner (issue #374).
// All tests are designed to pass with -race.
package permission

import (
	"strings"
	"sync"
	"testing"
)

func TestResultPolicy_BenignNoOp(t *testing.T) {
	rp := NewResultPolicy()
	cases := []string{
		"all tests passed",
		"ok  42 test cases",
		"found 3 matching files",
		"build succeeded",
	}
	for _, c := range cases {
		act, reason := rp.ScanResult("sin_test", c)
		if act != ActionNoOp {
			t.Errorf("%q: expected noop, got %s (%q)", c, act, reason)
		}
	}
}

func TestResultPolicy_DestructiveWarn(t *testing.T) {
	rp := NewResultPolicy()
	cases := []struct {
		tool   string
		result string
	}{
		{"sin_bash", "removed directory /tmp/old"},
		{"sin_rm", "deleted 12 files"},
		{"sin_db", "truncated table users"},
	}
	for _, c := range cases {
		act, reason := rp.ScanResult(c.tool, c.result)
		if act != ActionWarn {
			t.Errorf("%q: expected warn, got %s", c.result, act)
		}
		if !strings.Contains(reason, "destructive") {
			t.Errorf("%q: expected destructive reason, got %q", c.result, reason)
		}
	}
}

func TestResultPolicy_NetworkEgressWarn(t *testing.T) {
	rp := NewResultPolicy()
	act, reason := rp.ScanResult("sin_probe", "outbound connection to external host 1.2.3.4")
	if act != ActionWarn {
		t.Errorf("expected warn, got %s", act)
	}
	if !strings.Contains(reason, "egress") {
		t.Errorf("expected egress reason, got %q", reason)
	}
}

func TestResultPolicy_SecretEscalate(t *testing.T) {
	rp := NewResultPolicy()
	cases := []struct {
		tool   string
		result string
	}{
		{"aws_cli", "AKIAIOSFODNN7EXAMPLE"},
		{"cat_env", "api_key=1234567890abcdef1234567890abcdef"},
		{"curl", "Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U"},
	}
	for _, c := range cases {
		act, reason := rp.ScanResult(c.tool, c.result)
		if act != ActionEscalate {
			t.Errorf("%q: expected escalate, got %s", c.result, act)
		}
		if !strings.Contains(reason, "secret") && !strings.Contains(reason, "token") {
			t.Errorf("%q: expected secret/token reason, got %q", c.result, reason)
		}
	}
}

func TestResultPolicy_SecretScannerAllowed(t *testing.T) {
	rp := NewResultPolicy()
	// A secret-scanning tool is expected to mention secrets; we should not
	// escalate it as if it leaked the secret itself.
	result := "found api_key=1234567890abcdef1234567890abcdef in file.env"
	act, reason := rp.ScanResult("sin_security_scan", result)
	if act != ActionNoOp {
		t.Errorf("secret scanner should be allowed to mention secrets, got %s (%q)", act, reason)
	}
}

func TestResultPolicy_ActionString(t *testing.T) {
	if ActionNoOp.String() != "noop" {
		t.Errorf("noop string = %q", ActionNoOp.String())
	}
	if ActionWarn.String() != "warn" {
		t.Errorf("warn string = %q", ActionWarn.String())
	}
	if ActionEscalate.String() != "escalate" {
		t.Errorf("escalate string = %q", ActionEscalate.String())
	}
}

func TestResultPolicy_SampleDetections(t *testing.T) {
	rp := NewResultPolicy()
	samples := SampleDetections()
	if len(samples) == 0 {
		t.Fatal("expected sample detections")
	}
	warnOrEscalate := 0
	for _, s := range samples {
		act, _ := rp.ScanResult(s.Tool, s.Result)
		if act == ActionWarn || act == ActionEscalate {
			warnOrEscalate++
		}
	}
	if warnOrEscalate == 0 {
		t.Error("expected at least one sample to trigger a reactive policy")
	}
}

// TestResultPolicy_ConcurrentCompileRace runs many concurrent scans to
// ensure the lazy sync.Once regex compilation is race-free (mandate M7).
func TestResultPolicy_ConcurrentCompileRace(t *testing.T) {
	const workers = 50
	const iterations = 100

	rp := NewResultPolicy()
	var wg sync.WaitGroup
	wg.Add(workers)
	for i := 0; i < workers; i++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				if idx%2 == 0 {
					rp.ScanResult("sin_test", "ok")
				} else {
					rp.ScanResult("sin_bash", "removed file")
				}
			}
		}(i)
	}
	wg.Wait()
}
