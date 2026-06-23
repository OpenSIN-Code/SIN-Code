package internal

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestSeverityToSarifLevel(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"critical", "error"},
		{"HIGH", "error"},
		{"medium", "warning"},
		{"warning", "warning"},
		{"low", "note"},
		{"note", "note"},
		{"info", "note"},
		{"", "warning"},
	}
	for _, c := range cases {
		if got := severityToSarifLevel(c.in); got != c.want {
			t.Errorf("severityToSarifLevel(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeFindingPaths(t *testing.T) {
	findings := []SecurityFinding{
		{File: "/workspace/foo.go"},
		{File: "bar.go"},
		{File: ""},
		{File: "/workspace/baz/qux.go"},
	}
	out := normalizeFindingPaths("/workspace", findings)
	if out[0].File != "foo.go" {
		t.Errorf("expected foo.go, got %q", out[0].File)
	}
	if out[1].File != "bar.go" {
		t.Errorf("expected bar.go, got %q", out[1].File)
	}
	if out[2].File != "" {
		t.Errorf("expected empty, got %q", out[2].File)
	}
	if out[3].File != "baz/qux.go" {
		t.Errorf("expected baz/qux.go, got %q", out[3].File)
	}
}

func TestToSarif(t *testing.T) {
	findings := []SecurityFinding{
		{
			RuleID:      "SAST-001",
			RuleName:    "SQL Injection",
			Severity:    "high",
			File:        "api/handler.go",
			Line:        42,
			Column:      3,
			Description: "Untrusted input used in SQL query",
			CWE:         "CWE-89",
			OWASP:       "A03:2021 – Injection",
			Remediation: "Use parameterized queries",
		},
		{
			RuleID:      "SECRETS-001",
			RuleName:    "Hardcoded API Key",
			Severity:    "critical",
			File:        "config/app.yaml",
			Line:        10,
			Description: "Possible API key detected",
		},
	}

	out, err := toSarif(findings)
	if err != nil {
		t.Fatalf("toSarif: %v", err)
	}

	var res sarifLog
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal SARIF: %v\n%s", err, out)
	}

	if res.Version != "2.1.0" {
		t.Errorf("SARIF version: got %q, want 2.1.0", res.Version)
	}
	if len(res.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(res.Runs))
	}
	run := res.Runs[0]
	if run.Tool.Driver.Name != "sin-code security" {
		t.Errorf("tool name: got %q, want sin-code security", run.Tool.Driver.Name)
	}
	if len(run.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(run.Results))
	}
	if len(run.Tool.Driver.Rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(run.Tool.Driver.Rules))
	}

	first := run.Results[0]
	if first.RuleID != "SAST-001" {
		t.Errorf("result rule id: got %q, want SAST-001", first.RuleID)
	}
	if first.Level != "error" {
		t.Errorf("result level: got %q, want error", first.Level)
	}
	if len(first.Locations) != 1 {
		t.Fatalf("expected 1 location, got %d", len(first.Locations))
	}
	loc := first.Locations[0].PhysicalLocation
	if loc.ArtifactLocation.URI != "api/handler.go" {
		t.Errorf("artifact uri: got %q", loc.ArtifactLocation.URI)
	}
	if loc.Region.StartLine != 42 {
		t.Errorf("start line: got %d, want 42", loc.Region.StartLine)
	}
	if loc.Region.StartColumn != 3 {
		t.Errorf("start column: got %d, want 3", loc.Region.StartColumn)
	}
	if !strings.Contains(first.Message.Text, "Untrusted input") {
		t.Errorf("message text missing finding message: %q", first.Message.Text)
	}
}

func TestWriteSarif(t *testing.T) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)

	findings := []SecurityFinding{
		{
			RuleID:      "SAST-002",
			RuleName:    "Insecure Random",
			Severity:    "medium",
			File:        "utils/rand.go",
			Line:        15,
			Description: "math/rand used for security-sensitive operation",
		},
	}

	if err := writeSarif(cmd, findings); err != nil {
		t.Fatalf("writeSarif: %v", err)
	}

	var sarif sarifLog
	if err := json.Unmarshal(buf.Bytes(), &sarif); err != nil {
		t.Fatalf("unmarshal SARIF: %v\n%s", err, buf.String())
	}
	if len(sarif.Runs) != 1 || len(sarif.Runs[0].Results) != 1 {
		t.Fatalf("expected 1 run with 1 result")
	}
	if sarif.Runs[0].Results[0].RuleID != "SAST-002" {
		t.Errorf("rule id mismatch: %q", sarif.Runs[0].Results[0].RuleID)
	}
}

func TestSASTFindingToSecurity(t *testing.T) {
	f := sastFindingToSecurity(sastScanFinding{
		RuleID:      "GO-001",
		RuleName:    "Hardcoded Credentials",
		Severity:    "high",
		File:        "main.go",
		Line:        5,
		Column:      1,
		Description: "credential found",
		CWE:         "CWE-798",
		OWASP:       "A07",
		Remediation: "use env vars",
	})
	if f.RuleID != "GO-001" || f.Severity != "high" || f.Line != 5 || f.CWE != "CWE-798" {
		t.Errorf("unexpected sastFindingToSecurity result: %+v", f)
	}
	if f.Kind != "sast" || f.Tool != "sin-sast" {
		t.Errorf("unexpected kind/tool: %+v", f)
	}
}

func TestSecretsFindingToSecurity(t *testing.T) {
	f := secretsFindingToSecurity(secretsScanFinding{
		RuleID:   "SECRETS-API-KEY",
		RuleName: "API Key",
		Severity: "critical",
		File:     "keys.env",
		Line:     2,
		Match:    "sk-12345",
	})
	if f.RuleID != "SECRETS-API-KEY" || f.Severity != "critical" || f.Line != 2 || f.File != "keys.env" {
		t.Errorf("unexpected secretsFindingToSecurity result: %+v", f)
	}
	if f.Kind != "secret" || f.Tool != "sin-secrets" {
		t.Errorf("unexpected kind/tool: %+v", f)
	}
	if f.Match == "sk-12345" {
		t.Errorf("expected masked match, got %q", f.Match)
	}
}

func TestToolResultToSecurityFinding(t *testing.T) {
	tr := ToolResult{Name: "sast", Status: "issues", Issues: 3}
	result := toolResultToSecurityFinding("/base", tr, 0)
	if result.Severity != "high" {
		t.Errorf("expected severity high for issues, got %q", result.Severity)
	}
	if result.File != "/base" {
		t.Errorf("expected file /base, got %q", result.File)
	}
	if !strings.Contains(result.RuleID, "SAST") {
		t.Errorf("expected rule id to contain SAST, got %q", result.RuleID)
	}
}

func TestToolResultToSecurityFindingError(t *testing.T) {
	tr := ToolResult{Name: "sast", Status: "error", Error: "scanner crashed"}
	result := toolResultToSecurityFinding("/base", tr, 0)
	if result.Severity != "warning" {
		t.Errorf("expected severity warning for error, got %q", result.Severity)
	}
	if result.Kind != "error" {
		t.Errorf("expected kind error, got %q", result.Kind)
	}
}

func TestSarifJSON(t *testing.T) {
	findings := []SecurityFinding{{RuleID: "R1", Severity: "high", File: "a.go", Line: 1, Description: "m"}}
	out, err := toSarif(findings)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Contains(out, []byte(`"version": "2.1.0"`)) {
		t.Errorf("SARIF output missing version: %s", out)
	}
	if !bytes.Contains(out, []byte(`"ruleId": "R1"`)) {
		t.Errorf("SARIF output missing ruleId: %s", out)
	}
}

func TestSarifEmptyFindings(t *testing.T) {
	out, err := toSarif([]SecurityFinding{})
	if err != nil {
		t.Fatalf("toSarif empty: %v", err)
	}
	var res sarifLog
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(res.Runs) != 1 {
		t.Fatalf("expected 1 run")
	}
	if len(res.Runs[0].Results) != 0 {
		t.Errorf("expected 0 results, got %d", len(res.Runs[0].Results))
	}
	if len(res.Runs[0].Tool.Driver.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(res.Runs[0].Tool.Driver.Rules))
	}
}

func TestSarifRuleMetadata(t *testing.T) {
	f := SecurityFinding{
		RuleID:      "R2",
		RuleName:    "Unescaped Input",
		Severity:    "high",
		CWE:         "CWE-20",
		OWASP:       "A03:2021",
		Remediation: "sanitize",
		Description: "Unescaped user input reaches sink",
	}
	out, err := toSarif([]SecurityFinding{f})
	if err != nil {
		t.Fatalf("toSarif: %v", err)
	}
	var res sarifLog
	if err := json.Unmarshal(out, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	rule := res.Runs[0].Tool.Driver.Rules[0]
	if rule.ID != "R2" {
		t.Errorf("rule id: got %q", rule.ID)
	}
	if rule.Name != "Unescaped Input" {
		t.Errorf("rule name: got %q", rule.Name)
	}
	if rule.ShortDescription == nil || !strings.Contains(rule.ShortDescription.Text, "Unescaped user input") {
		t.Errorf("rule short description missing: %+v", rule.ShortDescription)
	}
	if rule.Properties["cwe"] != "CWE-20" {
		t.Errorf("expected CWE property, got %v", rule.Properties["cwe"])
	}
	if rule.Properties["owasp"] != "A03:2021" {
		t.Errorf("expected OWASP property, got %v", rule.Properties["owasp"])
	}
	if rule.Properties["remediation"] != "sanitize" {
		t.Errorf("expected remediation property, got %v", rule.Properties["remediation"])
	}
}

func TestSecurityFindingMessageText(t *testing.T) {
	f := SecurityFinding{Description: "foo"}
	if f.MessageText() != "foo" {
		t.Errorf("description precedence failed: %q", f.MessageText())
	}
	f2 := SecurityFinding{RuleName: "Rule", Context: "context", File: "a.go", Line: 3}
	if !strings.Contains(f2.MessageText(), "Rule") {
		t.Errorf("message missing rule name: %q", f2.MessageText())
	}
	if !strings.Contains(f2.MessageText(), "a.go:3") {
		t.Errorf("message missing location: %q", f2.MessageText())
	}
}

func TestMaskSecuritySecret(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"short", "*****"},
		{"sk-1234567890abcdef", "sk-1***********cdef"},
		{"", ""},
		{"12345678", "********"},
		{"123456789", "1234*6789"},
	}
	for _, c := range cases {
		got := maskSecuritySecret(c.in)
		if got != c.want {
			t.Errorf("maskSecuritySecret(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
