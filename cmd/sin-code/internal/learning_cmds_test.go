// SPDX-License-Identifier: MIT
// Purpose: coverage tests for learning_cmds.go.
package internal

import (
	"context"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
)

func TestDefaultEvalFactory(t *testing.T) {
	sub, scorer, err := defaultEvalFactory("test")
	if err != nil {
		t.Fatalf("defaultEvalFactory: %v", err)
	}
	if sub == nil {
		t.Fatal("expected non-nil subject")
	}
	if scorer == nil {
		t.Fatal("expected non-nil scorer")
	}
	_, ok := sub.(noopSubject)
	if !ok {
		t.Fatalf("expected noopSubject, got %T", sub)
	}
	_, ok = scorer.(evalharness.SuccessFlag)
	if !ok {
		t.Fatalf("expected SuccessFlag scorer, got %T", scorer)
	}
}

func TestNoopSubject_Run(t *testing.T) {
	out, err := noopSubject{}.Run(context.Background(), evalharness.EvalCase{})
	if err != nil {
		t.Fatalf("noopSubject.Run: %v", err)
	}
	if out.Success {
		t.Error("expected Success=false")
	}
	if !strings.Contains(out.Text, "no subject wired") {
		t.Errorf("expected no subject wired message, got %q", out.Text)
	}
}

func TestEvalSetCmd_Exists(t *testing.T) {
	if EvalSetCmd == nil {
		t.Fatal("EvalSetCmd is nil")
	}
	if EvalSetCmd.Use != "evalset" {
		t.Errorf("expected Use 'evalset', got %q", EvalSetCmd.Use)
	}
}

func TestInstinctCmd_Exists(t *testing.T) {
	if InstinctCmd == nil {
		t.Fatal("InstinctCmd is nil")
	}
}

func TestHooksCmd_Exists(t *testing.T) {
	if HooksCmd == nil {
		t.Fatal("HooksCmd is nil")
	}
}

func TestAssetsCmd_Exists(t *testing.T) {
	if AssetsCmd == nil {
		t.Fatal("AssetsCmd is nil")
	}
}

func TestPRPCmd_Exists(t *testing.T) {
	if PRPCmd == nil {
		t.Fatal("PRPCmd is nil")
	}
}
