// SPDX-License-Identifier: MIT
// Purpose: tests for issue #356 — native Honcho integration.
package memory

import (
	"context"
	"testing"
)

func TestHonchoNilStoreGraceful(t *testing.T) {
	h := NewHonchoIntegration(nil)
	ctx := context.Background()

	prefs, err := h.GetUserPreferences(ctx)
	if err != nil {
		t.Errorf("GetUserPreferences with nil store: %v", err)
	}
	if prefs != nil {
		t.Errorf("expected nil prefs, got %v", prefs)
	}

	model, err := h.GetPeerModel(ctx, "user1")
	if err != nil {
		t.Errorf("GetPeerModel with nil store: %v", err)
	}
	if model != nil {
		t.Errorf("expected nil model, got %v", model)
	}

	if err := h.SavePreference(ctx, "k", "v"); err != nil {
		t.Errorf("SavePreference with nil store: %v", err)
	}

	if err := h.SavePeerModel(ctx, "user1", map[string]any{"a": 1}); err != nil {
		t.Errorf("SavePeerModel with nil store: %v", err)
	}
}

func TestHonchoNilReceiverGraceful(t *testing.T) {
	var h *HonchoIntegration
	ctx := context.Background()

	if _, err := h.GetUserPreferences(ctx); err != nil {
		t.Errorf("GetUserPreferences on nil receiver: %v", err)
	}
	if _, err := h.GetPeerModel(ctx, "u"); err != nil {
		t.Errorf("GetPeerModel on nil receiver: %v", err)
	}
	if err := h.SavePreference(ctx, "k", "v"); err != nil {
		t.Errorf("SavePreference on nil receiver: %v", err)
	}
	if err := h.SavePeerModel(ctx, "u", nil); err != nil {
		t.Errorf("SavePeerModel on nil receiver: %v", err)
	}
}

func TestHonchoSaveAndGetPreferences(t *testing.T) {
	s := tempStore(t)
	h := NewHonchoIntegration(s)
	ctx := context.Background()

	if err := h.SavePreference(ctx, "language", "Go"); err != nil {
		t.Fatal(err)
	}
	if err := h.SavePreference(ctx, "editor", "vim"); err != nil {
		t.Fatal(err)
	}

	prefs, err := h.GetUserPreferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 2 {
		t.Fatalf("expected 2 preferences, got %d", len(prefs))
	}

	found := map[string]bool{}
	for _, p := range prefs {
		if key, val, ok := ParsePreference(p); ok {
			found[key+":"+val] = true
		}
	}
	if !found["language:Go"] {
		t.Error("expected language:Go preference")
	}
	if !found["editor:vim"] {
		t.Error("expected editor:vim preference")
	}
}

func TestHonchoSaveAndGetPeerModel(t *testing.T) {
	s := tempStore(t)
	h := NewHonchoIntegration(s)
	ctx := context.Background()

	model := map[string]any{
		"communication_style": "concise",
		"expertise":           []any{"Go", "systems"},
		"preferred_depth":     3,
	}
	if err := h.SavePeerModel(ctx, "user-42", model); err != nil {
		t.Fatal(err)
	}

	got, err := h.GetPeerModel(ctx, "user-42")
	if err != nil {
		t.Fatal(err)
	}
	if got["communication_style"] != "concise" {
		t.Errorf("communication_style = %v, want concise", got["communication_style"])
	}
	if got["preferred_depth"] != float64(3) {
		t.Errorf("preferred_depth = %v, want 3", got["preferred_depth"])
	}
}

func TestHonchoGetPeerModelNotFound(t *testing.T) {
	s := tempStore(t)
	h := NewHonchoIntegration(s)
	ctx := context.Background()

	got, err := h.GetPeerModel(ctx, "nonexistent")
	if err != nil {
		t.Errorf("expected nil error for missing peer model, got %v", err)
	}
	if got != nil {
		t.Errorf("expected nil model for missing user, got %v", got)
	}
}

func TestHonchoGetUserPreferencesEmpty(t *testing.T) {
	s := tempStore(t)
	h := NewHonchoIntegration(s)
	ctx := context.Background()

	prefs, err := h.GetUserPreferences(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(prefs) != 0 {
		t.Errorf("expected 0 preferences, got %d", len(prefs))
	}
}

func TestHonchoFormatParsePreference(t *testing.T) {
	formatted := FormatPreference("theme", "dark")
	if formatted != "theme: dark" {
		t.Errorf("FormatPreference = %q, want 'theme: dark'", formatted)
	}

	key, val, ok := ParsePreference("theme: dark")
	if !ok {
		t.Fatal("ParsePreference returned ok=false")
	}
	if key != "theme" || val != "dark" {
		t.Errorf("ParsePreference = (%q, %q), want (theme, dark)", key, val)
	}

	_, _, ok = ParsePreference("no colon here")
	if ok {
		t.Error("expected ok=false for string without colon")
	}
}

func TestHonchoPreferencesIsolatedFromPeerModels(t *testing.T) {
	s := tempStore(t)
	h := NewHonchoIntegration(s)
	ctx := context.Background()

	_ = h.SavePreference(ctx, "k", "v")
	_ = h.SavePeerModel(ctx, "u", map[string]any{"x": 1})

	prefs, _ := h.GetUserPreferences(ctx)
	if len(prefs) != 1 {
		t.Errorf("expected 1 preference, got %d", len(prefs))
	}

	model, _ := h.GetPeerModel(ctx, "u")
	if model == nil {
		t.Fatal("expected peer model")
	}
	if model["x"] == nil {
		t.Error("expected x in peer model")
	}
}
