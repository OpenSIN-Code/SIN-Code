// SPDX-License-Identifier: MIT
// Purpose: tests for issue #357 — import/export MEMORY.md and ECC instinct JSON.
package memory

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExportToMEMORYMD(t *testing.T) {
	memories := []Memory{
		{Insight: "Use JWT for auth", Tags: []string{"auth"}, Created: time.Now(), Updated: time.Now()},
		{Insight: "Run tests with -race", Tags: []string{"testing"}, Created: time.Now(), Updated: time.Now()},
		{Insight: "OAuth2 callback must match", Tags: []string{"auth"}, Created: time.Now(), Updated: time.Now()},
	}
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := ExportToMEMORYMD(memories, path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	content := string(raw)
	if !strings.Contains(content, "# Memory") {
		t.Error("expected '# Memory' header")
	}
	if !strings.Contains(content, "## Tag: auth") {
		t.Error("expected '## Tag: auth' section")
	}
	if !strings.Contains(content, "## Tag: testing") {
		t.Error("expected '## Tag: testing' section")
	}
	if !strings.Contains(content, "- Use JWT for auth") {
		t.Error("expected JWT content line")
	}
}

func TestImportFromMEMORYMD(t *testing.T) {
	content := "# Memory\n\n## Tag: auth\n- Use JWT for auth\n- OAuth2 callback must match\n\n## Tag: testing\n- Run tests with -race\n"
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	memories, err := ImportFromMEMORYMD(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 3 {
		t.Fatalf("expected 3 memories, got %d", len(memories))
	}
	authCount := 0
	testCount := 0
	for _, m := range memories {
		for _, tag := range m.Tags {
			if tag == "auth" {
				authCount++
			}
			if tag == "testing" {
				testCount++
			}
		}
	}
	if authCount != 2 {
		t.Errorf("expected 2 auth memories, got %d", authCount)
	}
	if testCount != 1 {
		t.Errorf("expected 1 testing memory, got %d", testCount)
	}
}

func TestMEMORYMDRoundTrip(t *testing.T) {
	original := []Memory{
		{Insight: "Preference for concise code", Tags: []string{"style"}, Created: time.Now(), Updated: time.Now()},
		{Insight: "Always check nil before deref", Tags: []string{"safety"}, Created: time.Now(), Updated: time.Now()},
	}
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := ExportToMEMORYMD(original, path); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFromMEMORYMD(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != len(original) {
		t.Fatalf("round-trip: expected %d, got %d", len(original), len(imported))
	}
	importedSet := map[string]bool{}
	for _, m := range imported {
		importedSet[m.Insight] = true
	}
	for _, orig := range original {
		if !importedSet[orig.Insight] {
			t.Errorf("round-trip: missing %q", orig.Insight)
		}
	}
}

func TestExportToMEMORYMDUntagged(t *testing.T) {
	memories := []Memory{
		{Insight: "No tags here", Tags: nil, Created: time.Now(), Updated: time.Now()},
	}
	path := filepath.Join(t.TempDir(), "MEMORY.md")
	if err := ExportToMEMORYMD(memories, path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "## Tag: untagged") {
		t.Error("expected untagged section for memories without tags")
	}
}

func TestImportFromMEMORYMDNonexistent(t *testing.T) {
	_, err := ImportFromMEMORYMD("/nonexistent/path/MEMORY.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestExportToInstinct(t *testing.T) {
	instincts := []Instinct{
		{ID: "inst-1", Content: "Prefer composition over inheritance", Confidence: 0.9, Scope: "global", Source: "observation"},
		{ID: "inst-2", Content: "Always write tests first", Confidence: 0.7, Scope: "project", Source: "feedback"},
	}
	path := filepath.Join(t.TempDir(), "instincts.json")
	if err := ExportToInstinct(instincts, path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var formats []InstinctFormat
	if err := json.Unmarshal(raw, &formats); err != nil {
		t.Fatal(err)
	}
	if len(formats) != 2 {
		t.Fatalf("expected 2 instincts, got %d", len(formats))
	}
	if formats[0].Content != "Prefer composition over inheritance" {
		t.Errorf("content = %q", formats[0].Content)
	}
	if formats[0].Confidence != 0.9 {
		t.Errorf("confidence = %f", formats[0].Confidence)
	}
	if formats[0].Scope != "global" {
		t.Errorf("scope = %q", formats[0].Scope)
	}
}

func TestImportFromInstinct(t *testing.T) {
	formats := []InstinctFormat{
		{Content: "Use explicit error handling", Confidence: 0.85, Scope: "global", Source: "code-review"},
		{Content: "Avoid global state", Confidence: 0.6, Scope: "project", Source: "observation"},
	}
	raw, _ := json.Marshal(formats)
	path := filepath.Join(t.TempDir(), "instincts.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	instincts, err := ImportFromInstinct(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(instincts) != 2 {
		t.Fatalf("expected 2 instincts, got %d", len(instincts))
	}
	if instincts[0].Content != "Use explicit error handling" {
		t.Errorf("content = %q", instincts[0].Content)
	}
	if instincts[0].Confidence != 0.85 {
		t.Errorf("confidence = %f", instincts[0].Confidence)
	}
	if instincts[0].ID == "" {
		t.Error("expected ID to be generated")
	}
}

func TestInstinctRoundTrip(t *testing.T) {
	original := []Instinct{
		{Content: "Test instinct A", Confidence: 0.9, Scope: "global", Source: "obs"},
		{Content: "Test instinct B", Confidence: 0.5, Scope: "project", Source: "fb"},
	}
	path := filepath.Join(t.TempDir(), "instincts.json")
	if err := ExportToInstinct(original, path); err != nil {
		t.Fatal(err)
	}
	imported, err := ImportFromInstinct(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(imported) != len(original) {
		t.Fatalf("round-trip: expected %d, got %d", len(original), len(imported))
	}
	for i, orig := range original {
		if imported[i].Content != orig.Content {
			t.Errorf("round-trip [%d]: content %q != %q", i, imported[i].Content, orig.Content)
		}
		if imported[i].Confidence != orig.Confidence {
			t.Errorf("round-trip [%d]: confidence %f != %f", i, imported[i].Confidence, orig.Confidence)
		}
		if imported[i].Scope != orig.Scope {
			t.Errorf("round-trip [%d]: scope %q != %q", i, imported[i].Scope, orig.Scope)
		}
		if imported[i].Source != orig.Source {
			t.Errorf("round-trip [%d]: source %q != %q", i, imported[i].Source, orig.Source)
		}
	}
}

func TestImportFromInstinctDefaults(t *testing.T) {
	formats := []InstinctFormat{
		{Content: "No scope or source", Confidence: 0.5},
	}
	raw, _ := json.Marshal(formats)
	path := filepath.Join(t.TempDir(), "instincts.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	instincts, err := ImportFromInstinct(path)
	if err != nil {
		t.Fatal(err)
	}
	if instincts[0].Scope != "project" {
		t.Errorf("expected default scope 'project', got %q", instincts[0].Scope)
	}
	if instincts[0].Source != "observation" {
		t.Errorf("expected default source 'observation', got %q", instincts[0].Source)
	}
}

func TestInstinctFormatJSONShape(t *testing.T) {
	f := InstinctFormat{Content: "x", Confidence: 0.5, Scope: "global", Source: "test"}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	str := string(raw)
	for _, field := range []string{`"content"`, `"confidence"`, `"scope"`, `"source"`} {
		if !strings.Contains(str, field) {
			t.Errorf("expected %s in JSON: %s", field, str)
		}
	}
}
