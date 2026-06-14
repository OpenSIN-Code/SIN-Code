// SPDX-License-Identifier: MIT
// Purpose: tests for the binary resolver used by subcommand dispatch.
package internal

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBinary_SIN_CODE_BIN(t *testing.T) {
	old := os.Getenv("SIN_CODE_BIN")
	defer os.Setenv("SIN_CODE_BIN", old)

	want := "/custom/path/to/sin-code"
	os.Setenv("SIN_CODE_BIN", want)
	got, err := resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestResolveBinary_FromPATH(t *testing.T) {
	oldBin := os.Getenv("SIN_CODE_BIN")
	oldPath := os.Getenv("PATH")
	defer func() {
		os.Setenv("SIN_CODE_BIN", oldBin)
		os.Setenv("PATH", oldPath)
	}()
	os.Unsetenv("SIN_CODE_BIN")

	dir := t.TempDir()
	bin := filepath.Join(dir, "sin-code")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("write fake binary: %v", err)
	}
	os.Setenv("PATH", dir+":"+oldPath)

	got, err := resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary: %v", err)
	}
	if got != bin {
		t.Fatalf("expected %q, got %q", bin, got)
	}
}

func TestResolveBinary_ExecutableFallback(t *testing.T) {
	oldBin := os.Getenv("SIN_CODE_BIN")
	oldPath := os.Getenv("PATH")
	defer func() {
		os.Setenv("SIN_CODE_BIN", oldBin)
		os.Setenv("PATH", oldPath)
	}()
	os.Unsetenv("SIN_CODE_BIN")
	os.Setenv("PATH", "/dev/null")

	got, err := resolveBinary()
	if err != nil {
		t.Fatalf("resolveBinary: %v", err)
	}
	self, _ := os.Executable()
	if got != self {
		t.Fatalf("expected %q, got %q", self, got)
	}
}

func TestRunSinCodeCLI_ResolveBinaryError(t *testing.T) {
	old := osExecutable
	osExecutable = func() (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { osExecutable = old }()

	os.Unsetenv("SIN_CODE_BIN")
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/dev/null")
	defer os.Setenv("PATH", oldPath)

	_, err := runSinCodeCLI("todo", "list")
	if err == nil {
		t.Fatal("expected error when resolveBinary fails")
	}
	if !strings.Contains(err.Error(), "cannot resolve sin-code binary") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunSubcommandRaw_ResolveBinaryError(t *testing.T) {
	old := osExecutable
	osExecutable = func() (string, error) {
		return "", os.ErrNotExist
	}
	defer func() { osExecutable = old }()

	os.Unsetenv("SIN_CODE_BIN")
	oldPath := os.Getenv("PATH")
	os.Setenv("PATH", "/dev/null")
	defer os.Setenv("PATH", oldPath)

	_, err := runSubcommandRaw(context.Background(), []string{"discover", "."})
	if err == nil {
		t.Fatal("expected error when resolveBinary fails")
	}
	if !strings.Contains(err.Error(), "cannot find self") {
		t.Fatalf("unexpected error: %v", err)
	}
}
