// SPDX-License-Identifier: MIT
// Purpose: /init built-in command tests.
package commands

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func initTestSession(t *testing.T) *session.Session {
	t.Helper()
	store, err := session.Open(filepath.Join(t.TempDir(), "init.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	return sess
}

func writeProjectFile(t *testing.T, root, rel string, content string) {
	t.Helper()
	full := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func TestInitCommand_NameAndDescription(t *testing.T) {
	c := NewInitCommand()
	if c.Name() != "init" {
		t.Errorf("Name: %q", c.Name())
	}
	if c.Description() != "Analyze project and generate AGENTS.md" {
		t.Errorf("Description: %q", c.Description())
	}
}

func TestInitCommand_DetectGoProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module github.com/example/myproject\n\ngo 1.23\n")
	writeProjectFile(t, root, "cmd/main.go", "package main\n")
	c := NewInitCommand()
	if got := c.DetectProjectType(root); got != "go" {
		t.Errorf("DetectProjectType: want go, got %q", got)
	}
}

func TestInitCommand_DetectPythonProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "pyproject.toml", "[project]\nname = \"myapp\"\n")
	writeProjectFile(t, root, "src/app.py", "print('hello')\n")
	c := NewInitCommand()
	if got := c.DetectProjectType(root); got != "python" {
		t.Errorf("DetectProjectType: want python, got %q", got)
	}
}

func TestInitCommand_DetectNodeProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "package.json", `{"name": "myapp", "version": "1.0.0"}`)
	c := NewInitCommand()
	if got := c.DetectProjectType(root); got != "node" {
		t.Errorf("DetectProjectType: want node, got %q", got)
	}
}

func TestInitCommand_DetectRustProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "Cargo.toml", `[package]\nname = "myapp"\nversion = "0.1.0"\n`)
	c := NewInitCommand()
	if got := c.DetectProjectType(root); got != "rust" {
		t.Errorf("DetectProjectType: want rust, got %q", got)
	}
}

func TestInitCommand_DetectGenericProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "README.md", "# My Project\n")
	c := NewInitCommand()
	if got := c.DetectProjectType(root); got != "generic" {
		t.Errorf("DetectProjectType: want generic, got %q", got)
	}
}

func TestInitCommand_DetectTools(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, ".golangci.yml", "linters:\n  enable:\n    - gosec\n")
	writeProjectFile(t, root, "go.mod", "module example\n")
	writeProjectFile(t, root, ".github/workflows/ci.yml", "name: ci\n")
	writeProjectFile(t, root, "Makefile", "build:\n\tgo build ./...\n")
	writeProjectFile(t, root, "Dockerfile", "FROM golang:1.23\n")
	c := NewInitCommand()
	tools := c.DetectTools(root)
	if !containsStr(tools, "golangci-lint") {
		t.Errorf("expected golangci-lint in tools: %v", tools)
	}
	if !containsStr(tools, "github-actions") {
		t.Errorf("expected github-actions in tools: %v", tools)
	}
	if !containsStr(tools, "make") {
		t.Errorf("expected make in tools: %v", tools)
	}
	if !containsStr(tools, "docker") {
		t.Errorf("expected docker in tools: %v", tools)
	}
}

func TestInitCommand_DetectTools_ESLint(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "package.json", `{"name": "app"}`)
	writeProjectFile(t, root, "eslint.config.js", "export default {};\n")
	writeProjectFile(t, root, ".prettierrc", `{"semi": true}`)
	c := NewInitCommand()
	tools := c.DetectTools(root)
	if !containsStr(tools, "eslint") {
		t.Errorf("expected eslint: %v", tools)
	}
	if !containsStr(tools, "prettier") {
		t.Errorf("expected prettier: %v", tools)
	}
}

func TestInitCommand_GenerateAGENTS_Go(t *testing.T) {
	info := &ProjectInfo{
		Type:          "go",
		RootDir:       "/tmp/myproject",
		BuildCmd:      "go build ./...",
		TestCmd:       "go test ./... -race -count=1",
		LintCmd:       "golangci-lint run",
		ModulePath:    "github.com/example/myproject",
		KeyDirs:       []string{"cmd/", "internal/"},
		TestFramework: "go test",
		CI:            "GitHub Actions",
		Tools:         []string{"golangci-lint", "github-actions", "make"},
	}
	c := NewInitCommand()
	out := c.GenerateAGENTS(info)
	if !strings.Contains(out, "go") {
		t.Errorf("expected 'go' in output")
	}
	if !strings.Contains(out, "github.com/example/myproject") {
		t.Errorf("expected module path in output")
	}
	if !strings.Contains(out, "go build ./...") {
		t.Errorf("expected build command in output")
	}
	if !strings.Contains(out, "go test ./... -race -count=1") {
		t.Errorf("expected test command in output")
	}
	if !strings.Contains(out, "golangci-lint run") {
		t.Errorf("expected lint command in output")
	}
	if !strings.Contains(out, "cmd/") {
		t.Errorf("expected key dir cmd/ in output")
	}
	if !strings.Contains(out, "GitHub Actions") {
		t.Errorf("expected CI in output")
	}
	if !strings.Contains(out, "golangci-lint") {
		t.Errorf("expected tool in output")
	}
}

func TestInitCommand_GenerateAGENTS_Python(t *testing.T) {
	info := &ProjectInfo{
		Type:          "python",
		BuildCmd:      "pip install -e .",
		TestCmd:       "pytest",
		LintCmd:       "ruff check .",
		TestFramework: "pytest",
		Tools:         []string{"ruff"},
	}
	c := NewInitCommand()
	out := c.GenerateAGENTS(info)
	if !strings.Contains(out, "python") {
		t.Errorf("expected 'python' in output")
	}
	if !strings.Contains(out, "pytest") {
		t.Errorf("expected pytest in output")
	}
	if !strings.Contains(out, "ruff check .") {
		t.Errorf("expected ruff lint command in output")
	}
}

func TestInitCommand_GenerateAGENTS_Node(t *testing.T) {
	info := &ProjectInfo{
		Type:          "node",
		BuildCmd:      "npm run build",
		TestCmd:       "npm test",
		LintCmd:       "npx eslint .",
		ModulePath:    "my-app",
		TestFramework: "jest",
		Tools:         []string{"eslint"},
	}
	c := NewInitCommand()
	out := c.GenerateAGENTS(info)
	if !strings.Contains(out, "node") {
		t.Errorf("expected 'node' in output")
	}
	if !strings.Contains(out, "npm run build") {
		t.Errorf("expected npm build command in output")
	}
	if !strings.Contains(out, "jest") {
		t.Errorf("expected jest in output")
	}
}

func TestInitCommand_GenerateAGENTS_Rust(t *testing.T) {
	info := &ProjectInfo{
		Type:          "rust",
		BuildCmd:      "cargo build",
		TestCmd:       "cargo test",
		LintCmd:       "cargo clippy",
		ModulePath:    "myapp",
		TestFramework: "cargo test",
	}
	c := NewInitCommand()
	out := c.GenerateAGENTS(info)
	if !strings.Contains(out, "rust") {
		t.Errorf("expected 'rust' in output")
	}
	if !strings.Contains(out, "cargo build") {
		t.Errorf("expected cargo build in output")
	}
	if !strings.Contains(out, "cargo clippy") {
		t.Errorf("expected cargo clippy in output")
	}
}

func TestInitCommand_Execute_GoProject(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module github.com/test/proj\n\ngo 1.23\n")
	writeProjectFile(t, root, "cmd/main.go", "package main\nfunc main() {}\n")
	writeProjectFile(t, root, "internal/foo/foo.go", "package foo\n")
	c := NewInitCommand()
	c.rootDir = root
	out, err := c.Execute(context.Background(), "", initTestSession(t))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !strings.Contains(out, "go build ./...") {
		t.Errorf("expected go build command in output")
	}
	if !strings.Contains(out, "github.com/test/proj") {
		t.Errorf("expected module path in output")
	}
	if !strings.Contains(out, "AGENTS.md (draft") {
		t.Errorf("expected draft header in output")
	}
}

func TestInitCommand_Execute_MergeWithExisting(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module github.com/test/proj\n\ngo 1.23\n")
	writeProjectFile(t, root, "AGENTS.md", "# Existing AGENTS.md\n\nCustom rules here.\n")
	c := NewInitCommand()
	c.rootDir = root
	out, err := c.Execute(context.Background(), "", initTestSession(t))
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if !strings.Contains(out, "Preserved from existing AGENTS.md") {
		t.Errorf("expected merge section in output")
	}
	if !strings.Contains(out, "Custom rules here.") {
		t.Errorf("expected existing content preserved in output")
	}
}

func TestInitCommand_Execute_DoesNotModifySession(t *testing.T) {
	sess := initTestSession(t)
	before := sess.History()
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module test\n\ngo 1.23\n")
	c := NewInitCommand()
	c.rootDir = root
	if _, err := c.Execute(context.Background(), "", sess); err != nil {
		t.Fatalf("err: %v", err)
	}
	after := sess.History()
	if len(after) != len(before) {
		t.Errorf("history changed: before=%d after=%d", len(before), len(after))
	}
}

func TestInitCommand_DetectTestFramework_Pytest(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "pyproject.toml", "[tool.pytest.ini_options]\n")
	writeProjectFile(t, root, "conftest.py", "import pytest\n")
	c := NewInitCommand()
	if got := c.detectTestFramework("python", root); got != "pytest" {
		t.Errorf("detectTestFramework: want pytest, got %q", got)
	}
}

func TestInitCommand_DetectTestFramework_Jest(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "package.json", `{"name": "app", "jest": {"testEnvironment": "node"}}`)
	c := NewInitCommand()
	if got := c.detectTestFramework("node", root); got != "jest" {
		t.Errorf("detectTestFramework: want jest, got %q", got)
	}
}

func TestInitCommand_DetectKeyDirs(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, "cmd/main.go", "package main\n")
	writeProjectFile(t, root, "internal/foo.go", "package internal\n")
	writeProjectFile(t, root, "docs/readme.md", "# docs\n")
	os.MkdirAll(filepath.Join(root, "node_modules"), 0755)
	os.MkdirAll(filepath.Join(root, ".git"), 0755)
	c := NewInitCommand()
	dirs := c.detectKeyDirs(root)
	if !containsStr(dirs, "cmd/") {
		t.Errorf("expected cmd/ in key dirs: %v", dirs)
	}
	if !containsStr(dirs, "internal/") {
		t.Errorf("expected internal/ in key dirs: %v", dirs)
	}
	if !containsStr(dirs, "docs/") {
		t.Errorf("expected docs/ in key dirs: %v", dirs)
	}
	for _, d := range dirs {
		if strings.Contains(d, "node_modules") || strings.Contains(d, ".git") {
			t.Errorf("ignored dir should not be in key dirs: %q", d)
		}
	}
}

func TestInitCommand_RegistryIntegration(t *testing.T) {
	r := NewRegistry()
	r.Register(NewInitCommand())
	cmd, ok := r.Get("init")
	if !ok {
		t.Fatal("init command not found in registry")
	}
	if cmd.Name() != "init" {
		t.Errorf("Name: %q", cmd.Name())
	}
	root := t.TempDir()
	writeProjectFile(t, root, "go.mod", "module test\n\ngo 1.23\n")
	initCmd := cmd.(*InitCommand)
	initCmd.rootDir = root
	sess := initTestSession(t)
	handled, out, err := r.Dispatch(context.Background(), "/init", sess)
	if !handled {
		t.Error("expected /init to be handled")
	}
	if err != nil {
		t.Errorf("err: %v", err)
	}
	if !strings.Contains(out, "AGENTS.md") {
		t.Errorf("expected AGENTS.md in output")
	}
}

func containsStr(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
