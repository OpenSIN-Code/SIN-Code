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
		if p.InputPer1M <= 0 || p.OutputPer1M <= 0 {
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
		{Name: "test-model", Slug: "accounts/fireworks/models/test", InputPer1M: 1.0, OutputPer1M: 2.0, ContextLen: 131072},
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
	expected := []string{"minimax-m3", "kimi-k2p7-code", "kimi-k2p7-code-fast", "deepseek-v4-pro", "qwen-3p7-plus", "glm-5p2"}
	names := make(map[string]bool)
	for _, m := range DefaultFireworksLineup {
		names[m.Name] = true
		if m.InputPer1M <= 0 || m.OutputPer1M <= 0 {
			t.Errorf("model %s: zero pricing (input=%.2f, output=%.2f)", m.Name, m.InputPer1M, m.OutputPer1M)
		}
		if m.ContextLen <= 0 {
			t.Errorf("model %s: zero context length", m.Name)
		}
		if !m.Thinking {
			t.Errorf("model %s: thinking should be enabled", m.Name)
		}
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

func TestDefaultFireworksLineup_CorrectPricing(t *testing.T) {
	// Prices verified from fireworks.ai model pages (2026-06-17).
	expected := map[string]struct{ Input, Output float64 }{
		"minimax-m3":         {0.30, 1.20},
		"kimi-k2p7-code":     {0.95, 4.00},
		"kimi-k2p7-code-fast": {0.95, 4.00},
		"deepseek-v4-pro":    {1.74, 3.48},
		"qwen-3p7-plus":      {0.40, 1.60},
		"glm-5p2":            {1.40, 4.40},
	}
	for _, m := range DefaultFireworksLineup {
		exp, ok := expected[m.Name]
		if !ok {
			continue
		}
		if m.InputPer1M != exp.Input {
			t.Errorf("model %s: input price %.2f, expected %.2f", m.Name, m.InputPer1M, exp.Input)
		}
		if m.OutputPer1M != exp.Output {
			t.Errorf("model %s: output price %.2f, expected %.2f", m.Name, m.OutputPer1M, exp.Output)
		}
	}
}

func TestDefaultFireworksLineup_CorrectContextLengths(t *testing.T) {
	expected := map[string]int{
		"minimax-m3":         524288,
		"kimi-k2p7-code":     262144,
		"kimi-k2p7-code-fast": 262144,
		"deepseek-v4-pro":    1048576,
		"qwen-3p7-plus":      262144,
		"glm-5p2":            1048576,
	}
	for _, m := range DefaultFireworksLineup {
		exp, ok := expected[m.Name]
		if !ok {
			continue
		}
		if m.ContextLen != exp {
			t.Errorf("model %s: context %d, expected %d", m.Name, m.ContextLen, exp)
		}
	}
}

func TestDefaultFireworksLineup_VisionCapabilities(t *testing.T) {
	expected := map[string]bool{
		"minimax-m3":         true,
		"kimi-k2p7-code":     true,
		"kimi-k2p7-code-fast": true,
		"deepseek-v4-pro":    false,
		"qwen-3p7-plus":      true,
		"glm-5p2":            false,
	}
	for _, m := range DefaultFireworksLineup {
		exp, ok := expected[m.Name]
		if !ok {
			continue
		}
		if m.Vision != exp {
			t.Errorf("model %s: vision %v, expected %v", m.Name, m.Vision, exp)
		}
	}
}

func startsWith(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
