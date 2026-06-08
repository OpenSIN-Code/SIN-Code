package plugins

import (
	"testing"
)

func TestDebugLoadDir(t *testing.T) {
	plugins, err := LoadDir("/tmp/test_mcp/config/sin-code/plugins")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Loaded %d plugins", len(plugins))
	for _, p := range plugins {
		t.Logf("Plugin: %s, Enabled: %v, Path: %s, Tools: %d", p.Name, p.Enabled, p.Path, len(p.Tools))
		for _, t2 := range p.Tools {
			t.Logf("  Tool: %s, Binary: %s, Args: %v", t2.Name, t2.Binary, t2.Args)
		}
	}
}

func TestDebugLoadFromConfigDir(t *testing.T) {
	reg := NewRegistry()
	err := reg.LoadFromDir("/tmp/test_mcp/config")
	if err != nil {
		t.Fatal(err)
	}
	tools := reg.MCPTools()
	t.Logf("Found %d tools", len(tools))
	for _, tool := range tools {
		t.Logf("  %s: %s", tool.Name, tool.Description)
	}
	
	t.Logf("Loaded plugins: %d", len(reg.plugins))
	for name, p := range reg.plugins {
		t.Logf("Plugin: %s, Enabled: %v, Path: %s, Tools: %d", name, p.Enabled, p.Path, len(p.Tools))
		for _, t2 := range p.Tools {
			t.Logf("  Tool: %s, Binary: %s, Args: %v", t2.Name, t2.Binary, t2.Args)
		}
	}
}
