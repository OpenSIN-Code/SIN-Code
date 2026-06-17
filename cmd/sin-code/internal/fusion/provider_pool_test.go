// SPDX-License-Identifier: MIT
package fusion

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestLoadProviderPool_AllProfiles(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "profiles")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("profiles dir not found: %s", dir)
	}

	pool, err := LoadProviderPool(dir, nil)
	if err != nil {
		t.Fatalf("LoadProviderPool: %v", err)
	}
	if len(pool) < 2 {
		t.Errorf("expected at least 2 profiles, got %d", len(pool))
	}

	names := make(map[string]bool)
	for _, p := range pool {
		names[p.Name] = true
		if p.Model == "" {
			t.Errorf("profile %s: empty model", p.Name)
		}
		if p.BaseURL == "" {
			t.Errorf("profile %s: empty base_url", p.Name)
		}
	}
	if !names["fireworks"] {
		t.Error("expected fireworks profile")
	}
	if !names["qwen-relay"] {
		t.Error("expected qwen-relay profile")
	}
}

func TestLoadProviderPool_Filtered(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "..", "profiles")
	if _, err := os.Stat(dir); err != nil {
		t.Skipf("profiles dir not found: %s", dir)
	}

	pool, err := LoadProviderPool(dir, []string{"fireworks"})
	if err != nil {
		t.Fatalf("LoadProviderPool: %v", err)
	}
	if len(pool) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(pool))
	}
	if pool[0].Name != "fireworks" {
		t.Errorf("expected fireworks, got %s", pool[0].Name)
	}
}

func TestLoadProviderPool_NonexistentDir(t *testing.T) {
	_, err := LoadProviderPool("/nonexistent/path", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestShouldTournament_StructuralFailure(t *testing.T) {
	tests := []struct {
		report string
		want   bool
	}{
		{"compile error: undefined variable x", true},
		{"build failed: missing dependency", true},
		{"tests failed: 3 of 10 tests failed", true},
		{"syntax error: unexpected token", true},
		{"type error: cannot use string as int", true},
		{"panic: nil pointer dereference", true},
		{"style: naming convention violation", false},
		{"format: incorrect indentation", false},
		{"documentation: missing comment", false},
		{"unknown edge case in module X", true},
		{"", true},
	}

	for _, tt := range tests {
		vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: tt.report}
		got := ShouldTournament(vr)
		if got != tt.want {
			t.Errorf("ShouldTournament(report=%q) = %v, want %v", tt.report, got, tt.want)
		}
	}
}

func TestShouldTournament_PassedResult(t *testing.T) {
	vr := verify.Result{Passed: true, Mode: verify.ModePoC, Report: "all good"}
	if ShouldTournament(vr) {
		t.Error("expected false for passed result")
	}
}
