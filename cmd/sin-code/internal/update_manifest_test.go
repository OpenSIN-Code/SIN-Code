// SPDX-License-Identifier: MIT
// Purpose: Unit tests for UpdateManifest serialize/deserialize.
package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateManifest_RoundTrip(t *testing.T) {
	td := t.TempDir()
	m := NewManifest("v3.9.0")
	m.Pre = UpdateSnapshot{
		PipxPackages: map[string]string{"sin-code-bundle": "1.2.0"},
		GoBins:       map[string]string{"discover": "v1.0.0"},
		SkillsDirs:   map[string]string{"sin-websearch": "/some/path"},
	}

	if err := m.Write(td); err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	loaded, err := ReadManifest(td)
	if err != nil {
		t.Fatalf("ReadManifest failed: %v", err)
	}

	if loaded.SinCodeVer != "v3.9.0" {
		t.Errorf("SinCodeVer = %q, want v3.9.0", loaded.SinCodeVer)
	}
	if loaded.Pre.PipxPackages["sin-code-bundle"] != "1.2.0" {
		t.Errorf("PipxPackages mismatch")
	}
	if loaded.Pre.GoBins["discover"] != "v1.0.0" {
		t.Errorf("GoBins mismatch")
	}
	if loaded.Timestamp == "" {
		t.Error("Timestamp should not be empty")
	}
}

func TestUpdateManifest_InvalidJSON(t *testing.T) {
	td := t.TempDir()
	// write corrupt JSON
	if err := os.WriteFile(filepath.Join(td, "manifest.json"), []byte("not-json{{"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := ReadManifest(td)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestUpdateManifest_MissingFile(t *testing.T) {
	td := t.TempDir()
	_, err := ReadManifest(td)
	if err == nil {
		t.Error("expected error for missing manifest")
	}
}

func TestUpdateManifest_WriteEmptyPre(t *testing.T) {
	td := t.TempDir()
	m := NewManifest("dev")
	m.Pre = UpdateSnapshot{}
	m.Success = true
	if err := m.Write(td); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	loaded, err := ReadManifest(td)
	if err != nil {
		t.Fatalf("ReadManifest failed: %v", err)
	}
	if !loaded.Success {
		t.Error("Success should be true")
	}
}

func TestUpdateManifest_JSONContent(t *testing.T) {
	td := t.TempDir()
	m := NewManifest("v4.0.0")
	m.GoVersion = "go1.25.11"
	m.OSArch = "darwin/arm64"
	if err := m.Write(td); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(td, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]interface{}
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if generic["success"] != false {
		t.Errorf("success default should be false")
	}
}
