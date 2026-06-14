// SPDX-License-Identifier: MIT
// Purpose: Additional unit tests for small internal helpers that were
// uncovered as of st-cov1. (st-cov1)
package internal

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/plugins"
)

func TestStringJoin(t *testing.T) {
	if got := stringJoin([]string{"a", "b", "c"}, ","); got != "a,b,c" {
		t.Errorf("stringJoin = %q, want %q", got, "a,b,c")
	}
	if got := stringJoin(nil, ","); got != "" {
		t.Errorf("stringJoin(nil) = %q, want empty", got)
	}
}

func TestIsHashCommentLang(t *testing.T) {
	for _, ext := range []string{".py", ".rb", ".sh", ".bash", ".yaml", ".yml", ".toml", ".pl", ".r"} {
		if !isHashCommentLang("file" + ext) {
			t.Errorf("isHashCommentLang(%q) = false, want true", ext)
		}
	}
	for _, ext := range []string{".go", ".js", ".ts", ".rs", ".java", ".c"} {
		if isHashCommentLang("file" + ext) {
			t.Errorf("isHashCommentLang(%q) = true, want false", ext)
		}
	}
}

func TestCheckBracketBalance(t *testing.T) {
	tests := []struct {
		name    string
		content string
		path    string
		wantErr bool
	}{
		{"balanced", "func foo() { bar(); }", "x.go", false},
		{"unbalanced", "func foo() { bar();", "x.go", true},
		{"unexpected close", "func foo() }", "x.go", true},
		{"string ignored", `x := "(){}"`, "x.go", false},
		{"single quote ignored", "x := '(){}'", "x.go", false},
		{"backtick ignored", "x := `(){}`", "x.go", false},
		{"python comment ignored", "x = 1 # (\n", "x.py", false},
		{"empty", "", "x.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkBracketBalance(tt.path, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkBracketBalance() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDrainMatches_StCov1(t *testing.T) {
	ch := make(chan match, 3)
	ch <- match{results: []scoutResult{{File: "a"}}}
	ch <- match{results: []scoutResult{{File: "b"}}}
	ch <- match{err: nil}
	close(ch)
	drainMatches(ch)
}

func TestRelOf(t *testing.T) {
	wd, _ := os.Getwd()
	abs := filepath.Join(wd, "foo.go")
	if got := relOf(abs); got != "foo.go" {
		t.Errorf("relOf(%q) = %q, want %q", abs, got, "foo.go")
	}
}

func TestSearchSingleFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0o644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := searchSingleFile(p, "Hello", "regex", 10, "text")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("searchSingleFile failed: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "Hello") {
		t.Errorf("expected output to contain 'Hello', got %q", string(out))
	}
}

func TestPluginDir(t *testing.T) {
	old := pluginPath
	pluginPath = "/tmp/custom-plugins"
	defer func() { pluginPath = old }()

	if got := pluginDir(); got != "/tmp/custom-plugins" {
		t.Errorf("pluginDir() = %q, want %q", got, "/tmp/custom-plugins")
	}
}

func TestLoadPlugin(t *testing.T) {
	oldPluginPath := pluginPath
	defer func() { pluginPath = oldPluginPath }()

	dir := t.TempDir()
	pluginPath = dir
	pluginDir := filepath.Join(dir, "test-plugin")
	os.MkdirAll(pluginDir, 0o755)
	manifest := `name = "test-plugin"
version = "1.0.0"
`
	os.WriteFile(filepath.Join(pluginDir, plugins.ManifestFile), []byte(manifest), 0o644)

	p, err := loadPlugin("test-plugin")
	if err != nil {
		t.Fatalf("loadPlugin failed: %v", err)
	}
	if p.Name != "test-plugin" {
		t.Errorf("loadPlugin name = %q, want %q", p.Name, "test-plugin")
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n"), 0o644)

	res, err := readFile(p, "raw", 1, 10, 0)
	if err != nil {
		t.Fatalf("readFile failed: %v", err)
	}
	if res.Path != p {
		t.Errorf("readFile path = %q, want %q", res.Path, p)
	}
	if res.TotalLines != 5 {
		t.Errorf("readFile total_lines = %d, want 5", res.TotalLines)
	}
	if !strings.Contains(res.Content, "Hello") {
		t.Errorf("readFile content missing 'Hello': %q", res.Content)
	}
}

func TestBuildOutlineResult(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\n\nfunc Hello() string {\n\treturn \"hello\"\n}\n"), 0o644)

	res, err := readFile(p, "outline", 1, 10, 0)
	if err != nil {
		t.Fatalf("readFile outline failed: %v", err)
	}
	if res == nil {
		t.Fatal("readFile outline returned nil")
	}
	if !strings.Contains(res.Content, "symbols") {
		t.Errorf("outline content missing 'symbols': %q", res.Content)
	}
}

func TestOutputTextPOC(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	result := &pocResult{
		Spec:   "spec.md",
		Code:   "code.go",
		Passed: 1,
		Failed: 1,
		Checks: []pocCheck{
			{Name: "Hello", Type: "required", Status: "pass", Message: "found", File: "code.go", Line: 1},
			{Name: "World", Type: "required", Status: "fail", Message: "missing"},
			{Name: "TODO", Type: "forbidden", Status: "warn", Message: "warn"},
		},
		Summary: "Coverage: 50.0%",
	}
	result.Coverage = 50.0
	if err := outputTextPOC(result); err != nil {
		t.Fatal(err)
	}

	w.Close()
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "Proof-of-Correctness") {
		t.Errorf("expected output header, got %q", string(out))
	}
	if !strings.Contains(string(out), "Hello") || !strings.Contains(string(out), "World") {
		t.Errorf("expected output to contain checks, got %q", string(out))
	}
}

func TestSearchSingleFile_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "test.go")
	os.WriteFile(p, []byte("package main\nfunc Hello() {}\n"), 0o644)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := searchSingleFile(p, "Hello", "regex", 10, "json")

	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("searchSingleFile json failed: %v", err)
	}
	out, _ := io.ReadAll(r)
	if !strings.Contains(string(out), "Hello") {
		t.Errorf("expected JSON output to contain 'Hello', got %q", string(out))
	}
}

func TestLoadAllAgents_WithPlugin(t *testing.T) {
	oldNoPlugins := orch2NoPlugins
	oldAgentsDir := orch2AgentsDir
	oldConfigDir := os.Getenv("SIN_CODE_CONFIG_DIR")
	defer func() {
		orch2NoPlugins = oldNoPlugins
		orch2AgentsDir = oldAgentsDir
		os.Setenv("SIN_CODE_CONFIG_DIR", oldConfigDir)
	}()

	orch2NoPlugins = false
	orch2AgentsDir = t.TempDir()

	cfgDir := t.TempDir()
	os.Setenv("SIN_CODE_CONFIG_DIR", cfgDir)
	subDir := filepath.Join(cfgDir, "sin-code", "plugins", "my-plugin")
	os.MkdirAll(subDir, 0o755)
	manifest := `name = "my-plugin"
version = "1.0.0"
[[agents]]
name = "plugin-agent"
type = "code"
model = "openai/gpt-4o"
provider = "openai"
`
	os.WriteFile(filepath.Join(subDir, plugins.ManifestFile), []byte(manifest), 0o644)

	agents, err := loadAllAgents()
	if err != nil {
		t.Fatalf("loadAllAgents failed: %v", err)
	}
	found := false
	for _, a := range agents {
		if a.Name == "plugin-my-plugin-plugin-agent" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected plugin agent, got %v", agents)
	}
}

func TestSetAgentField_Extended(t *testing.T) {
	cfg := &orchestrator.AgentConfig{Name: "test"}
	tests := []struct {
		key string
		val string
	}{
		{"name", "renamed"},
		{"description", "desc"},
		{"provider", "openai"},
		{"model", "gpt-4"},
		{"max_tokens", "2048"},
		{"temperature", "0.5"},
		{"tools_allow", "read,write"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if err := setAgentField(cfg, tt.key, tt.val); err != nil {
				t.Fatalf("setAgentField(%q, %q): %v", tt.key, tt.val, err)
			}
		})
	}
	if cfg.Name != "renamed" {
		t.Errorf("name = %q, want renamed", cfg.Name)
	}
	if cfg.MaxTokens != 2048 {
		t.Errorf("max_tokens = %d, want 2048", cfg.MaxTokens)
	}
	if cfg.Temperature != 0.5 {
		t.Errorf("temperature = %f, want 0.5", cfg.Temperature)
	}
	if len(cfg.ToolsAllow) != 2 {
		t.Errorf("tools_allow = %v, want 2 entries", cfg.ToolsAllow)
	}
}

func TestSetAgentField_InvalidNumber(t *testing.T) {
	cfg := &orchestrator.AgentConfig{}
	if err := setAgentField(cfg, "max_tokens", "abc"); err == nil {
		t.Fatal("expected error for invalid max_tokens")
	}
	if err := setAgentField(cfg, "temperature", "abc"); err == nil {
		t.Fatal("expected error for invalid temperature")
	}
}

