// SPDX-License-Identifier: MIT
// Purpose: MCP handler tests for sin_sbom_generate (issue #36).
// All tests are hermetic — no network, no real tool lookups.
package internal

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestHandleSbom_SPDXGoProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/test\n\ngo 1.23\n\nrequire github.com/spf13/cobra v1.8.0\n"), 0644)

	ctx := context.Background()
	result, err := handleSbom(ctx, map[string]any{
		"path":   dir,
		"format": "spdx-json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc SPDXDocument
	if uerr := json.Unmarshal([]byte(result), &doc); uerr != nil {
		t.Fatalf("result is not valid SPDX JSON: %v\n%s", uerr, result[:min(len(result), 300)])
	}
	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("expected SPDXVersion=SPDX-2.3, got %s", doc.SPDXVersion)
	}
	if doc.SPDXID != "SPDXRef-DOCUMENT" {
		t.Errorf("expected SPDXID=SPDXRef-DOCUMENT, got %s", doc.SPDXID)
	}
	if len(doc.Packages) == 0 {
		t.Fatal("expected at least one package in SPDX document")
	}
}

func TestHandleSbom_CycloneDXPProject(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests==2.31.0\nflask>=3.0\n"), 0644)

	ctx := context.Background()
	result, err := handleSbom(ctx, map[string]any{
		"path":   dir,
		"format": "cyclonedx-json",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var doc CycloneDXDocument
	if uerr := json.Unmarshal([]byte(result), &doc); uerr != nil {
		t.Fatalf("result is not valid CycloneDX JSON: %v\n%s", uerr, result[:min(len(result), 300)])
	}
	if doc.BomFormat != "CycloneDX" {
		t.Errorf("expected BomFormat=CycloneDX, got %s", doc.BomFormat)
	}
	if doc.SpecVersion != "1.5" {
		t.Errorf("expected SpecVersion=1.5, got %s", doc.SpecVersion)
	}
	if len(doc.Components) == 0 {
		t.Fatal("expected at least one component in CycloneDX document")
	}
}

func TestHandleSbom_OutputPathEscape(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	_, err := handleSbom(ctx, map[string]any{
		"path":   dir,
		"format": "spdx-json",
		"output": "../../../tmp/escape.json",
	})
	if err == nil {
		t.Fatal("expected error for path-escape output")
	}
	if !strings.Contains(err.Error(), "escapes scan root") {
		t.Errorf("expected 'escapes scan root' in error, got: %v", err)
	}
}

func TestHandleSbom_OutputInsideRoot(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	result, err := handleSbom(ctx, map[string]any{
		"path":   dir,
		"format": "spdx-json",
		"output": filepath.Join(dir, "sbom.json"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "wrote SBOM to") {
		t.Errorf("expected 'wrote SBOM to' in result, got: %s", result)
	}

	// Verify file exists and is valid SPDX JSON
	data, rerr := os.ReadFile(filepath.Join(dir, "sbom.json"))
	if rerr != nil {
		t.Fatalf("output file not created: %v", rerr)
	}
	var doc SPDXDocument
	if uerr := json.Unmarshal(data, &doc); uerr != nil {
		t.Fatalf("output file is not valid SPDX JSON: %v", uerr)
	}
}

func TestHandleSbom_InlineDash(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "requirements.txt"), []byte("requests==2.31.0\n"), 0644)

	ctx := context.Background()
	result, err := handleSbom(ctx, map[string]any{
		"path":   dir,
		"format": "spdx-json",
		"output": "-",
	})
	if err != nil {
		t.Fatalf("unexpected error with output=-: %v", err)
	}
	var doc SPDXDocument
	if uerr := json.Unmarshal([]byte(result), &doc); uerr != nil {
		t.Fatalf("output=- should return inline JSON: %v", uerr)
	}
}

func TestHandleSbom_GenericProjectDefaults(t *testing.T) {
	dir := t.TempDir()

	ctx := context.Background()
	result, err := handleSbom(ctx, map[string]any{
		"path": dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var doc SPDXDocument
	if uerr := json.Unmarshal([]byte(result), &doc); uerr != nil {
		t.Fatalf("default format should return SPDX JSON: %v", uerr)
	}
}

func TestHandleSbom_RaceSafety(t *testing.T) {
	var wg sync.WaitGroup
	errCh := make(chan error, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			dir := t.TempDir()
			_, err := handleSbom(context.Background(), map[string]any{
				"path":   dir,
				"format": "spdx-json",
			})
			if err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("goroutine error: %v", err)
	}
}
