// SPDX-License-Identifier: MIT
// Purpose: coverage tests for the remaining handleSbom branches in serve.go.
package internal

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleSbom_UnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	_, err := handleSbom(context.Background(), map[string]any{
		"path":   dir,
		"format": "unsupported-format",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestHandleSbom_AbsError(t *testing.T) {
	old := pathAbs
	pathAbs = func(string) (string, error) { return "", errors.New("injected abs error") }
	defer func() { pathAbs = old }()

	_, err := handleSbom(context.Background(), map[string]any{"path": "."})
	if err == nil || !strings.Contains(err.Error(), "injected abs error") {
		t.Fatalf("expected abs error, got %v", err)
	}
}

func TestHandleSbom_CreateError(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "outdir")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := handleSbom(context.Background(), map[string]any{
		"path":   dir,
		"format": "spdx-json",
		"output": outDir,
	})
	if err == nil {
		t.Fatal("expected error when output path is a directory")
	}
}

func TestHandleSbom_MarshalInlineError(t *testing.T) {
	old := sbomMarshalIndent
	sbomMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("injected marshal error")
	}
	defer func() { sbomMarshalIndent = old }()

	_, err := handleSbom(context.Background(), map[string]any{
		"path":   t.TempDir(),
		"format": "spdx-json",
		"output": "-",
	})
	if err == nil || !strings.Contains(err.Error(), "injected marshal error") {
		t.Fatalf("expected marshal error, got %v", err)
	}
}

func TestHandleSbom_EncodeError(t *testing.T) {
	old := sbomEncode
	sbomEncode = func(*json.Encoder, any) error {
		return errors.New("injected encode error")
	}
	defer func() { sbomEncode = old }()

	dir := t.TempDir()
	_, err := handleSbom(context.Background(), map[string]any{
		"path":   dir,
		"format": "spdx-json",
		"output": filepath.Join(dir, "sbom.json"),
	})
	if err == nil || !strings.Contains(err.Error(), "injected encode error") {
		t.Fatalf("expected encode error, got %v", err)
	}
}
