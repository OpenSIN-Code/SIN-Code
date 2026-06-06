package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/models"
	"github.com/OpenSIN-Code/SIN-Code-Secrets-Scanner/pkg/rules"
)

func TestNewEngine(t *testing.T) {
	r := rules.AllRules()
	eng := NewEngine(r, []string{"test.go"}, true)
	if eng == nil {
		t.Fatal("expected engine to be non-nil")
	}
	if len(eng.Rules) != len(r) {
		t.Fatalf("expected %d rules, got %d", len(r), len(eng.Rules))
	}
	if !eng.CheckEntropy {
		t.Fatal("expected entropy check to be enabled")
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	eng := NewEngine(rules.AllRules(), []string{}, false)
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != "passed" {
		t.Fatalf("expected status passed, got %s", result.Status)
	}
	if result.Summary.FilesScanned != 0 {
		t.Fatalf("expected 0 files scanned, got %d", result.Summary.FilesScanned)
	}
}

func TestScanWithOpenAIKey(t *testing.T) {
	tmpDir := t.TempDir()

	content := `import openai

openai.api_key = "sk-1234567890abcdef1234567890abcdef"
response = openai.ChatCompletion.create(model="gpt-4", messages=[{"role": "user", "content": "Hello"}])
`
	filePath := filepath.Join(tmpDir, "config.py")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(rules.AllRules(), []string{}, true)
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	foundOpenAI := false
	for _, f := range result.Findings {
		if f.RuleID == "SECRETS-001" {
			foundOpenAI = true
			if f.Severity != "critical" {
				t.Fatalf("expected critical severity, got %s", f.Severity)
			}
		}
	}
	if !foundOpenAI {
		t.Fatal("expected OpenAI API key finding")
	}
}

func TestScanWithAWSKey(t *testing.T) {
	tmpDir := t.TempDir()

	content := `[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
`
	filePath := filepath.Join(tmpDir, "aws_credentials")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(rules.AllRules(), []string{}, false)
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	foundAWS := false
	for _, f := range result.Findings {
		if f.RuleID == "SECRETS-002" {
			foundAWS = true
		}
	}
	if !foundAWS {
		t.Fatal("expected AWS Access Key finding")
	}
}

func TestScanWithPassword(t *testing.T) {
	tmpDir := t.TempDir()

	content := `password = "SuperSecret123!"
DB_PASS = "another_password_here"
`
	filePath := filepath.Join(tmpDir, "settings.py")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(rules.AllRules(), []string{}, false)
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	foundPassword := false
	for _, f := range result.Findings {
		if f.RuleID == "SECRETS-014" {
			foundPassword = true
		}
	}
	if !foundPassword {
		t.Fatal("expected database password finding")
	}
}

func TestCalculateEntropy(t *testing.T) {
	// High entropy string
	high := calculateEntropy("abcdefghijklmnopqrstuvwxyz")
	if high < 4.0 {
		t.Fatalf("expected high entropy, got %.2f", high)
	}

	// Low entropy string
	low := calculateEntropy("aaaaaaaaaaaaaaaa")
	if low > 1.0 {
		t.Fatalf("expected low entropy, got %.2f", low)
	}

	// Empty string
	empty := calculateEntropy("")
	if empty != 0 {
		t.Fatalf("expected 0 entropy for empty string, got %.2f", empty)
	}
}

func TestBuildSummary(t *testing.T) {
	findings := []models.SecretFinding{
		{Severity: "critical", SecretType: "api-key"},
		{Severity: "high", SecretType: "token"},
		{Severity: "medium", SecretType: "password"},
	}

	summary := buildSummary(findings, 15)

	if summary.Critical != 1 {
		t.Fatalf("expected 1 critical, got %d", summary.Critical)
	}
	if summary.High != 1 {
		t.Fatalf("expected 1 high, got %d", summary.High)
	}
	if summary.Medium != 1 {
		t.Fatalf("expected 1 medium, got %d", summary.Medium)
	}
	if summary.FilesScanned != 15 {
		t.Fatalf("expected 15 files, got %d", summary.FilesScanned)
	}
	if summary.SecretsFound != 3 {
		t.Fatalf("expected 3 secrets, got %d", summary.SecretsFound)
	}
	if summary.ByType["api-key"] != 1 {
		t.Fatalf("expected 1 api-key, got %d", summary.ByType["api-key"])
	}
}
