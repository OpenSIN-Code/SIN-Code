// SPDX-License-Identifier: MIT
package fusion

import (
	"os"
	"testing"
)

func TestLoadFireworksPool_AllModels(t *testing.T) {
	pool := LoadFireworksPool(nil, nil)
	if len(pool) != len(DefaultFireworksLineup) {
		t.Fatalf("expected %d models, got %d", len(DefaultFireworksLineup), len(pool))
	}

	names := make(map[string]bool)
	for _, p := range pool {
		names[p.Name] = true
		if p.BaseURL == "" {
			t.Errorf("model %s: empty base_url", p.Name)
		}
		if p.Model == "" {
			t.Errorf("model %s: empty model slug", p.Name)
		}
		if p.PricePer1MTok <= 0 {
			t.Errorf("model %s: zero price", p.Name)
		}
	}

	for _, m := range DefaultFireworksLineup {
		if !names[m.Name] {
			t.Errorf("expected model %s in pool", m.Name)
		}
	}
}

func TestLoadFireworksPool_Filtered(t *testing.T) {
	pool := LoadFireworksPool(nil, []string{"minimax-m3", "glm-5p2"})
	if len(pool) != 2 {
		t.Fatalf("expected 2 models, got %d", len(pool))
	}
	if pool[0].Name != "minimax-m3" && pool[1].Name != "minimax-m3" {
		t.Error("expected minimax-m3 in filtered pool")
	}
	if pool[0].Name != "glm-5p2" && pool[1].Name != "glm-5p2" {
		t.Error("expected glm-5p2 in filtered pool")
	}
}

func TestLoadFireworksPool_CustomLineup(t *testing.T) {
	custom := []FireworksModel{
		{Name: "test-model", Slug: "accounts/fireworks/models/test", PricePer1MT: 2.0, ContextLen: 131072},
	}
	pool := LoadFireworksPool(custom, nil)
	if len(pool) != 1 {
		t.Fatalf("expected 1 model, got %d", len(pool))
	}
	if pool[0].Name != "test-model" {
		t.Errorf("expected test-model, got %s", pool[0].Name)
	}
	if pool[0].Model != "accounts/fireworks/models/test" {
		t.Errorf("expected test slug, got %s", pool[0].Model)
	}
}

func TestLoadFireworksPool_BaseURLFromEnv(t *testing.T) {
	original := os.Getenv("FIREWORKS_BASE_URL")
	defer func() {
		if original != "" {
			os.Setenv("FIREWORKS_BASE_URL", original)
		} else {
			os.Unsetenv("FIREWORKS_BASE_URL")
		}
	}()

	os.Setenv("FIREWORKS_BASE_URL", "https://custom-fireworks.example.com/v1")
	pool := LoadFireworksPool(nil, []string{"minimax-m3"})
	if len(pool) != 1 {
		t.Fatalf("expected 1 model, got %d", len(pool))
	}
	if pool[0].BaseURL != "https://custom-fireworks.example.com/v1" {
		t.Errorf("expected custom base URL from env, got %s", pool[0].BaseURL)
	}
}

func TestLoadFireworksPool_DefaultBaseURL(t *testing.T) {
	original := os.Getenv("FIREWORKS_BASE_URL")
	defer func() {
		if original != "" {
			os.Setenv("FIREWORKS_BASE_URL", original)
		} else {
			os.Unsetenv("FIREWORKS_BASE_URL")
		}
	}()

	os.Unsetenv("FIREWORKS_BASE_URL")
	pool := LoadFireworksPool(nil, []string{"minimax-m3"})
	if len(pool) != 1 {
		t.Fatalf("expected 1 model, got %d", len(pool))
	}
	if pool[0].BaseURL != "https://sinatorpool-router.delqhi.com/inference/v1" {
		t.Errorf("expected SINator pool router URL, got %s", pool[0].BaseURL)
	}
}

func TestDefaultFireworksLineup_ContainsKeyModels(t *testing.T) {
	expected := []string{"minimax-m3", "kimi-k2p7-code", "deepseek-v4-pro", "qwen-3p7-plus", "glm-5p2"}
	names := make(map[string]bool)
	for _, m := range DefaultFireworksLineup {
		names[m.Name] = true
	}
	for _, e := range expected {
		if !names[e] {
			t.Errorf("expected %s in DefaultFireworksLineup", e)
		}
	}
}

func TestDefaultFireworksLineup_AllSlugsAreFireworks(t *testing.T) {
	for _, m := range DefaultFireworksLineup {
		if m.Slug == "" {
			t.Errorf("model %s: empty slug", m.Name)
			continue
		}
		if !startsWith(m.Slug, "accounts/fireworks/") {
			t.Errorf("model %s: slug %q does not start with 'accounts/fireworks/'", m.Name, m.Slug)
		}
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
