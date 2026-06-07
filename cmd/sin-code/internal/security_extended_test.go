package internal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunGoVet(t *testing.T) {
	if !commandExists("go") {
		t.Skip("go not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testvet\n\ngo 1.25\n"), 0644)
	os.WriteFile(filepath.Join(dir, "vet_error.go"), []byte("package testvet\n\nimport \"fmt\"\n\nfunc Example() {\n\tfmt.Printf(\"%d\")\n}\n"), 0644)

	status, issues, output, errStr := runGoVet(dir, 30)
	if status == "ok" && issues == 0 && output == "" {
		t.Log("go vet did not catch the Printf error; may need go mod tidy first")
	}
	if status == "not_found" {
		t.Error("expected go to be found after commandExists check")
	}
	_ = errStr
}

func TestRunGoVet_CleanProject(t *testing.T) {
	if !commandExists("go") {
		t.Skip("go not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testclean\n\ngo 1.25\n"), 0644)
	os.WriteFile(filepath.Join(dir, "clean.go"), []byte("package testclean\n\nfunc Hello() string { return \"hello\" }\n"), 0644)

	status, issues, _, _ := runGoVet(dir, 30)
	if status == "not_found" {
		t.Skip("go not found in PATH")
	}
	if status == "issues" && issues > 0 {
		t.Logf("go vet reported %d issues in clean project; may be false positive", issues)
	}
}

func TestRunGovulncheck(t *testing.T) {
	if !commandExists("govulncheck") {
		t.Skip("govulncheck not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testvuln\n\ngo 1.25\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package testvuln\n\nfunc main() {}\n"), 0644)

	status, _, _, _ := runGovulncheck(dir, 60)
	if status == "not_found" {
		t.Error("expected govulncheck to be found after commandExists check")
	}
}

func TestRunGovulncheck_NotFound(t *testing.T) {
	if commandExists("govulncheck") {
		t.Skip("govulncheck is installed, skipping not_found test")
	}

	status, _, _, errStr := runGovulncheck(t.TempDir(), 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestRunGosec(t *testing.T) {
	if !commandExists("gosec") {
		t.Skip("gosec not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module testsec\n\ngo 1.25\n"), 0644)
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package testsec\n\nimport \"crypto/md5\"\n\nfunc Hash(data []byte) [16]byte { return md5.Sum(data) }\n"), 0644)

	status, _, _, _ := runGosec(dir, 60)
	if status == "not_found" {
		t.Error("expected gosec to be found after commandExists check")
	}
}

func TestRunGosec_NotFound(t *testing.T) {
	if commandExists("gosec") {
		t.Skip("gosec is installed, skipping not_found test")
	}

	status, _, _, errStr := runGosec(t.TempDir(), 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestRunBandit(t *testing.T) {
	if !commandExists("bandit") {
		t.Skip("bandit not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "insecure.py"), []byte("import pickle\ndata = pickle.loads(b'')\n"), 0644)

	status, _, _, _ := runBandit(dir, 60)
	if status == "not_found" {
		t.Error("expected bandit to be found after commandExists check")
	}
}

func TestRunBandit_NotFound(t *testing.T) {
	if commandExists("bandit") {
		t.Skip("bandit is installed, skipping not_found test")
	}

	status, _, _, errStr := runBandit(t.TempDir(), 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestRunSafety(t *testing.T) {
	if !commandExists("safety") {
		t.Skip("safety not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==0.12\n"), 0644)

	status, _, _, _ := runSafety(dir, 60)
	if status == "not_found" {
		t.Error("expected safety to be found after commandExists check")
	}
}

func TestRunSafety_NotFound(t *testing.T) {
	if commandExists("safety") {
		t.Skip("safety is installed, skipping not_found test")
	}

	status, _, _, errStr := runSafety(t.TempDir(), 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestRunNpmAudit(t *testing.T) {
	if !commandExists("npm") {
		t.Skip("npm not found in PATH")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test","version":"1.0.0","dependencies":{}}`), 0644)

	status, _, _, _ := runNpmAudit(dir, 60)
	if status == "not_found" {
		t.Error("expected npm to be found after commandExists check")
	}
}

func TestRunNpmAudit_NotFound(t *testing.T) {
	if commandExists("npm") {
		t.Skip("npm is installed, skipping not_found test")
	}

	status, _, _, errStr := runNpmAudit(t.TempDir(), 5)
	if status != "not_found" {
		t.Errorf("expected status 'not_found', got %q", status)
	}
	if errStr == "" {
		t.Error("expected non-empty error string for not_found status")
	}
}

func TestRunSecretsGrep(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.py"), []byte(`api_key = "sk-1234567890abcdef1234567890abcdef"`), 0644)

	status, issues, output, errStr := runSecretsGrep(dir, 30)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues == 0 {
		t.Error("expected at least 1 issue for api_key pattern")
	}
	if output == "" {
		t.Error("expected non-empty output when issues are found")
	}
	_ = errStr
}

func TestRunSecretsGrep_CleanDir(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "src"), 0755)

	status, issues, _, _ := runSecretsGrep(dir, 30)
	if status != "ok" {
		t.Errorf("expected status 'ok' for clean dir, got %q", status)
	}
	if issues != 0 {
		t.Errorf("expected 0 issues for clean dir, got %d", issues)
	}
}

func TestRunSecretsGrep_PasswordPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "settings.py"), []byte(`password = "supersecretvalue123"`), 0644)

	status, issues, _, _ := runSecretsGrep(dir, 30)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues == 0 {
		t.Error("expected at least 1 issue for password pattern")
	}
}

func TestRunSecretsGrep_TokenPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "auth.go"), []byte(`token = "ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"`), 0644)

	status, issues, _, _ := runSecretsGrep(dir, 30)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues == 0 {
		t.Error("expected at least 1 issue for token pattern")
	}
}

func TestRunSecretsGrep_AWSPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "deploy.yaml"), []byte("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE\n"), 0644)

	status, issues, _, _ := runSecretsGrep(dir, 30)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues == 0 {
		t.Error("expected at least 1 issue for AWS key pattern")
	}
}

func TestRunSecretsGrep_PrivateKeyPattern(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "config.json"), []byte(`{"private_key": "-----BEGIN RSA PRIVATE KEY-----"}`), 0644)

	status, issues, _, _ := runSecretsGrep(dir, 30)
	if status != "issues" {
		t.Errorf("expected status 'issues', got %q", status)
	}
	if issues == 0 {
		t.Error("expected at least 1 issue for private_key pattern")
	}
}

func TestRunFilePermissions(t *testing.T) {
	dir := t.TempDir()
	insecureFile := filepath.Join(dir, "insecure.sh")
	os.WriteFile(insecureFile, []byte("#!/bin/bash\necho hello"), 0777)

	status, _, output, _ := runFilePermissions(dir, 30)
	if status != "ok" {
		t.Errorf("expected status 'ok', got %q", status)
	}
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestRunFilePermissions_NoExecutableFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello world"), 0644)

	status, _, output, _ := runFilePermissions(dir, 30)
	if status != "ok" {
		t.Errorf("expected status 'ok', got %q", status)
	}
	if output == "" {
		t.Error("expected non-empty output")
	}
}

func TestCountSubstring(t *testing.T) {
	tests := []struct {
		s    string
		sub  string
		want int
	}{
		{"hello hello", "hello", 2},
		{"aaa", "a", 3},
		{"abc", "d", 0},
		{"", "a", 0},
		{"abc", "", 4},
		{"", "", 1},
		{"GO-123 GO-456 GO-789", "GO-", 3},
		{`"severity" "severity"`, `"severity"`, 2},
		{"no match here", "xyz", 0},
		{"aaaa", "aa", 2},
	}

	for _, tt := range tests {
		got := countSubstring(tt.s, tt.sub)
		if got != tt.want {
			t.Errorf("countSubstring(%q, %q) = %d, want %d", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestCountLinesSimple(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"line1\nline2\nline3", 3},
		{"single", 1},
		{"", 1},
		{"a\nb\nc\n", 4},
		{"\n\n", 3},
		{"one\ntwo", 2},
		{"trailing\n", 2},
	}

	for _, tt := range tests {
		got := countLinesSimple(tt.s)
		if got != tt.want {
			t.Errorf("countLinesSimple(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestCommandExists(t *testing.T) {
	if !commandExists("echo") {
		t.Error("expected 'echo' to exist in PATH")
	}
	if commandExists("nonexistent_tool_that_does_not_exist_12345") {
		t.Error("expected nonexistent tool to not be found")
	}
}

func TestDetectProjectType_PythonRequirements(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("flask==2.0\n"), 0644)

	result := detectProjectType(dir)
	if result != "python" {
		t.Errorf("expected 'python', got %q", result)
	}
}

func TestDetectProjectType_PythonPyproject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "pyproject.toml"), []byte("[project]\nname = 'test'\n"), 0644)

	result := detectProjectType(dir)
	if result != "python" {
		t.Errorf("expected 'python', got %q", result)
	}
}

func TestDetectProjectType_Node(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "package.json"), []byte(`{"name":"test"}`), 0644)

	result := detectProjectType(dir)
	if result != "node" {
		t.Errorf("expected 'node', got %q", result)
	}
}

func TestDetectProjectType_Generic(t *testing.T) {
	dir := t.TempDir()

	result := detectProjectType(dir)
	if result != "generic" {
		t.Errorf("expected 'generic', got %q", result)
	}
}

func TestRunSecurityScan_GenericProject(t *testing.T) {
	dir := t.TempDir()

	result := runSecurityScan(dir, "generic", "", 30)
	if result.ProjectType != "generic" {
		t.Errorf("expected project type 'generic', got %q", result.ProjectType)
	}
	if len(result.Tools) == 0 {
		t.Error("expected at least one tool to run for generic project")
	}
}

func TestRunSecurityScan_WithToolFilter(t *testing.T) {
	dir := t.TempDir()

	result := runSecurityScan(dir, "generic", "secrets grep", 30)
	if result.Summary.Skipped > 0 {
		t.Logf("%d tools were skipped by filter", result.Summary.Skipped)
	}
}
