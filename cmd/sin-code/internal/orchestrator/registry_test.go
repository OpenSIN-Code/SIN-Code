// SPDX-License-Identifier: MIT
// Purpose: tests for the agent registry's NIM/Mock factory wiring.
package orchestrator

import (
	"testing"
)

func TestUseNIM(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	if UseNIM() {
		t.Error("expected false with empty env")
	}
	t.Setenv("SIN_NIM_API_KEY", "abc")
	if !UseNIM() {
		t.Error("expected true with non-empty env")
	}
}

func TestUseLoopAgent(t *testing.T) {
	t.Setenv("SIN_LOOP_AGENTS", "")
	if UseLoopAgent() {
		t.Error("expected false with empty env")
	}
	t.Setenv("SIN_LOOP_AGENTS", "1")
	if !UseLoopAgent() {
		t.Error("expected true with SIN_LOOP_AGENTS=1")
	}
	t.Setenv("SIN_LOOP_AGENTS", "0")
	if UseLoopAgent() {
		t.Error("expected false with SIN_LOOP_AGENTS=0")
	}
}

func TestDefaultAgentFactoryNIM(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	t.Setenv("SIN_LOOP_AGENTS", "")
	defer t.Setenv("SIN_NIM_API_KEY", "")
	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku"}
	a := defaultAgentFactory(cfg)
	if _, ok := a.(*NIMAgent); !ok {
		t.Errorf("expected *NIMAgent, got %T", a)
	}
}

func TestDefaultAgentFactoryMock(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("SIN_LOOP_AGENTS", "")
	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku"}
	a := defaultAgentFactory(cfg)
	if _, ok := a.(*MockAgent); !ok {
		t.Errorf("expected *MockAgent, got %T", a)
	}
}

func TestDefaultAgentFactoryLoopAgent(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	t.Setenv("SIN_LOOP_AGENTS", "1")
	defer t.Setenv("SIN_NIM_API_KEY", "")
	defer t.Setenv("SIN_LOOP_AGENTS", "")
	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku"}
	a := defaultAgentFactory(cfg)
	if _, ok := a.(*LoopAgent); !ok {
		t.Errorf("expected *LoopAgent, got %T", a)
	}
}

func TestDefaultAgentFactoryLoopAgentFallsBackToNIM(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	t.Setenv("SIN_LOOP_AGENTS", "1")
	defer t.Setenv("SIN_NIM_API_KEY", "")
	defer t.Setenv("SIN_LOOP_AGENTS", "")
	cfg := AgentConfig{Name: "coder", Type: TaskCode, Model: "haiku", Provider: "no-such-provider"}
	a := defaultAgentFactory(cfg)
	// Unknown provider → defaultLLMClient returns nil → falls back to
	// LLMAgent (NIMAgent alias) since UseLLM() is still true via NIM key.
	if _, ok := a.(*NIMAgent); !ok {
		t.Errorf("expected *NIMAgent fallback, got %T", a)
	}
}

func TestNewRegistryWithDefaultsUsesNIM(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	t.Setenv("SIN_LOOP_AGENTS", "")
	defer t.Setenv("SIN_NIM_API_KEY", "")
	r := NewRegistryWithDefaults(nil)
	if len(r.List()) == 0 {
		t.Fatal("empty registry")
	}
	for _, cfg := range r.List() {
		a, ok := r.Get(cfg.Name)
		if !ok {
			t.Fatalf("missing %s", cfg.Name)
		}
		if _, isNIM := a.(*NIMAgent); !isNIM {
			t.Errorf("agent %s: expected *NIMAgent, got %T", cfg.Name, a)
		}
	}
}

func TestNewRegistryWithDefaultsUsesMock(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "")
	t.Setenv("SIN_LOOP_AGENTS", "")
	r := NewRegistryWithDefaults(nil)
	if len(r.List()) == 0 {
		t.Fatal("empty registry")
	}
	for _, cfg := range r.List() {
		a, ok := r.Get(cfg.Name)
		if !ok {
			t.Fatalf("missing %s", cfg.Name)
		}
		if _, isMock := a.(*MockAgent); !isMock {
			t.Errorf("agent %s: expected *MockAgent, got %T", cfg.Name, a)
		}
	}
}

func TestNewRegistryWithDefaultsUsesLoopAgent(t *testing.T) {
	t.Setenv("SIN_NIM_API_KEY", "test-key")
	t.Setenv("SIN_LOOP_AGENTS", "1")
	defer t.Setenv("SIN_NIM_API_KEY", "")
	defer t.Setenv("SIN_LOOP_AGENTS", "")
	r := NewRegistryWithDefaults(nil)
	if len(r.List()) == 0 {
		t.Fatal("empty registry")
	}
	for _, cfg := range r.List() {
		a, ok := r.Get(cfg.Name)
		if !ok {
			t.Fatalf("missing %s", cfg.Name)
		}
		if _, isLoop := a.(*LoopAgent); !isLoop {
			t.Errorf("agent %s: expected *LoopAgent, got %T", cfg.Name, a)
		}
	}
}
