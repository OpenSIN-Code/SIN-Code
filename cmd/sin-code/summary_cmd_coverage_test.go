// SPDX-License-Identifier: MIT
// Purpose: coverage tests for summary_cmd.go — exercise every branch of the
// summary subcommand including markdown, evidence, and all error paths.
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
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
)

func makeSummaryStore(t *testing.T) string {
	t.Helper()
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "sess-sum",
		Type:      ledger.TypeUserPrompt,
		Summary:   "prompt",
		Data:      map[string]any{"content": "hello world"},
	}); err != nil {
		t.Fatalf("record prompt: %v", err)
	}
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "sess-sum",
		Type:      ledger.TypeToolCall,
		Summary:   "ran tool",
		Data:      map[string]any{"tool": "scout"},
	}); err != nil {
		t.Fatalf("record tool: %v", err)
	}
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "sess-sum",
		Type:      ledger.TypeVerifyPass,
		Summary:   "verified",
		Data:      map[string]any{"mode": "poc"},
	}); err != nil {
		t.Fatalf("record verify: %v", err)
	}
	store.Close()
	return db
}

func TestSummaryCmd_Markdown(t *testing.T) {
	db := makeSummaryStore(t)
	t.Setenv("SIN_CODE_LEDGER", db)

	cmd := NewSummaryCmd()
	outFn := captureStdout(t)
	err := cmd.RunE(cmd, []string{"sess-sum"})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Session Summary: sess-sum") {
		t.Errorf("output missing header: %q", out)
	}
	if !strings.Contains(out, "hello world") {
		t.Errorf("output missing prompt: %q", out)
	}
	if !strings.Contains(out, "scout") {
		t.Errorf("output missing tool: %q", out)
	}
}

func TestSummaryCmd_Evidence(t *testing.T) {
	db := makeSummaryStore(t)
	t.Setenv("SIN_CODE_LEDGER", db)

	cmd := NewSummaryCmd()
	cmd.Flags().Set("evidence", "true")
	outFn := captureStdout(t)
	err := cmd.RunE(cmd, []string{"sess-sum"})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "VERIFIED") {
		t.Errorf("evidence missing VERIFIED: %q", out)
	}
}

func TestSummaryCmd_LedgerOpenError(t *testing.T) {
	orig := summaryLedgerOpenFn
	defer func() { summaryLedgerOpenFn = orig }()

	summaryLedgerOpenFn = func(string) (*ledger.Store, error) {
		return nil, errors.New("open failed")
	}

	cmd := NewSummaryCmd()
	err := cmd.RunE(cmd, []string{"sess-x"})
	if err == nil || !strings.Contains(err.Error(), "open failed") {
		t.Errorf("expected open error, got %v", err)
	}
}

func TestSummaryCmd_BuildError(t *testing.T) {
	orig := summaryBuildFn
	defer func() { summaryBuildFn = orig }()

	db := makeSummaryStore(t)
	t.Setenv("SIN_CODE_LEDGER", db)

	summaryBuildFn = func(context.Context, *ledger.Store, string) (*summary.Summary, error) {
		return nil, errors.New("build failed")
	}

	cmd := NewSummaryCmd()
	err := cmd.RunE(cmd, []string{"sess-sum"})
	if err == nil || !strings.Contains(err.Error(), "build failed") {
		t.Errorf("expected build error, got %v", err)
	}
}

func TestSummaryCmd_EvidenceHook(t *testing.T) {
	db := makeSummaryStore(t)
	t.Setenv("SIN_CODE_LEDGER", db)

	origEvidence := summaryEvidenceFn
	origFormat := summaryFormatFn
	defer func() {
		summaryEvidenceFn = origEvidence
		summaryFormatFn = origFormat
	}()

	summaryEvidenceFn = func(s *summary.Summary) string { return "EVIDENCE:" + s.SessionID }
	summaryFormatFn = func(s *summary.Summary) string { return "FORMAT:" + s.SessionID }

	cmd := NewSummaryCmd()
	cmd.Flags().Set("evidence", "true")
	outFn := captureStdout(t)
	err := cmd.RunE(cmd, []string{"sess-sum"})
	out := outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out); got != "EVIDENCE:sess-sum" {
		t.Errorf("evidence output = %q, want EVIDENCE:sess-sum", got)
	}

	cmd = NewSummaryCmd()
	outFn = captureStdout(t)
	err = cmd.RunE(cmd, []string{"sess-sum"})
	out = outFn()
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got := strings.TrimSpace(out); got != "FORMAT:sess-sum" {
		t.Errorf("format output = %q, want FORMAT:sess-sum", got)
	}
}

func TestSummaryCmd_NoEntries(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	store.Close()

	t.Setenv("SIN_CODE_LEDGER", db)
	cmd := NewSummaryCmd()
	err = cmd.RunE(cmd, []string{"sess-empty"})
	if err == nil || !strings.Contains(err.Error(), "no ledger entries") {
		t.Errorf("expected no entries error, got %v", err)
	}
}

// keep fmt import valid across edits.
var _ = fmt.Sprintf
