// SPDX-License-Identifier: MIT
package engine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/models"
	"github.com/OpenSIN-Code/SIN-Code-SAST-Tool/pkg/rules"
)

func TestNewEngine(t *testing.T) {
	r := rules.AllRules()
	eng := NewEngine(r, []string{"test.go"})
	if eng == nil {
		t.Fatal("expected engine to be non-nil")
	}
	if len(eng.Rules) != len(r) {
		t.Fatalf("expected %d rules, got %d", len(r), len(eng.Rules))
	}
}

func TestScanEmptyDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	eng := NewEngine(rules.AllRules(), []string{})
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

func TestScanWithSQLInjection(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a vulnerable Python file
	content := `import sqlite3

def get_user(username):
    conn = sqlite3.connect('db.sqlite')
    cursor = conn.cursor()
    query = f"SELECT * FROM users WHERE username = '{username}'"
    cursor.execute(query)
    return cursor.fetchone()
`
	filePath := filepath.Join(tmpDir, "vulnerable.py")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(rules.AllRules(), []string{})
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Status != "failed" && result.Status != "warning" {
		t.Fatalf("expected status failed or warning, got %s", result.Status)
	}
	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	foundSQLInjection := false
	for _, f := range result.Findings {
		if f.RuleID == "SAST-001" || f.RuleID == "SAST-002" {
			foundSQLInjection = true
		}
	}
	if !foundSQLInjection {
		t.Fatal("expected SQL injection finding")
	}
}

func TestScanWithHardcodedSecret(t *testing.T) {
	tmpDir := t.TempDir()

	content := `api_key = "1234567890abcdef1234567890abcdef"
API_KEY = "1234567890abcdef1234567890abcdef"
`
	filePath := filepath.Join(tmpDir, "secrets.py")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	eng := NewEngine(rules.AllRules(), []string{})
	result, err := eng.Scan(tmpDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Findings) == 0 {
		t.Fatal("expected findings, got none")
	}

	foundSecret := false
	for _, f := range result.Findings {
		if f.RuleID == "SAST-006" {
			foundSecret = true
		}
	}
	if !foundSecret {
		t.Fatal("expected hardcoded secret finding")
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		file     string
		expected string
	}{
		{"test.py", "python"},
		{"test.js", "javascript"},
		{"test.ts", "typescript"},
		{"test.go", "go"},
		{"test.java", "java"},
		{"test.php", "php"},
		{"test.rb", "ruby"},
		{"test.yaml", "yaml"},
		{"test.json", "json"},
	}
	for _, tt := range tests {
		result := detectLanguage(tt.file)
		if result != tt.expected {
			t.Fatalf("expected %s for %s, got %s", tt.expected, tt.file, result)
		}
	}
}

func TestBuildSummary(t *testing.T) {
	findings := []models.SASTFinding{
		{Severity: "critical", Language: "python", OWASP: "A03:2021 - Injection"},
		{Severity: "high", Language: "python", OWASP: "A03:2021 - Injection"},
		{Severity: "medium", Language: "go", OWASP: "A02:2021 - Cryptographic Failures"},
	}

	summary := buildSummary(findings, 10, 500)

	if summary.Critical != 1 {
		t.Fatalf("expected 1 critical, got %d", summary.Critical)
	}
	if summary.High != 1 {
		t.Fatalf("expected 1 high, got %d", summary.High)
	}
	if summary.Medium != 1 {
		t.Fatalf("expected 1 medium, got %d", summary.Medium)
	}
	if summary.FilesScanned != 10 {
		t.Fatalf("expected 10 files, got %d", summary.FilesScanned)
	}
	if summary.LinesScanned != 500 {
		t.Fatalf("expected 500 lines, got %d", summary.LinesScanned)
	}
	if summary.ByLanguage["python"] != 2 {
		t.Fatalf("expected 2 python findings, got %d", summary.ByLanguage["python"])
	}
	if summary.ByLanguage["go"] != 1 {
		t.Fatalf("expected 1 go finding, got %d", summary.ByLanguage["go"])
	}
}

func TestContains(t *testing.T) {
	if !contains([]string{"a", "b", "c"}, "b") {
		t.Fatal("expected contains to return true")
	}
	if contains([]string{"a", "b", "c"}, "d") {
		t.Fatal("expected contains to return false")
	}
}

func TestIsExcluded(t *testing.T) {
	eng := NewEngine([]models.Rule{}, []string{"test.go", "*.tmp"})
	if !eng.isExcluded("/path/test.go") {
		t.Fatal("expected test.go to be excluded")
	}
	if eng.isExcluded("/path/main.py") {
		t.Fatal("expected main.py not to be excluded")
	}
}
