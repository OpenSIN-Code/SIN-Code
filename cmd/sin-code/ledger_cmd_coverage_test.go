// SPDX-License-Identifier: MIT
// Purpose: coverage tests for ledger_cmd.go — exercise every statement in
// ledgerStore, ledger list, and ledger show without touching the real ledger.
package main

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func TestLedgerStore_UsesEnvPath(t *testing.T) {
	orig := ledgerOpenFn
	defer func() { ledgerOpenFn = orig }()

	var gotPath string
	ledgerOpenFn = func(path string) (*ledger.Store, error) {
		gotPath = path
		return nil, errors.New("stub")
	}

	dir := t.TempDir()
	want := filepath.Join(dir, "env-ledger.db")
	t.Setenv("SIN_CODE_LEDGER", want)
	_, _ = ledgerStore()
	if gotPath != want {
		t.Errorf("ledgerStore path = %q, want %q", gotPath, want)
	}
}

func TestLedgerStore_OpenError(t *testing.T) {
	orig := ledgerOpenFn
	defer func() { ledgerOpenFn = orig }()

	ledgerOpenFn = func(string) (*ledger.Store, error) {
		return nil, errors.New("open failed")
	}

	t.Setenv("SIN_CODE_LEDGER", "")
	cmd := newLedgerListCmd()
	err := cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "open failed") {
		t.Errorf("expected open error, got %v", err)
	}
}

func TestLedgerListCmd_Empty(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerListCmd()
	outFn := captureStdout(t)
	err = cmd.RunE(cmd, []string{})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out); got != "No sessions recorded." {
		t.Errorf("output = %q, want \"No sessions recorded.\"", got)
	}
}

func TestLedgerListCmd_WithSessions(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "sess-1", Type: ledger.TypeUserPrompt, Summary: "hello", Data: map[string]any{}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerListCmd()
	outFn := captureStdout(t)
	err = cmd.RunE(cmd, []string{})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "sess-1") {
		t.Errorf("output missing sess-1: %q", out)
	}
}

func TestLedgerListCmd_SessionsError(t *testing.T) {
	orig := ledgerSessionsFn
	defer func() { ledgerSessionsFn = orig }()

	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	store.Close()

	ledgerSessionsFn = func(context.Context, *ledger.Store, int) ([]string, error) {
		return nil, errors.New("sessions failed")
	}

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerListCmd()
	err = cmd.RunE(cmd, []string{})
	if err == nil || !strings.Contains(err.Error(), "sessions failed") {
		t.Errorf("expected sessions error, got %v", err)
	}
}

func TestLedgerShowCmd_Empty(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerShowCmd()
	outFn := captureStdout(t)
	err = cmd.RunE(cmd, []string{"sess-x"})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out); got != "No ledger entries for this session." {
		t.Errorf("output = %q, want \"No ledger entries for this session.\"", got)
	}
}

func TestLedgerShowCmd_WithEntries(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "sess-2", Type: ledger.TypeToolCall, Summary: "did work", Data: map[string]any{"tool": "scout"}}); err != nil {
		t.Fatalf("record: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerShowCmd()
	outFn := captureStdout(t)
	err = cmd.RunE(cmd, []string{"sess-2"})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "tool_call") || !strings.Contains(out, "did work") {
		t.Errorf("output missing expected fields: %q", out)
	}
}

func TestLedgerShowCmd_ListError(t *testing.T) {
	orig := ledgerListFn
	defer func() { ledgerListFn = orig }()

	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	store.Close()

	ledgerListFn = func(context.Context, *ledger.Store, string, int) ([]ledger.Entry, error) {
		return nil, errors.New("list failed")
	}

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := newLedgerShowCmd()
	err = cmd.RunE(cmd, []string{"sess-3"})
	if err == nil || !strings.Contains(err.Error(), "list failed") {
		t.Errorf("expected list error, got %v", err)
	}
}

func TestLedgerShowCmd_OpenError(t *testing.T) {
	orig := ledgerOpenFn
	defer func() { ledgerOpenFn = orig }()

	ledgerOpenFn = func(string) (*ledger.Store, error) {
		return nil, errors.New("open failed")
	}

	cmd := newLedgerShowCmd()
	err := cmd.RunE(cmd, []string{"sess-4"})
	if err == nil || !strings.Contains(err.Error(), "open failed") {
		t.Errorf("expected open error, got %v", err)
	}
}

func TestLedgerNewLedgerCmd(t *testing.T) {
	cmd := NewLedgerCmd()
	if cmd.Name() != "ledger" {
		t.Errorf("name = %q, want ledger", cmd.Name())
	}
	if len(cmd.Commands()) != 2 {
		t.Errorf("expected 2 subcommands, got %d", len(cmd.Commands()))
	}
}

// ensure fmt import remains used when tests are edited.
var _ = fmt.Sprintf
