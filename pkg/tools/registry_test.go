// SPDX-License-Identifier: MIT
package tools

import (
	"strings"
	"testing"
)

type fakeTool struct {
	meta ToolMetadata
	run  func(map[string]interface{}) (interface{}, error)
}

func (f *fakeTool) GetMetadata() ToolMetadata { return f.meta }
func (f *fakeTool) Execute(args map[string]interface{}) (interface{}, error) {
	if f.run != nil {
		return f.run(args)
	}
	return "ok", nil
}

func newTool(name string, caps ...string) *fakeTool {
	return &fakeTool{meta: ToolMetadata{
		Name:         name,
		Description:  name + " desc",
		Parameters:   map[string]interface{}{"x": "string"},
		Capabilities: caps,
	}}
}

func TestRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	if err := r.RegisterTool(newTool("write_file", "fs", "write")); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, ok := r.GetTool("write_file"); !ok {
		t.Error("expected to find write_file")
	}
	if _, ok := r.GetTool("nope"); ok {
		t.Error("did not expect to find nope")
	}
}

func TestRegisterDuplicateAndEmpty(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterTool(newTool("a"))
	if err := r.RegisterTool(newTool("a")); err == nil {
		t.Error("expected duplicate error")
	}
	if err := r.RegisterTool(newTool("")); err == nil {
		t.Error("expected empty-name error")
	}
}

func TestCapabilityIndex(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterTool(newTool("read", "fs"))
	_ = r.RegisterTool(newTool("write", "fs", "mutate"))
	got := r.ToolsByCapability("fs")
	if len(got) != 2 || got[0] != "read" || got[1] != "write" {
		t.Errorf("ToolsByCapability(fs) = %v", got)
	}
	if got := r.ToolsByCapability("mutate"); len(got) != 1 || got[0] != "write" {
		t.Errorf("ToolsByCapability(mutate) = %v", got)
	}
}

func TestGenerateAgentPromptDeterministic(t *testing.T) {
	r := NewRegistry()
	_ = r.RegisterTool(newTool("zeta"))
	_ = r.RegisterTool(newTool("alpha"))
	p1 := r.GenerateAgentPrompt()
	p2 := r.GenerateAgentPrompt()
	if p1 != p2 {
		t.Error("prompt is not deterministic across calls")
	}
	// alpha must appear before zeta (sorted)
	if strings.Index(p1, "alpha") > strings.Index(p1, "zeta") {
		t.Error("tools not sorted in prompt")
	}
	if !strings.Contains(p1, "AUTHORIZED SIN-CODE SYSTEM TOOLS") {
		t.Error("prompt missing header")
	}
}

func TestGetRegistrySingleton(t *testing.T) {
	if GetRegistry() != GetRegistry() {
		t.Error("GetRegistry should return the same instance")
	}
}
