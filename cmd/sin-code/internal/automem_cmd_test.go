// SPDX-License-Identifier: MIT
// Purpose: coverage tests for automem_cmd.go.
package internal

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/auto_mem"
)

func TestOpenAutoMem_Success(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	s, proj, err := openAutoMem()
	if err != nil {
		t.Fatalf("openAutoMem: %v", err)
	}
	if s == nil {
		t.Fatal("expected store")
	}
	if proj != "global" {
		t.Errorf("expected project 'global', got %q", proj)
	}
}

func TestOpenAutoMem_ProjectOverride(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	oldProject := autoMemProject
	autoMemProject = "myproject"
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
		autoMemProject = oldProject
	}()

	_, proj, err := openAutoMem()
	if err != nil {
		t.Fatalf("openAutoMem: %v", err)
	}
	if proj != "myproject" {
		t.Errorf("expected project 'myproject', got %q", proj)
	}
}

func TestOpenAutoMem_DefaultHomeError(t *testing.T) {
	oldHome := autoMemDefaultHome
	autoMemDefaultHome = func() (string, error) { return "", errors.New("home error") }
	defer func() { autoMemDefaultHome = oldHome }()

	_, _, err := openAutoMem()
	if err == nil || !strings.Contains(err.Error(), "home error") {
		t.Fatalf("expected home error, got %v", err)
	}
}

func TestOpenAutoMem_OpenError(t *testing.T) {
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return t.TempDir(), nil }
	autoMemOpen = func(string, string) (*auto_mem.Store, error) { return nil, errors.New("open error") }
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	_, _, err := openAutoMem()
	if err == nil || !strings.Contains(err.Error(), "open error") {
		t.Fatalf("expected open error, got %v", err)
	}
}

func captureAutoMemCmd(t *testing.T, cmd *cobra.Command, args []string) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	err := cmd.RunE(cmd, args)
	w.Close()
	os.Stdout = old
	out, _ := io.ReadAll(r)
	return string(out), err
}

func TestAutoMem_ListCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	out, err := captureAutoMemCmd(t, memAutoListCmd, []string{})
	if err != nil {
		t.Fatalf("memAutoListCmd: %v", err)
	}
	if !strings.Contains(out, "MEMORY.md") {
		t.Errorf("expected MEMORY.md in output, got %q", out)
	}
}

func TestAutoMem_ListCmdJSON(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	oldFormat := autoMemFormat
	autoMemFormat = "json"
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
		autoMemFormat = oldFormat
	}()

	out, err := captureAutoMemCmd(t, memAutoListCmd, []string{})
	if err != nil {
		t.Fatalf("memAutoListCmd: %v", err)
	}
	if !strings.Contains(out, "\"headings\"") {
		t.Errorf("expected JSON output, got %q", out)
	}
}

func TestAutoMem_ShowCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	store, _, err := openAutoMem()
	if err != nil {
		t.Fatalf("openAutoMem: %v", err)
	}
	if err := store.Append(auto_mem.Entry{Heading: "topic", Body: "body text"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	out, err := captureAutoMemCmd(t, memAutoShowCmd, []string{"topic"})
	if err != nil {
		t.Fatalf("memAutoShowCmd: %v", err)
	}
	if !strings.Contains(out, "body text") {
		t.Errorf("expected body text, got %q", out)
	}
}

func TestAutoMem_AppendCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	out, err := captureAutoMemCmd(t, memAutoAppendCmd, []string{"heading", "body"})
	if err != nil {
		t.Fatalf("memAutoAppendCmd: %v", err)
	}
	if !strings.Contains(out, "updated MEMORY.md topic") {
		t.Errorf("expected append output, got %q", out)
	}
}

func TestAutoMem_RmCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	store, _, err := openAutoMem()
	if err != nil {
		t.Fatalf("openAutoMem: %v", err)
	}
	if err := store.Append(auto_mem.Entry{Heading: "rm-topic", Body: "x"}); err != nil {
		t.Fatalf("append: %v", err)
	}

	out, err := captureAutoMemCmd(t, memAutoRmCmd, []string{"rm-topic"})
	if err != nil {
		t.Fatalf("memAutoRmCmd: %v", err)
	}
	if !strings.Contains(out, "removed topic") {
		t.Errorf("expected rm output, got %q", out)
	}
}

func TestAutoMem_GcCmd(t *testing.T) {
	tmpDir := t.TempDir()
	oldHome := autoMemDefaultHome
	oldOpen := autoMemOpen
	autoMemDefaultHome = func() (string, error) { return tmpDir, nil }
	autoMemOpen = auto_mem.Open
	defer func() {
		autoMemDefaultHome = oldHome
		autoMemOpen = oldOpen
	}()

	out, err := captureAutoMemCmd(t, memAutoGcCmd, []string{})
	if err != nil {
		t.Fatalf("memAutoGcCmd: %v", err)
	}
	if !strings.Contains(out, "rotated MEMORY.md") {
		t.Errorf("expected gc output, got %q", out)
	}
}
