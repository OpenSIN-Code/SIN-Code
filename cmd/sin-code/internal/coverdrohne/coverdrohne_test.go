// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the coverdrohne package (meta-coverage).
package coverdrohne

import (
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewScannerDefaults(t *testing.T) {
	s := NewScanner()
	if s.GoTest != "go" {
		t.Errorf("GoTest default = %q, want go", s.GoTest)
	}
	if s.Packages != "./cmd/sin-code/..." {
		t.Errorf("Packages default = %q, want ./cmd/sin-code/...", s.Packages)
	}
}

func TestScannerScan(t *testing.T) {
	s := NewScanner()
	s.runGoTest = func(dir, packages, coverprofile string) ([]byte, error) {
		if packages != "./..." {
			t.Errorf("packages = %q, want ./...", packages)
		}
		return []byte(
				"ok  github.com/example/a  0.010s  coverage: 50.0% of statements\n" +
					"ok  github.com/example/b  0.020s  coverage: 100.0% of statements\n" +
					"ok  github.com/example/c  0.030s  coverage: 0.0% of statements\n"),
			nil
	}
	s.Packages = "./..."
	res, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("len(res) = %d, want 3", len(res))
	}
	want := []PackageCoverage{
		{ImportPath: "github.com/example/c", Coverage: 0},
		{ImportPath: "github.com/example/a", Coverage: 50},
		{ImportPath: "github.com/example/b", Coverage: 100},
	}
	for i, r := range res {
		if r.ImportPath != want[i].ImportPath || r.Coverage != want[i].Coverage {
			t.Errorf("res[%d] = %+v, want %+v", i, r, want[i])
		}
	}
}

func TestScannerScanFailure(t *testing.T) {
	s := NewScanner()
	s.runGoTest = func(dir, packages, coverprofile string) ([]byte, error) {
		return []byte("fail output"), &exitError{msg: "exit status 1"}
	}
	_, err := s.Scan()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "go test failed") {
		t.Errorf("error = %q, want go test failed", err.Error())
	}
}

func TestScannerScanVerbose(t *testing.T) {
	s := NewScanner()
	s.runGoTest = func(dir, packages, coverprofile string) ([]byte, error) {
		return []byte("ok  github.com/example/a  0.010s  coverage: 75.0% of statements\n"), nil
	}
	s.Verbose = true
	_, err := s.Scan()
	if err != nil {
		t.Fatal(err)
	}
}

type exitError struct {
	msg string
}

func (e *exitError) Error() string { return e.msg }

// repoRoot returns the repository root by walking up from the test's working
// directory until a go.mod file is found.
func repoRoot() string {
	dir, _ := os.Getwd()
	for dir != "" && dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func TestParseGoTestCoverageOutput(t *testing.T) {
	out := "ok  github.com/example/a  0.010s  coverage: 75.5% of statements\n" +
		"ok  github.com/example/b  0.020s  coverage: 100.0% of statements\n" +
		"some noise line\n"
	res, err := parseGoTestCoverageOutput(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("len = %d, want 2", len(res))
	}
	if res[0].ImportPath != "github.com/example/a" || res[0].Coverage != 75.5 {
		t.Errorf("res[0] = %+v", res[0])
	}
	if res[1].ImportPath != "github.com/example/b" || res[1].Coverage != 100.0 {
		t.Errorf("res[1] = %+v", res[1])
	}
}

func TestModulePath(t *testing.T) {
	dir := t.TempDir()
	if got := modulePath(dir); got != "" {
		t.Errorf("missing go.mod = %q, want empty", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/example/mod\n\ngo 1.23\n"), 0o644)
	if got := modulePath(dir); got != "github.com/example/mod" {
		t.Errorf("modulePath = %q, want github.com/example/mod", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module 'github.com/example/quoted'\n"), 0o644)
	if got := modulePath(dir); got != "github.com/example/quoted" {
		t.Errorf("quoted modulePath = %q, want github.com/example/quoted", got)
	}
}

func TestProfileFileToLocal(t *testing.T) {
	root := "/project"
	mod := "github.com/example/mod"
	if got := profileFileToLocal(root, mod, "github.com/example/mod/foo/bar.go"); got != "/project/foo/bar.go" {
		t.Errorf("module path = %q, want /project/foo/bar.go", got)
	}
	if got := profileFileToLocal(root, "", "foo/bar.go"); got != "/project/foo/bar.go" {
		t.Errorf("relative path = %q, want /project/foo/bar.go", got)
	}
	if got := profileFileToLocal(root, "", "/abs/foo/bar.go"); got != "/abs/foo/bar.go" {
		t.Errorf("absolute path = %q, want /abs/foo/bar.go", got)
	}
}

func TestGaps(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/example/mod\n\ngo 1.23\n"), 0o644)
	srcDir := filepath.Join(dir, "pkg")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "foo.go"), []byte(`package pkg

func Foo() {
	if true {
		_ = 1
	}
	Bar()
}

func Bar() {
	_ = 2
}
`), 0o644)

	profile := filepath.Join(dir, "coverage.out")
	data := "mode: set\n" +
		"github.com/example/mod/pkg/foo.go:3.12,8.2 1 1\n" +
		"github.com/example/mod/pkg/foo.go:7.2,7.8 1 0\n" +
		"github.com/example/mod/pkg/foo.go:10.12,12.2 1 0\n"
	_ = os.WriteFile(profile, []byte(data), 0o644)

	gaps, err := Gaps(profile, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 {
		t.Fatalf("len(gaps) = %d, want 1", len(gaps))
	}
	g := gaps[0]
	if g.File != "pkg/foo.go" {
		t.Errorf("file = %q, want pkg/foo.go", g.File)
	}
	if len(g.Blocks) != 2 {
		t.Fatalf("len(blocks) = %d, want 2", len(g.Blocks))
	}
	if g.Blocks[0].FuncName != "Foo" {
		t.Errorf("func[0] = %q, want Foo", g.Blocks[0].FuncName)
	}
	if g.Blocks[1].FuncName != "Bar" {
		t.Errorf("func[1] = %q, want Bar", g.Blocks[1].FuncName)
	}
}

func TestGapsInvalidProfile(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "coverage.out")
	_ = os.WriteFile(profile, []byte("not a mode line\n"), 0o644)
	_, err := Gaps(profile, dir)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseProfileLine(t *testing.T) {
	b, err := parseProfileLine("github.com/example/mod/pkg/foo.go:10.12,12.2 1 0")
	if err != nil {
		t.Fatal(err)
	}
	if b.File != "github.com/example/mod/pkg/foo.go" || b.StartLine != 10 || b.EndLine != 12 || b.NumStmts != 1 || b.Count != 0 {
		t.Errorf("block = %+v", b)
	}
	_, err = parseProfileLine("bad")
	if err == nil {
		t.Fatal("expected error for bad line")
	}
	_, err = parseProfileLine("file:1.2,3.4 abc 0")
	if err == nil {
		t.Fatal("expected error for bad stmt count")
	}
	_, err = parseProfileLine("file:1.2,3.4 1 xyz")
	if err == nil {
		t.Fatal("expected error for bad count")
	}
}

func TestParseFilePos(t *testing.T) {
	file, start, end, err := parseFilePos("foo/bar.go:1.2,3.4")
	if err != nil || file != "foo/bar.go" || start != 1 || end != 3 {
		t.Errorf("got %q %d %d %v", file, start, end, err)
	}
	_, _, _, err = parseFilePos("bad")
	if err == nil {
		t.Fatal("expected error")
	}
	_, _, _, err = parseFilePos("foo:1,2,3")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLineFromPos(t *testing.T) {
	v, err := lineFromPos("12.34")
	if err != nil || v != 12 {
		t.Errorf("lineFromPos = %d, want 12", v)
	}
	v, err = lineFromPos("56")
	if err != nil || v != 56 {
		t.Errorf("lineFromPos = %d, want 56", v)
	}
	_, err = lineFromPos("abc")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFuncNameForBlock(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "foo.go"), []byte(`package pkg

func Foo() {}
func (s *Store) Bar() {}
type T struct{}
func (t T) Baz() {}
`), 0o644)
	n, err := funcNameForBlock(dir, filepath.Join(dir, "foo.go"), 3)
	if err != nil || n != "Foo" {
		t.Errorf("Foo = %q %v, want Foo", n, err)
	}
	n, err = funcNameForBlock(dir, filepath.Join(dir, "foo.go"), 4)
	if err != nil || n != "(Store).Bar" {
		t.Errorf("Bar = %q %v, want (Store).Bar", n, err)
	}
	n, err = funcNameForBlock(dir, filepath.Join(dir, "foo.go"), 6)
	if err != nil || n != "(T).Baz" {
		t.Errorf("Baz = %q %v, want (T).Baz", n, err)
	}
}

func TestFuncDeclName(t *testing.T) {
	// Cannot test funcDeclName directly without a real AST, so we test via funcNameForBlock.
	n, _ := funcNameForBlock(t.TempDir(), filepath.Join("does", "not", "exist.go"), 1)
	if n != "" {
		t.Errorf("missing file = %q, want empty", n)
	}
}

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	if cmd.Name() != "cover" {
		t.Errorf("name = %q, want cover", cmd.Name())
	}
	if cmd.Commands() == nil || len(cmd.Commands()) == 0 {
		t.Fatal("expected subcommands")
	}
}

func TestScanCmd(t *testing.T) {
	cmd := newScanCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestScanCmdJSON(t *testing.T) {
	var buf strings.Builder
	cmd := newScanCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out []PackageCoverage
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
}

func TestCheckCmdPass(t *testing.T) {
	var buf strings.Builder
	cmd := newCheckCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--min", "0"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "all packages") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestCheckCmdFail(t *testing.T) {
	cmd := newCheckCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetErr(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--min", "100"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGapsCmd(t *testing.T) {
	cmd := newGapsCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "instinct"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestGapsCmdJSON(t *testing.T) {
	var buf strings.Builder
	cmd := newGapsCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "instinct", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out []Gap
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCmd(t *testing.T) {
	var buf strings.Builder
	cmd := newGenerateCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "instinct"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCmdRequiresPackage(t *testing.T) {
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error without --package")
	}
}

func TestGenerateCmdOutFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "req.json")
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "instinct", "--out", out})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateCmdNotFound(t *testing.T) {
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "nonexistent-package-xyz"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestHookCmd(t *testing.T) {
	var buf strings.Builder
	cmd := newHookCmd()
	cmd.SetOut(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "sin-code cover check") {
		t.Errorf("hook = %q", out)
	}
	if !strings.Contains(out, "#!/bin/sh") {
		t.Errorf("missing shebang")
	}
}

func TestHookCmdInstall(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	_ = os.MkdirAll(gitDir, 0o755)
	cmd := newHookCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--install", "--min", "80"})
	// The hook writes to .git/hooks relative to cwd, so switch to the temp dir.
	old, _ := os.Getwd()
	_ = os.Chdir(dir)
	defer os.Chdir(old)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(gitDir, "hooks", "pre-commit"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "80.0") {
		t.Errorf("installed hook = %q", string(data))
	}
}

func TestHookCmdInstallMkdirError(t *testing.T) {
	mkdirAllHook = func(path string, perm os.FileMode) error {
		return fmt.Errorf("mkdir err")
	}
	defer func() { mkdirAllHook = os.MkdirAll }()
	cmd := newHookCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--install"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "mkdir err") {
		t.Fatalf("error = %v", err)
	}
}

func TestHookCmdInstallWriteError(t *testing.T) {
	mkdirAllHook = func(path string, perm os.FileMode) error { return nil }
	defer func() { mkdirAllHook = os.MkdirAll }()
	writeFileModeHook = func(name string, data []byte, perm os.FileMode) error {
		return fmt.Errorf("write err")
	}
	defer func() {
		writeFileModeHook = func(name string, data []byte, perm os.FileMode) error { return os.WriteFile(name, data, perm) }
	}()
	cmd := newHookCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--install"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "write err") {
		t.Fatalf("error = %v", err)
	}
}

func TestFilterGapsByPackage(t *testing.T) {
	gaps := []Gap{
		{File: "a/b.go"},
		{File: "c/d.go"},
	}
	filtered := filterGapsByPackage(gaps, "c")
	if len(filtered) != 1 || filtered[0].File != "c/d.go" {
		t.Errorf("filtered = %+v", filtered)
	}
}

func TestScanDefaults(t *testing.T) {
	mkdirTemp = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTemp = os.MkdirTemp }()
	s := &Scanner{}
	if _, err := s.Scan(); err == nil || !strings.Contains(err.Error(), "mkdir err") {
		t.Fatalf("error = %v", err)
	}
}

func TestGapsOpenError(t *testing.T) {
	openFileHook = func(name string) (*os.File, error) {
		return nil, fmt.Errorf("open err")
	}
	defer func() { openFileHook = os.Open }()
	_, err := Gaps("x.out", ".")
	if err == nil || !strings.Contains(err.Error(), "open err") {
		t.Fatalf("error = %v", err)
	}
}

func TestGapsScannerError(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "coverage.out")
	data := "mode: set\n" + strings.Repeat("x", 1<<20) + "\n"
	_ = os.WriteFile(profile, []byte(data), 0o644)
	openFileHook = func(name string) (*os.File, error) {
		return os.Open(profile)
	}
	defer func() { openFileHook = os.Open }()
	_, err := Gaps(profile, ".")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestModulePathReadError(t *testing.T) {
	readFileHook = func(name string) ([]byte, error) {
		return nil, fmt.Errorf("read err")
	}
	defer func() { readFileHook = os.ReadFile }()
	if got := modulePath("."); got != "" {
		t.Errorf("modulePath = %q, want empty", got)
	}
}

func TestGapsModulePath(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/example/mod\n\ngo 1.23\n"), 0o644)
	srcDir := filepath.Join(dir, "pkg")
	_ = os.MkdirAll(srcDir, 0o755)
	_ = os.WriteFile(filepath.Join(srcDir, "foo.go"), []byte("package pkg\n\nfunc Foo() {}\n"), 0o644)

	profile := filepath.Join(dir, "coverage.out")
	data := "mode: set\n" + "github.com/example/mod/pkg/foo.go:1.12,3.2 1 0\n"
	_ = os.WriteFile(profile, []byte(data), 0o644)

	gaps, err := Gaps(profile, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].File != "pkg/foo.go" {
		t.Errorf("gaps = %+v", gaps)
	}
}

func TestCheckCmdJSON(t *testing.T) {
	var buf strings.Builder
	cmd := newCheckCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--min", "0", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
	if passed, _ := out["passed"].(bool); !passed {
		t.Errorf("passed = false")
	}
}

func TestCheckCmdJSONFail(t *testing.T) {
	var buf strings.Builder
	cmd := newCheckCmd()
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--min", "100", "--json"})
	if err := cmd.Execute(); err != nil {
		// JSON mode returns the failed list rather than an error so CI can consume it.
		t.Fatalf("json check should not error: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatal(err)
	}
	if passed, _ := out["passed"].(bool); passed {
		t.Errorf("passed = true")
	}
	if failed, ok := out["failed"].([]any); !ok || len(failed) == 0 {
		t.Errorf("failed = %v", out["failed"])
	}
}

func TestGenerateCmdWriteFileError(t *testing.T) {
	writeFileHook = func(name string, data []byte, perm os.FileMode) error {
		return fmt.Errorf("write err")
	}
	defer func() { writeFileHook = os.WriteFile }()
	out := filepath.Join(t.TempDir(), "req.json")
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/instinct", "--package", "instinct", "--out", out})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "write err") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunCoverageProfileMkdirError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	_, err := runCoverageProfile(repoRoot(), "./cmd/sin-code/internal/wiring")
	if err == nil || !strings.Contains(err.Error(), "mkdir err") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunCoverageProfileTestError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		return []byte("fail"), &exitError{msg: "boom"}
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	_, err := runCoverageProfile(repoRoot(), "./cmd/sin-code/internal/wiring")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestScanWithProfileError(t *testing.T) {
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		return []byte("fail"), &exitError{msg: "boom"}
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	_, err := scanWithProfile(repoRoot(), "./cmd/sin-code/internal/wiring", "x.out")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseGoTestCoverageOutputBadFields(t *testing.T) {
	_, err := parseGoTestCoverageOutput("ok  pkg  0.010s  coverage: 1.2.3% of statements\n")
	if err == nil {
		t.Fatal("expected error")
	}
	_, err = parseGoTestCoverageOutput("ok  pkg  0.010s  coverage: abc% of statements\n")
	if err != nil {
		// regex does not match, so returns empty results without error.
		t.Fatalf("non-matching line should not error: %v", err)
	}
}

func TestParseGoTestCoverageOutputScannerError(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	_, err := parseGoTestCoverageOutput("ok  " + big + "  coverage: 50.0% of statements\n")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseGoTestCoverageOutputParseError(t *testing.T) {
	_, err := parseGoTestCoverageOutput("ok  pkg  0.010s  coverage: 1.2.3% of statements\n")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestFuncDeclNameGeneric(t *testing.T) {
	_ = funcDeclName // silence unused warning if not reached
	if _, ok := interface{}(funcDeclName).(func(*ast.FuncDecl) string); !ok {
		t.Fatal("funcDeclName signature mismatch")
	}
}

func TestFuncDeclNameGenericReceivers(t *testing.T) {
	cases := []struct {
		code string
		line int
		want string
	}{
		{"package pkg\n\ntype G[T any] struct{}\n\nfunc (g *G[T]) Foo() {}\n", 5, "(G).Foo"},
		{"package pkg\n\ntype G[T any] struct{}\n\nfunc (g G[T]) Bar() {}\n", 5, "(G).Bar"},
	}
	for _, c := range cases {
		dir := t.TempDir()
		f := filepath.Join(dir, "foo.go")
		_ = os.WriteFile(f, []byte(c.code), 0o644)
		got, _ := funcNameForBlock(dir, f, c.line)
		if got != c.want {
			t.Errorf("line %d: got %q, want %q", c.line, got, c.want)
		}
	}
}

func TestFuncDeclNameEmptyRecv(t *testing.T) {
	// Create an AST with a receiver whose Type is unknown so recv stays empty.
	dir := t.TempDir()
	f := filepath.Join(dir, "foo.go")
	_ = os.WriteFile(f, []byte("package pkg\n\ntype T struct{}\n\nfunc (t.T) Baz() {}\n"), 0o644)
	got, _ := funcNameForBlock(dir, f, 5)
	if got != "Baz" {
		t.Errorf("got %q, want Baz", got)
	}
}

func TestModulePathNoModuleLine(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.23\n"), 0o644)
	if got := modulePath(dir); got != "" {
		t.Errorf("modulePath = %q, want empty", got)
	}
}

func TestParseFilePosLineFromPosError(t *testing.T) {
	_, _, _, err := parseFilePos("file:abc.1,2.3")
	if err == nil {
		t.Fatal("expected error")
	}
	_, _, _, err = parseFilePos("file:1.2,xyz.3")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseProfileLineInvalidFilePos(t *testing.T) {
	_, err := parseProfileLine("badfile:1.2,xyz 1 0")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGapsEmptySrcRoot(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "coverage.out")
	data := "mode: set\n" + "foo.go:1.12,3.2 1 0\n"
	_ = os.WriteFile(profile, []byte(data), 0o644)

	gaps, err := Gaps(profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].File != "foo.go" {
		t.Errorf("gaps = %+v", gaps)
	}
}

func TestGapsRelEmpty(t *testing.T) {
	absFile := filepath.Join(t.TempDir(), "foo.go")
	_ = os.WriteFile(absFile, []byte("package pkg\n\nfunc Foo() {}\n"), 0o644)

	profile := filepath.Join(t.TempDir(), "coverage.out")
	data := "mode: set\n" + absFile + ":3.12,5.2 1 0\n"
	_ = os.WriteFile(profile, []byte(data), 0o644)

	gaps, err := Gaps(profile, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || gaps[0].File != absFile {
		t.Errorf("gaps = %+v", gaps)
	}
}

func TestScanCmdError(t *testing.T) {
	mkdirTemp = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTemp = os.MkdirTemp }()
	cmd := newScanCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestCheckCmdScanError(t *testing.T) {
	mkdirTemp = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTemp = os.MkdirTemp }()
	cmd := newCheckCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--min", "0"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGapsCmdRunCoverageError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	cmd := newGapsCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGapsCmdGapsError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\nbad line\n"), 0o644)
		return []byte("ok  pkg  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	cmd := newGapsCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGapsParseProfileLineError(t *testing.T) {
	dir := t.TempDir()
	profile := filepath.Join(dir, "coverage.out")
	_ = os.WriteFile(profile, []byte("mode: set\nbad line\n"), 0o644)
	_, err := Gaps(profile, dir)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateCmdRunCoverageError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateCmdScanWithProfileError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  github.com/example/wiring  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	scanWithProfileHook = func(root, packages, coverprofile string) ([]PackageCoverage, error) {
		return nil, fmt.Errorf("scan err")
	}
	defer func() { scanWithProfileHook = scanWithProfile }()
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "scan err") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateCmdJSONMarshalError(t *testing.T) {
	jsonMarshalIndentHook = func(v any, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("marshal err")
	}
	defer func() { jsonMarshalIndentHook = json.MarshalIndent }()
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "marshal err") {
		t.Fatalf("error = %v", err)
	}
}

func TestGenerateCmdGapsError(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\nbad line\n"), 0o644)
		return []byte("ok  github.com/example/wiring  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	cmd := newGenerateCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", repoRoot(), "--packages", "./cmd/sin-code/internal/wiring", "--package", "wiring"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}

func TestScanParseError(t *testing.T) {
	mkdirTemp = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTemp = os.MkdirTemp }()
	s := &Scanner{}
	s.runGoTest = func(dir, packages, coverprofile string) ([]byte, error) {
		return []byte("ok  pkg  0.010s  coverage: 1.2.3% of statements\n"), nil
	}
	_, err := s.Scan()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunCoverageProfileDefaultPackages(t *testing.T) {
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	_, err := runCoverageProfile(repoRoot(), "")
	if err == nil || !strings.Contains(err.Error(), "mkdir err") {
		t.Fatalf("error = %v", err)
	}
}

func TestPackageImportPath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pkg := filepath.Join(tmp, "cmd", "foo", "bar.go")
	if got := PackageImportPath(tmp, pkg); got != "example.com/demo/cmd/foo" {
		t.Errorf("PackageImportPath = %q, want example.com/demo/cmd/foo", got)
	}
	root := filepath.Join(tmp, "main.go")
	if got := PackageImportPath(tmp, root); got != "example.com/demo" {
		t.Errorf("PackageImportPath root = %q, want example.com/demo", got)
	}
}

func TestPackageImportPathNoGoMod(t *testing.T) {
	tmp := t.TempDir()
	pkg := filepath.Join(tmp, "cmd", "foo", "bar.go")
	if got := PackageImportPath(tmp, pkg); got != pkg {
		t.Errorf("PackageImportPath = %q, want %q", got, pkg)
	}
}

func TestPackageImportPathOutsideRoot(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(tmp, "..", "other", "bar.go")
	if got := PackageImportPath(tmp, outside); got != outside {
		t.Errorf("PackageImportPath = %q, want %q", got, outside)
	}
}

// --- drain ---

func TestDrainReadDirError(t *testing.T) {
	readDirHook = func(name string) ([]os.DirEntry, error) {
		return nil, fmt.Errorf("read dir err")
	}
	defer func() { readDirHook = os.ReadDir }()
	cmd := newDrainCmd()
	cmd.SetOut(&strings.Builder{})
	cmd.SetArgs([]string{"--root", t.TempDir()})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "read dir err") {
		t.Fatalf("error = %v", err)
	}
}

func TestDrainEmptyQueue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".sin-code/coverage-requests"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "drained 0 requests") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainInvalidJSON(t *testing.T) {
	tmp := t.TempDir()
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "bad.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "invalid JSON") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainMissingPackage(t *testing.T) {
	tmp := t.TempDir()
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "bad.json"), []byte(`{"file":"x.go"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "missing package") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainOneRequest(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "generated") {
		t.Errorf("output = %q", out)
	}
	if !strings.Contains(out, "drained 1 requests, generated 1, enqueued 0") {
		t.Errorf("summary = %q", out)
	}
}

func TestDrainCoverageProfileError(t *testing.T) {
	tmp := t.TempDir()
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return "", fmt.Errorf("mkdir err")
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "coverage profile failed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainEnqueue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()

	var called bool
	EnqueueGoal = func(ctx context.Context, prompt, workspace string) error {
		called = true
		return nil
	}
	defer func() { EnqueueGoal = nil }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./...", "--enqueue"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("EnqueueGoal not called")
	}
	if !strings.Contains(buf.String(), "enqueued 1") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainSkipsDirAndNonJSON(t *testing.T) {
	tmp := t.TempDir()
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(filepath.Join(reqDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "readme.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "drained 2 requests, generated 0, enqueued 0") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainReadFileError(t *testing.T) {
	tmp := t.TempDir()
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"x"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	readFileHook = func(name string) ([]byte, error) {
		return nil, fmt.Errorf("read err")
	}
	defer func() { readFileHook = os.ReadFile }()
	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "read error") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainScanWithProfileError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	scanWithProfileHook = func(root, packages, coverprofile string) ([]PackageCoverage, error) {
		return nil, fmt.Errorf("scan err")
	}
	defer func() { scanWithProfileHook = scanWithProfile }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "scan failed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainTargetNotFound(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"other"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	scanWithProfileHook = func(root, packages, coverprofile string) ([]PackageCoverage, error) {
		return []PackageCoverage{{ImportPath: "example.com/demo/pkg", Coverage: 100}}, nil
	}
	defer func() { scanWithProfileHook = scanWithProfile }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "not found in coverage scan") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainGapsError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	openFileHook = func(name string) (*os.File, error) {
		return nil, fmt.Errorf("open err")
	}
	defer func() { openFileHook = os.Open }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "gaps failed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainMkdirOutputError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	mkdirAllHook = func(path string, perm os.FileMode) error {
		return fmt.Errorf("mkdir all err")
	}
	defer func() { mkdirAllHook = os.MkdirAll }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cannot create output dir") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainMarshalError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	jsonMarshalIndentHook = func(v any, prefix, indent string) ([]byte, error) {
		return nil, fmt.Errorf("marshal err")
	}
	defer func() { jsonMarshalIndentHook = json.MarshalIndent }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "marshal failed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainWriteFileError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	writeFileHook = func(name string, data []byte, perm os.FileMode) error {
		return fmt.Errorf("write err")
	}
	defer func() { writeFileHook = os.WriteFile }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./..."})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "write failed") {
		t.Errorf("output = %q", buf.String())
	}
}

func TestDrainEnqueueError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/demo\n\ngo 1.23\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reqDir := filepath.Join(tmp, ".sin-code/coverage-requests")
	if err := os.MkdirAll(reqDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reqDir, "pkg.json"), []byte(`{"package":"example.com/demo/pkg"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mkdirTempCmd = func(dir, pattern string) (string, error) {
		return t.TempDir(), nil
	}
	defer func() { mkdirTempCmd = os.MkdirTemp }()
	runGoTestHook = func(dir, packages, coverprofile string) ([]byte, error) {
		_ = os.WriteFile(coverprofile, []byte("mode: set\n"), 0o644)
		return []byte("ok  example.com/demo/pkg  0.010s  coverage: 50.0% of statements\n"), nil
	}
	defer func() { runGoTestHook = defaultRunGoTest }()
	EnqueueGoal = func(ctx context.Context, prompt, workspace string) error {
		return fmt.Errorf("enqueue err")
	}
	defer func() { EnqueueGoal = nil }()

	cmd := newDrainCmd()
	buf := &strings.Builder{}
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--root", tmp, "--packages", "./...", "--enqueue"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "enqueue example.com/demo/pkg failed") {
		t.Errorf("output = %q", buf.String())
	}
}
