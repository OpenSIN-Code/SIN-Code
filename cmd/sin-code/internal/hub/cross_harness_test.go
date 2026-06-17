// SPDX-License-Identifier: MIT
// Purpose: Tests for the cross-harness MCP inventory (issue #336).
package hub

import (
	"sync"
	"testing"
)

func sampleTools() map[string][]ToolSummary {
	return map[string][]ToolSummary{
		"sin-code": {
			{Name: "discover", Category: "core", Description: "Find files", ReadOnly: true},
			{Name: "scout", Category: "core", Description: "Search code", ReadOnly: true},
			{Name: "execute", Category: "exec", Description: "Run commands", ReadOnly: false},
		},
		"claude-code": {
			{Name: "discover", Category: "core", Description: "Find files", ReadOnly: true},
			{Name: "edit", Category: "code", Description: "Edit files", ReadOnly: false},
			{Name: "scout", Category: "core", Description: "Search code", ReadOnly: true},
		},
		"codex": {
			{Name: "discover", Category: "core", Description: "Find files", ReadOnly: true},
			{Name: "apply_patch", Category: "code", Description: "Apply patches", ReadOnly: false},
		},
	}
}

func TestRegisterAndAllTools(t *testing.T) {
	inv := NewCrossHarnessInventory()
	samples := sampleTools()
	for name, tools := range samples {
		inv.RegisterHarness(name, tools)
	}
	all := inv.AllTools()
	// unique names: discover, scout, execute, edit, apply_patch = 5
	if len(all) != 5 {
		t.Fatalf("want 5 unique tools, got %d: %+v", len(all), all)
	}
	names := make(map[string]bool)
	for _, tool := range all {
		if names[tool.Name] {
			t.Fatalf("duplicate in AllTools: %s", tool.Name)
		}
		names[tool.Name] = true
	}
	if !names["discover"] || !names["apply_patch"] {
		t.Fatal("missing expected tools")
	}
}

func TestCompare(t *testing.T) {
	inv := NewCrossHarnessInventory()
	samples := sampleTools()
	for name, tools := range samples {
		inv.RegisterHarness(name, tools)
	}
	diffs := inv.Compare("sin-code", "claude-code")
	diffMap := make(map[string]ToolDiff)
	for _, d := range diffs {
		diffMap[d.Tool] = d
	}
	if d := diffMap["discover"]; d.Diff != "both" {
		t.Fatalf("discover should be both: %+v", d)
	}
	if d := diffMap["execute"]; d.Diff != "only_a" || !d.InA || d.InB {
		t.Fatalf("execute should be only_a: %+v", d)
	}
	if d := diffMap["edit"]; d.Diff != "only_b" || d.InA || !d.InB {
		t.Fatalf("edit should be only_b: %+v", d)
	}
}

func TestUnique(t *testing.T) {
	inv := NewCrossHarnessInventory()
	samples := sampleTools()
	for name, tools := range samples {
		inv.RegisterHarness(name, tools)
	}
	// sin-code unique: execute (only in sin-code)
	uniq := inv.Unique("sin-code")
	if len(uniq) != 1 || uniq[0].Name != "execute" {
		t.Fatalf("want [execute], got %+v", uniq)
	}
	// codex unique: apply_patch
	uniq = inv.Unique("codex")
	if len(uniq) != 1 || uniq[0].Name != "apply_patch" {
		t.Fatalf("want [apply_patch], got %+v", uniq)
	}
}

func TestCommon(t *testing.T) {
	inv := NewCrossHarnessInventory()
	samples := sampleTools()
	for name, tools := range samples {
		inv.RegisterHarness(name, tools)
	}
	// Only "discover" is in all 3 harnesses
	common := inv.Common()
	if len(common) != 1 || common[0].Name != "discover" {
		t.Fatalf("want [discover], got %+v", common)
	}
}

func TestCommonSingleHarness(t *testing.T) {
	inv := NewCrossHarnessInventory()
	tools := []ToolSummary{
		{Name: "a", Category: "c", Description: "d"},
		{Name: "b", Category: "c", Description: "d"},
	}
	inv.RegisterHarness("only", tools)
	common := inv.Common()
	if len(common) != 2 {
		t.Fatalf("single harness: want 2, got %d", len(common))
	}
}

func TestUniqueUnknownHarness(t *testing.T) {
	inv := NewCrossHarnessInventory()
	inv.RegisterHarness("sin-code", sampleTools()["sin-code"])
	if u := inv.Unique("nonexistent"); u != nil {
		t.Fatalf("unknown harness should return nil, got %+v", u)
	}
}

func TestAllToolsEmpty(t *testing.T) {
	inv := NewCrossHarnessInventory()
	all := inv.AllTools()
	if len(all) != 0 {
		t.Fatalf("want empty, got %d", len(all))
	}
}

func TestHarnesses(t *testing.T) {
	inv := NewCrossHarnessInventory()
	samples := sampleTools()
	for name, tools := range samples {
		inv.RegisterHarness(name, tools)
	}
	names := inv.Harnesses()
	if len(names) != 3 {
		t.Fatalf("want 3 harnesses, got %d: %v", len(names), names)
	}
	if names[0] != "claude-code" || names[1] != "codex" || names[2] != "sin-code" {
		t.Fatalf("harnesses not sorted: %v", names)
	}
}

func TestRegisterHarnessReplaces(t *testing.T) {
	inv := NewCrossHarnessInventory()
	inv.RegisterHarness("h", []ToolSummary{{Name: "old"}})
	inv.RegisterHarness("h", []ToolSummary{{Name: "new"}})
	all := inv.AllTools()
	if len(all) != 1 || all[0].Name != "new" {
		t.Fatalf("re-register should replace: %+v", all)
	}
}

func TestConcurrentAccess(t *testing.T) {
	inv := NewCrossHarnessInventory()
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N * 2)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			inv.RegisterHarness("h", []ToolSummary{{Name: "tool"}})
		}(i)
		go func() {
			defer wg.Done()
			_ = inv.AllTools()
			_ = inv.Common()
			_ = inv.Harnesses()
		}()
	}
	wg.Wait()
}
