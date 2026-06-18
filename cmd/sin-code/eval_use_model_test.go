// SPDX-License-Identifier: MIT
// Purpose: Coverage tests for issue #261 — the --use-model switch in
// eval run. The real chat is exercised elsewhere; here we only
// guarantee that the helper builds/validates the configuration path.
package main

import (
	"context"
	"os"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

func TestBuildEvalCompletion_MissingKey(t *testing.T) {
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("SIN_CODE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, err := buildEvalCompletion()
	if err == nil {
		t.Fatal("expected error when API key is missing")
	}
	if err.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestBuildEvalCompletion_MissingModel(t *testing.T) {
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("SIN_CODE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	_, err := buildEvalCompletion()
	if err == nil {
		t.Fatal("expected error when model is missing")
	}
}

func loadOrSkipConfig(t *testing.T) string {
	t.Helper()
	return os.Getenv("SIN_CODE_HOME")
}

func restoreEnv(t *testing.T) {
	t.Helper()
	_ = context.TODO()
	_ = session.Message{}
}
