// SPDX-License-Identifier: MIT
// Purpose: Unit tests for writeFileAtomic and write CLI. (st-cov1)
package internal

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteFileAtomic_Backup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "file.go")
	os.WriteFile(p, []byte("package main\n"), 0o644)

	res, err := writeFileAtomic(p, "package main\nfunc main() {}\n", writeOpts{validate: true, backup: true})
	if err != nil {
		t.Fatalf("writeFileAtomic backup failed: %v", err)
	}
	if res.BackupPath == "" {
		t.Error("expected backup path")
	}
	if _, err := os.Stat(res.BackupPath); err != nil {
		t.Errorf("backup file missing: %v", err)
	}
}

func TestWriteFileAtomic_Mkdir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "file.go")

	res, err := writeFileAtomic(p, "package main\nfunc main() {}\n", writeOpts{validate: true, mkdir: true})
	if err != nil {
		t.Fatalf("writeFileAtomic mkdir failed: %v", err)
	}
	if !res.Created {
		t.Error("expected created flag true")
	}
	if _, err := os.Stat(p); err != nil {
		t.Errorf("file missing: %v", err)
	}
}

func TestWriteFileAtomic_MissingParentDir(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing", "file.go")

	if _, err := writeFileAtomic(p, "package main\n", writeOpts{validate: false, mkdir: false}); err == nil {
		t.Fatal("expected error for missing parent dir")
	}
}

func TestWriteFileAtomic_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")

	if _, err := writeFileAtomic(p, "{not json", writeOpts{validate: true}); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestWriteFileAtomic_BracketBalance(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "script.py")

	if _, err := writeFileAtomic(p, "print([1, 2)\n", writeOpts{validate: true}); err == nil {
		t.Fatal("expected error for unbalanced brackets")
	}
}

func TestWriteFileAtomic_NoValidate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "script.py")

	res, err := writeFileAtomic(p, "print([1, 2)\n", writeOpts{validate: false})
	if err != nil {
		t.Fatalf("writeFileAtomic no-validate failed: %v", err)
	}
	if res.Validated {
		t.Error("expected validated=false")
	}
}

func TestWriteFileAtomic_InvalidGo(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "main.go")

	if _, err := writeFileAtomic(p, "package main\nfunc\n", writeOpts{validate: true}); err == nil {
		t.Fatal("expected error for invalid Go syntax")
	}
}

func TestWriteFileAtomic_RenameFails(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "existing")
	os.Mkdir(p, 0o755)

	if _, err := writeFileAtomic(p, "content", writeOpts{validate: false}); err == nil {
		t.Fatal("expected error when destination is a directory")
	}
}

func TestWriteFileAtomic_ChmodOnReadOnlyDir(t *testing.T) {
	dir := t.TempDir()
	os.Chmod(dir, 0o555)
	defer os.Chmod(dir, 0o755)

	p := filepath.Join(dir, "file.go")
	if _, err := writeFileAtomic(p, "package main\n", writeOpts{validate: false}); err == nil {
		t.Fatal("expected error writing to read-only directory")
	}
}

func TestWriteCmd_ContentAndBackup(t *testing.T) {
	dir := t.TempDir()
	oldContent := writeContent
	oldFormat := writeFormat
	oldBackup := writeBackup
	defer func() {
		writeContent = oldContent
		writeFormat = oldFormat
		writeBackup = oldBackup
	}()

	p := filepath.Join(dir, "existing.txt")
	os.WriteFile(p, []byte("old"), 0o644)
	writeContent = "new"
	writeFormat = "text"
	writeBackup = true

	out := captureWriteCmd(t, []string{p}, func() {})
	if !strings.Contains(out, "backup") {
		t.Errorf("expected backup mention, got %q", out)
	}
	if _, err := os.Stat(p + ".bak"); err != nil {
		t.Errorf("backup file missing: %v", err)
	}
}

func captureWriteCmd(t *testing.T, args []string, setup func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	setup()
	err := WriteCmd.RunE(WriteCmd, args)

	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("WriteCmd failed: %v", err)
	}
	return string(out)
}

func TestWriteCmd_Stdin(t *testing.T) {
	dir := t.TempDir()
	oldStdin := os.Stdin
	oldStdinReader := writeStdinReader
	oldStdinContent := writeStdin
	oldFormat := writeFormat
	defer func() {
		os.Stdin = oldStdin
		writeStdinReader = oldStdinReader
		writeStdin = oldStdinContent
		writeFormat = oldFormat
	}()

	r, w, _ := os.Pipe()
	os.Stdin = r
	writeStdinReader = r
	go func() {
		w.WriteString("package main\nfunc main() {}\n")
		w.Close()
	}()

	writeStdin = true
	writeFormat = "text"
	p := filepath.Join(dir, "stdin.go")

	out := captureWriteCmd(t, []string{p}, func() {})
	if !strings.Contains(out, "wrote") {
		t.Errorf("expected write output, got %q", out)
	}
}

func TestWriteCmd_JSON(t *testing.T) {
	dir := t.TempDir()
	oldContent := writeContent
	oldFormat := writeFormat
	defer func() {
		writeContent = oldContent
		writeFormat = oldFormat
	}()

	writeContent = "package main\n"
	writeFormat = "json"
	p := filepath.Join(dir, "json.go")

	out := captureWriteCmd(t, []string{p}, func() {})
	if !strings.Contains(out, "{") {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestWriteCmd_InvalidAbsPath(t *testing.T) {
	if err := WriteCmd.RunE(WriteCmd, []string{"\x00invalid"}); err == nil {
		t.Fatal("expected error for invalid abs path")
	}
}

func TestWriteCmd_StdinError(t *testing.T) {
	oldReader := writeStdinReader
	oldStdinFlag := writeStdin
	defer func() {
		writeStdinReader = oldReader
		writeStdin = oldStdinFlag
	}()

	writeStdinReader = &errorReader{err: fmt.Errorf("simulated stdin error")}
	writeStdin = true

	if err := WriteCmd.RunE(WriteCmd, []string{"/tmp/should-not-write"}); err == nil {
		t.Fatal("expected error reading stdin")
	}
}

func TestWriteCmd_ValidateFailed(t *testing.T) {
	dir := t.TempDir()
	oldContent := writeContent
	oldNoValidate := writeNoValidate
	defer func() {
		writeContent = oldContent
		writeNoValidate = oldNoValidate
	}()

	writeContent = "package main\nfunc\n"
	writeNoValidate = false
	p := filepath.Join(dir, "bad.go")

	if err := WriteCmd.RunE(WriteCmd, []string{p}); err == nil {
		t.Fatal("expected validation error")
	}
}

type errorReader struct {
	err error
}

func (r *errorReader) Read(p []byte) (int, error) {
	return 0, r.err
}

func TestWriteFileAtomicWithHooks_CreateTempError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.createTemp = func(dir, pattern string) (*os.File, error) {
		return nil, fmt.Errorf("simulated create temp error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected create temp error")
	}
}

func TestWriteFileAtomicWithHooks_WriteError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.writeAll = func(w io.Writer, data []byte) (int, error) {
		return 0, fmt.Errorf("simulated write error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected write error")
	}
}

func TestWriteFileAtomicWithHooks_SyncError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.syncFile = func(f *os.File) error {
		return fmt.Errorf("simulated sync error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected sync error")
	}
}

func TestWriteFileAtomicWithHooks_CloseError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.closeFile = func(f *os.File) error {
		return fmt.Errorf("simulated close error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected close error")
	}
}

func TestWriteFileAtomicWithHooks_ChmodError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.chmod = func(name string, mode os.FileMode) error {
		return fmt.Errorf("simulated chmod error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected chmod error")
	}
}

func TestWriteFileAtomicWithHooks_RenameError(t *testing.T) {
	dir := t.TempDir()
	hooks := defaultWriteHooks
	hooks.rename = func(oldpath, newpath string) error {
		return fmt.Errorf("simulated rename error")
	}
	if _, err := writeFileAtomicWithHooks(filepath.Join(dir, "x.go"), "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected rename error")
	}
}

func TestWriteFileAtomicWithHooks_MkdirError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "parent", "x.go")
	hooks := defaultWriteHooks
	hooks.mkdirAll = func(path string, perm os.FileMode) error {
		return fmt.Errorf("simulated mkdir error")
	}
	if _, err := writeFileAtomicWithHooks(p, "package main\n", writeOpts{mkdir: true}, hooks); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestWriteFileAtomicWithHooks_StatDirError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing", "x.go")
	hooks := defaultWriteHooks
	hooks.stat = func(name string) (os.FileInfo, error) {
		if name == filepath.Dir(p) {
			return nil, fmt.Errorf("simulated stat error")
		}
		return os.Stat(name)
	}
	if _, err := writeFileAtomicWithHooks(p, "package main\n", writeOpts{}, hooks); err == nil {
		t.Fatal("expected stat dir error")
	}
}

func TestWriteFileAtomicWithHooks_BackupReadError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	os.WriteFile(p, []byte("package main\n"), 0o644)
	hooks := defaultWriteHooks
	hooks.readFile = func(name string) ([]byte, error) {
		return nil, fmt.Errorf("simulated read error")
	}
	if _, err := writeFileAtomicWithHooks(p, "package main\nfunc main() {}\n", writeOpts{backup: true}, hooks); err == nil {
		t.Fatal("expected backup read error")
	}
}

func TestWriteFileAtomicWithHooks_BackupWriteError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	os.WriteFile(p, []byte("package main\n"), 0o644)
	hooks := defaultWriteHooks
	hooks.writeFile = func(name string, data []byte, perm os.FileMode) error {
		return fmt.Errorf("simulated write backup error")
	}
	if _, err := writeFileAtomicWithHooks(p, "package main\nfunc main() {}\n", writeOpts{backup: true}, hooks); err == nil {
		t.Fatal("expected backup write error")
	}
}

func TestWriteFileAtomicWithHooks_StatExistingError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.go")
	hooks := defaultWriteHooks
	hooks.stat = func(name string) (os.FileInfo, error) {
		if name == p {
			return nil, fmt.Errorf("simulated stat error")
		}
		return os.Stat(name)
	}
	res, err := writeFileAtomicWithHooks(p, "package main\n", writeOpts{}, hooks)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Created {
		t.Error("expected created=true when stat returns error")
	}
}

func TestWriteFileAtomicWithHooks_ValidateOtherExtension(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "script.lua")
	res, err := writeFileAtomicWithHooks(p, "function x() return 1 end\n", writeOpts{validate: true}, defaultWriteHooks)
	if err != nil {
		t.Fatalf("unexpected error for balanced brackets: %v", err)
	}
	if !res.Validated {
		t.Error("expected validated=true")
	}
}

func TestCheckBracketBalance_Unclosed(t *testing.T) {
	if err := checkBracketBalance("x.txt", "foo(bar\n"); err == nil {
		t.Fatal("expected error for unclosed bracket")
	}
}

func TestCheckBracketBalance_SingleQuoteString(t *testing.T) {
	if err := checkBracketBalance("x.txt", "x = '(\ny'"); err != nil {
		t.Fatalf("single-quoted string should ignore bracket: %v", err)
	}
}

func TestCheckBracketBalance_EscapedQuote(t *testing.T) {
	if err := checkBracketBalance("x.txt", `foo("\"")`); err != nil {
		t.Fatalf("escaped quote should not unbalance: %v", err)
	}
}

func TestCheckBracketBalance_HashComment(t *testing.T) {
	if err := checkBracketBalance("x.py", "foo() #)\n"); err != nil {
		t.Fatalf("hash comment should ignore bracket: %v", err)
	}
}

func TestCheckBracketBalance_LineCommentReset(t *testing.T) {
	if err := checkBracketBalance("x.py", "#)\nfoo("); err == nil {
		t.Fatal("expected error for unclosed bracket after comment")
	}
}
