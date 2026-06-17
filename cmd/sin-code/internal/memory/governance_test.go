// SPDX-License-Identifier: MIT
// Purpose: tests for governance capture (issue #354). Covers secret
// detection, policy detection, approval detection, redaction,
// criticality classification, and summary formatting.
package memory

import (
	"strings"
	"sync"
	"testing"
)

func TestGovernanceScanDetectsOpenAIKey(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("the key is sk-abcdefghijklmnopqrstuvwxyz1234567890")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	if findings[0].Type != GovSecret {
		t.Errorf("expected secret type, got %s", findings[0].Type)
	}
	if !strings.Contains(findings[0].Match, "sk-") {
		t.Errorf("match should contain sk-: %s", findings[0].Match)
	}
}

func TestGovernanceScanDetectsAWSKey(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("AWS key: AKIAIOSFODNN7EXAMPLE")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	found := false
	for _, f := range findings {
		if f.Type == GovSecret && strings.Contains(f.Match, "AKIA") {
			found = true
		}
	}
	if !found {
		t.Error("expected AWS key secret finding")
	}
}

func TestGovernanceScanDetectsGitHubToken(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("token: ghp_1234567890abcdefghijklmnopqrstuvwxyz1234")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Match, "ghp_") {
			found = true
		}
	}
	if !found {
		t.Error("expected GitHub token finding")
	}
}

func TestGovernanceScanDetectsSlackToken(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("xoxb-1234567890-abcdefghij")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	found := false
	for _, f := range findings {
		if strings.Contains(f.Match, "xox") {
			found = true
		}
	}
	if !found {
		t.Error("expected Slack token finding")
	}
}

func TestGovernanceScanDetectsPolicy(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("this change requires approval from the team lead")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	found := false
	for _, f := range findings {
		if f.Type == GovPolicy {
			found = true
		}
	}
	if !found {
		t.Error("expected policy finding")
	}
}

func TestGovernanceScanDetectsApproval(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("approved by alice on 2026-01-15")
	if len(findings) == 0 {
		t.Fatal("expected at least 1 finding")
	}
	found := false
	for _, f := range findings {
		if f.Type == GovApproval {
			found = true
		}
	}
	if !found {
		t.Error("expected approval finding")
	}
}

func TestGovernanceScanCleanContent(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("this is a normal code review with no secrets")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for clean content, got %d", len(findings))
	}
}

func TestGovernanceIsCritical(t *testing.T) {
	g := NewGovernanceCapture()
	secret := GovernanceFinding{Type: GovSecret, Severity: SeverityCritical}
	policy := GovernanceFinding{Type: GovPolicy, Severity: SeverityMedium}
	approval := GovernanceFinding{Type: GovApproval, Severity: SeverityLow}
	highPwd := GovernanceFinding{Type: GovSecret, Severity: SeverityHigh}
	if !g.IsCritical(secret) {
		t.Error("secret should be critical")
	}
	if !g.IsCritical(highPwd) {
		t.Error("high severity should be critical")
	}
	if g.IsCritical(policy) {
		t.Error("medium policy should not be critical")
	}
	if g.IsCritical(approval) {
		t.Error("low approval should not be critical")
	}
}

func TestGovernanceScanMultipleFindings(t *testing.T) {
	g := NewGovernanceCapture()
	content := "key=sk-abcdefghijklmnopqrstuvwxyz1234567890 requires approval, approved by bob"
	findings := g.Scan(content)
	if len(findings) < 3 {
		t.Fatalf("expected >= 3 findings, got %d", len(findings))
	}
	types := map[GovernanceFindingType]bool{}
	for _, f := range findings {
		types[f.Type] = true
	}
	if !types[GovSecret] {
		t.Error("expected secret finding")
	}
	if !types[GovPolicy] {
		t.Error("expected policy finding")
	}
	if !types[GovApproval] {
		t.Error("expected approval finding")
	}
}

func TestGovernanceRedact(t *testing.T) {
	g := NewGovernanceCapture()
	original := "key is sk-abcdefghijklmnopqrstuvwxyz1234567890 and AKIAIOSFODNN7EXAMPLE"
	redacted := g.Redact(original)
	if strings.Contains(redacted, "sk-abcdef") {
		t.Error("OpenAI key should be redacted")
	}
	if strings.Contains(redacted, "AKIA") {
		t.Error("AWS key should be redacted")
	}
	if !strings.Contains(redacted, "[REDACTED:") {
		t.Error("redacted content should contain REDACTED marker")
	}
}

func TestGovernanceHasSecret(t *testing.T) {
	g := NewGovernanceCapture()
	if !g.HasSecret("sk-abcdefghijklmnopqrstuvwxyz1234567890") {
		t.Error("HasSecret should return true for secret content")
	}
	if g.HasSecret("clean content") {
		t.Error("HasSecret should return false for clean content")
	}
}

func TestGovernanceSummary(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("sk-abcdefghijklmnopqrstuvwxyz1234567890 requires approval")
	summary := g.Summary(findings)
	if !strings.Contains(summary, "secret") {
		t.Errorf("summary should mention secret: %s", summary)
	}
	if !strings.Contains(summary, "critical") {
		t.Errorf("summary should mention critical: %s", summary)
	}
	empty := g.Summary(nil)
	if empty != "no governance findings" {
		t.Errorf("empty summary: %s", empty)
	}
}

func TestGovernanceScanRaceFree(t *testing.T) {
	g := NewGovernanceCapture()
	content := "sk-abcdefghijklmnopqrstuvwxyz1234567890 requires approval"
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = g.Scan(content)
			_ = g.Redact(content)
			_ = g.HasSecret(content)
		}()
	}
	wg.Wait()
}

func TestGovernanceDetectsPrivateKey(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAKCAQEA...")
	if len(findings) == 0 {
		t.Fatal("expected private key detection")
	}
	if findings[0].Type != GovSecret {
		t.Errorf("expected secret type, got %s", findings[0].Type)
	}
}

func TestGovernanceDetectsPasswordAssignment(t *testing.T) {
	g := NewGovernanceCapture()
	findings := g.Scan("password = mysecret1234")
	if len(findings) == 0 {
		t.Fatal("expected password detection")
	}
	found := false
	for _, f := range findings {
		if f.Type == GovSecret {
			found = true
		}
	}
	if !found {
		t.Error("expected secret finding for password assignment")
	}
}
