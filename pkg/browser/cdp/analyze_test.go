// SPDX-License-Identifier: MIT
// Purpose: pure unit tests for the deterministic analysis layer.
//
// These tests do NOT require Chrome or Chromium. They run on every CI build
// and exercise grouping, correlation, diff, and Vitals threshold logic using
// synthetic events constructed from JSON literals.
package cdp

import (
	"encoding/json"
	"testing"
)

// mkEvent is a convenience constructor for synthetic test events.
func mkEvent(seq uint64, domain, method string, params interface{}) *Event {
	b, _ := json.Marshal(params)
	return &Event{Seq: seq, Domain: domain, Method: method, Params: b}
}

// ---------------------------------------------------------------------------
// Analyze
// ---------------------------------------------------------------------------

func TestAnalyzeGroupsRepeatedConsoleErrors(t *testing.T) {
	var events []*Event
	for i := uint64(1); i <= 5; i++ {
		events = append(events, mkEvent(i, "Runtime", "consoleAPICalled", map[string]interface{}{
			"type": "error",
			"args": []map[string]interface{}{{"value": json.RawMessage(`"same error"`)}},
		}))
	}
	findings := Analyze(events)
	if len(findings) != 1 {
		t.Fatalf("expected 1 grouped finding, got %d", len(findings))
	}
	if findings[0].Count != 5 {
		t.Errorf("expected count 5, got %d", findings[0].Count)
	}
}

func TestAnalyzeConsoleWarningSignature(t *testing.T) {
	events := []*Event{
		mkEvent(1, "Runtime", "consoleAPICalled", map[string]interface{}{
			"type": "warning",
			"args": []map[string]interface{}{{"value": json.RawMessage(`"heads up"`)}},
		}),
	}
	findings := Analyze(events)
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Severity != SevWarn {
		t.Errorf("expected warn severity, got %q", findings[0].Severity)
	}
	// Warnings must use the "console:warning:" prefix so they don't collide
	// with same-text errors.
	if len(findings[0].Signature) < len("console:warning:") ||
		findings[0].Signature[:len("console:warning:")] != "console:warning:" {
		t.Errorf("unexpected signature for console warning: %q", findings[0].Signature)
	}
}

// ---------------------------------------------------------------------------
// Correlate
// ---------------------------------------------------------------------------

func TestCorrelateLinksFailureToException(t *testing.T) {
	events := []*Event{
		mkEvent(1, "Network", "loadingFailed", map[string]interface{}{
			"type": "Script", "errorText": "net::ERR_CONNECTION_REFUSED",
		}),
		mkEvent(2, "Runtime", "exceptionThrown", map[string]interface{}{
			"exceptionDetails": map[string]interface{}{"text": "ReferenceError: x is not defined"},
		}),
	}
	chains := Correlate(events, 25)
	if len(chains) != 1 {
		t.Fatalf("expected 1 chain, got %d", len(chains))
	}
	if len(chains[0].Symptoms) != 1 {
		t.Errorf("expected 1 symptom linked to the failure, got %d", len(chains[0].Symptoms))
	}
}

func TestCorrelateIgnoresOutOfWindowSymptoms(t *testing.T) {
	events := []*Event{
		mkEvent(1, "Network", "loadingFailed", map[string]interface{}{
			"type": "Script", "errorText": "net::ERR_CONNECTION_REFUSED",
		}),
		// seq 100 is well outside window=5
		mkEvent(100, "Runtime", "exceptionThrown", map[string]interface{}{
			"exceptionDetails": map[string]interface{}{"text": "ReferenceError: late error"},
		}),
	}
	chains := Correlate(events, 5)
	if len(chains) != 0 {
		t.Errorf("expected 0 chains (symptom outside window), got %d", len(chains))
	}
}

// ---------------------------------------------------------------------------
// DiffReports
// ---------------------------------------------------------------------------

func TestDiffDetectsImprovement(t *testing.T) {
	before := &Report{
		Findings: []*Finding{{Signature: "exc:boom", Severity: SevError, Count: 1}},
		Summary:  ReportSummary{Errors: 1},
	}
	after := &Report{
		Findings: []*Finding{},
		Summary:  ReportSummary{Errors: 0},
	}
	d := DiffReports(before, after)
	if !d.Improved {
		t.Error("expected Improved=true when the only error is resolved")
	}
	if len(d.Resolved) != 1 {
		t.Errorf("expected 1 resolved finding, got %d", len(d.Resolved))
	}
	if len(d.Introduced) != 0 {
		t.Errorf("expected 0 introduced findings, got %d", len(d.Introduced))
	}
}

func TestDiffDetectsRegression(t *testing.T) {
	before := &Report{
		Findings: []*Finding{{Signature: "exc:a", Severity: SevError, Count: 1}},
		Summary:  ReportSummary{Errors: 1},
	}
	after := &Report{
		Findings: []*Finding{{Signature: "exc:b", Severity: SevError, Count: 1}},
		Summary:  ReportSummary{Errors: 1},
	}
	d := DiffReports(before, after)
	if d.Improved {
		t.Error("expected Improved=false: same error count and a new error introduced")
	}
	if len(d.Introduced) != 1 {
		t.Errorf("expected 1 introduced finding, got %d", len(d.Introduced))
	}
}

func TestDiffPersisted(t *testing.T) {
	f := &Finding{Signature: "http:404", Severity: SevWarn, Count: 2}
	before := &Report{Findings: []*Finding{f}, Summary: ReportSummary{Warnings: 2}}
	after := &Report{
		Findings: []*Finding{{Signature: "http:404", Severity: SevWarn, Count: 2}},
		Summary:  ReportSummary{Warnings: 2},
	}
	d := DiffReports(before, after)
	if len(d.Persisted) != 1 {
		t.Errorf("expected 1 persisted finding, got %d", len(d.Persisted))
	}
	if d.Improved {
		t.Error("should not be improved when errors unchanged")
	}
}

// ---------------------------------------------------------------------------
// vitalSeverity thresholds
// ---------------------------------------------------------------------------

func TestVitalSeverityThresholds(t *testing.T) {
	cases := []struct {
		name  string
		value float64
		want  Severity
	}{
		// LCP
		{"LCP", 1000, ""},
		{"LCP", 3000, SevWarn},
		{"LCP", 5000, SevError},
		// CLS
		{"CLS", 0.05, ""},
		{"CLS", 0.2, SevWarn},
		{"CLS", 0.4, SevError},
		// INP
		{"INP", 100, ""},
		{"INP", 300, SevWarn},
		{"INP", 700, SevError},
		// LongTask
		{"LongTask", 30, ""},
		{"LongTask", 100, SevInfo},
		{"LongTask", 250, SevWarn},
	}
	for _, c := range cases {
		got, _ := vitalSeverity(c.name, c.value)
		if got != c.want {
			t.Errorf("%s=%.2f: want %q, got %q", c.name, c.value, c.want, got)
		}
	}
}
