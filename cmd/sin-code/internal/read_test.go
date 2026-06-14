// SPDX-License-Identifier: MIT
// Purpose: coverage tests for read.go (st-cov2).
package internal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func captureReadCmd(t *testing.T, args []string, setup func()) string {
	t.Helper()
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	setup()
	err := ReadCmd.RunE(ReadCmd, args)
	w.Close()
	os.Stdout = oldStdout
	out, _ := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadCmd failed: %v", err)
	}
	return string(out)
}

func TestReadCmd_RunE_Text(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\n"), 0644)

	oldMode := readMode
	oldFormat := readFormat
	defer func() { readMode = oldMode; readFormat = oldFormat }()
	readMode = "raw"
	readFormat = "text"

	out := captureReadCmd(t, []string{p}, func() {})
	if !strings.Contains(out, "package main") {
		t.Errorf("expected file content, got %q", out)
	}
}

func TestReadCmd_RunE_JSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("package main\n"), 0644)

	oldFormat := readFormat
	defer func() { readFormat = oldFormat }()
	readFormat = "json"

	out := captureReadCmd(t, []string{p}, func() {})
	if !strings.Contains(out, `"path"`) {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestReadCmd_RunE_Truncated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a\nb\nc\nd\n"), 0644)

	oldLimit := readLimit
	oldFormat := readFormat
	defer func() { readLimit = oldLimit; readFormat = oldFormat }()
	readLimit = 2
	readFormat = "text"

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	outR, outW, _ := os.Pipe()
	errR, errW, _ := os.Pipe()
	os.Stdout = outW
	os.Stderr = errW
	err := ReadCmd.RunE(ReadCmd, []string{p})
	outW.Close()
	errW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr
	if err != nil {
		t.Fatalf("ReadCmd failed: %v", err)
	}
	stdout, _ := io.ReadAll(outR)
	stderr, _ := io.ReadAll(errR)
	if !bytes.Contains(stderr, []byte("truncated")) {
		t.Errorf("expected truncated stderr, got stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestReadFile_NotFound(t *testing.T) {
	_, err := readFile("/nonexistent/file.go", "raw", 1, 100, 1<<20)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestReadFile_Directory(t *testing.T) {
	dir := t.TempDir()
	_, err := readFile(dir, "raw", 1, 100, 1<<20)
	if err == nil {
		t.Fatal("expected error for directory")
	}
}

func TestReadFile_TooLarge(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "big.bin")
	os.WriteFile(p, make([]byte, 1024), 0644)
	_, err := readFile(p, "raw", 1, 100, 512)
	if err == nil {
		t.Fatal("expected error for file over max-bytes")
	}
}

func TestReadFile_OffsetBeyondEnd_Cli(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a\n"), 0644)
	_, err := readFile(p, "raw", 10, 100, 1<<20)
	if err == nil {
		t.Fatal("expected error for offset beyond end")
	}
}

func TestReadFile_UnknownMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.go")
	os.WriteFile(p, []byte("a\n"), 0644)
	_, err := readFile(p, "invalid", 1, 100, 1<<20)
	if err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestReadFile_Binary(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.bin")
	os.WriteFile(p, []byte{0xff, 0xfe}, 0644)
	_, err := readFile(p, "raw", 1, 100, 1<<20)
	if err == nil {
		t.Fatal("expected error for binary file")
	}
}
