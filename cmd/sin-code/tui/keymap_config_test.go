package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
)

func TestDefaultKeymapConfig(t *testing.T) {
	cfg := DefaultKeymapConfig()
	if cfg.Global == nil {
		t.Error("expected non-nil Global context")
	}
	if len(cfg.Global) == 0 {
		t.Error("expected bindings in Global context")
	}
	if _, ok := cfg.Global["quit"]; !ok {
		t.Error("expected 'quit' action in Global context")
	}
}

func TestDefaultKeymapConfigAllContexts(t *testing.T) {
	cfg := DefaultKeymapConfig()
	for _, ctx := range allContexts {
		bindings := cfg.Context(ctx)
		if bindings == nil {
			t.Errorf("expected non-nil bindings for context %s", ctx)
		}
	}
}

func TestKeymapConfigToKeymap(t *testing.T) {
	cfg := DefaultKeymapConfig()
	km := cfg.ToKeymap()
	if !km.Quit.Enabled() {
		t.Error("expected quit binding to be enabled")
	}
	if len(km.Quit.Keys()) == 0 {
		t.Error("expected quit binding to have keys")
	}
}

func TestKeymapConfigToKeymapOverride(t *testing.T) {
	cfg := DefaultKeymapConfig()
	cfg.Global["quit"] = []string{"ctrl+q"}
	km := cfg.ToKeymap()
	keys := km.Quit.Keys()
	if len(keys) != 1 || keys[0] != "ctrl+q" {
		t.Errorf("expected quit=[ctrl+q], got %v", keys)
	}
}

func TestKeymapConfigSaveLoad(t *testing.T) {
	cfg := DefaultKeymapConfig()
	cfg.Global["quit"] = []string{"ctrl+q"}

	dir := t.TempDir()
	path := filepath.Join(dir, "keymap.json")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}

	loaded, err := LoadKeymapConfig(path)
	if err != nil {
		t.Fatalf("LoadKeymapConfig: %v", err)
	}

	if loaded.Global["quit"] == nil || loaded.Global["quit"][0] != "ctrl+q" {
		t.Errorf("expected quit=[ctrl+q] after load, got %v", loaded.Global["quit"])
	}
}

func TestLoadKeymapConfigMissingFile(t *testing.T) {
	_, err := LoadKeymapConfig("/nonexistent/keymap.json")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadKeymapConfigInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadKeymapConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadKeymapConfigNilContexts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "partial.json")
	content := `{"global": {"quit": ["ctrl+q"]}}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadKeymapConfig(path)
	if err != nil {
		t.Fatalf("LoadKeymapConfig: %v", err)
	}
	if cfg.Global["quit"][0] != "ctrl+q" {
		t.Errorf("expected ctrl+q, got %v", cfg.Global["quit"])
	}
	if cfg.Tools == nil {
		t.Error("expected Tools to be initialized to empty map")
	}
	if cfg.Chat == nil {
		t.Error("expected Chat to be initialized to empty map")
	}
}

func TestKeymapConfigPath(t *testing.T) {
	path, err := KeymapConfigPath()
	if err != nil {
		t.Fatalf("KeymapConfigPath: %v", err)
	}
	if !strings.HasSuffix(path, "keymap.json") {
		t.Errorf("expected path ending in keymap.json, got %s", path)
	}
}

func TestDetectConflictsNone(t *testing.T) {
	cfg := DefaultKeymapConfig()
	conflicts := cfg.DetectConflicts()
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts in default config, got %d", len(conflicts))
	}
}

func TestDetectConflictsFound(t *testing.T) {
	cfg := DefaultKeymapConfig()
	cfg.Global["quit"] = []string{"ctrl+b"}
	cfg.Global["toggle_sidebar"] = []string{"ctrl+b"}
	conflicts := cfg.DetectConflicts()
	if len(conflicts) == 0 {
		t.Fatal("expected conflicts when same key bound to multiple actions")
	}
	found := false
	for _, c := range conflicts {
		if c.Key == "ctrl+b" && len(c.Actions) >= 2 {
			found = true
		}
	}
	if !found {
		t.Error("expected ctrl+b conflict to be detected")
	}
}

func TestKeymapConfigMerge(t *testing.T) {
	base := DefaultKeymapConfig()
	overlay := KeymapConfig{
		Global: ContextBindings{
			"quit": []string{"ctrl+q"},
		},
		Chat: ContextBindings{
			"submit": []string{"ctrl+enter"},
		},
	}
	merged := base.Merge(overlay)
	if merged.Global["quit"][0] != "ctrl+q" {
		t.Errorf("expected merged quit=ctrl+q, got %v", merged.Global["quit"])
	}
	if merged.Global["help"] == nil {
		t.Error("expected base 'help' binding to be preserved after merge")
	}
	if merged.Chat["submit"][0] != "ctrl+enter" {
		t.Errorf("expected merged chat submit=ctrl+enter, got %v", merged.Chat["submit"])
	}
}

func TestKeymapConfigSummary(t *testing.T) {
	cfg := DefaultKeymapConfig()
	summary := cfg.Summary()
	if !strings.Contains(summary, "global") {
		t.Errorf("expected 'global' in summary, got %s", summary)
	}
	for _, ctx := range allContexts {
		if !strings.Contains(summary, string(ctx)) {
			t.Errorf("expected '%s' in summary", ctx)
		}
	}
}

func TestVimPreset(t *testing.T) {
	cfg := DefaultKeymapConfig().Merge(VimPreset)
	km := cfg.ToKeymap()
	upKeys := km.ToolUp.Keys()
	downKeys := km.ToolDown.Keys()
	hasK := false
	hasJ := false
	for _, k := range upKeys {
		if k == "k" {
			hasK = true
		}
	}
	for _, k := range downKeys {
		if k == "j" {
			hasJ = true
		}
	}
	if !hasK {
		t.Error("expected 'k' in tool_up keys after vim preset")
	}
	if !hasJ {
		t.Error("expected 'j' in tool_down keys after vim preset")
	}
}

func TestKeymapConfigRoundTrip(t *testing.T) {
	cfg := DefaultKeymapConfig()
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.json")
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save: %v", err)
	}
	loaded, err := LoadKeymapConfig(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	km1 := cfg.ToKeymap()
	km2 := loaded.ToKeymap()
	if !keyBindsEqual(km1.Quit, km2.Quit) {
		t.Error("quit binding changed after round-trip")
	}
	if !keyBindsEqual(km1.Submit, km2.Submit) {
		t.Error("submit binding changed after round-trip")
	}
}

func keyBindsEqual(a, b key.Binding) bool {
	ak := a.Keys()
	bk := b.Keys()
	if len(ak) != len(bk) {
		return false
	}
	for i := range ak {
		if ak[i] != bk[i] {
			return false
		}
	}
	return true
}

func TestKeymapConfigAllContextsList(t *testing.T) {
	cfg := DefaultKeymapConfig()
	contexts := cfg.AllContexts()
	if len(contexts) != 7 {
		t.Errorf("expected 7 contexts, got %d", len(contexts))
	}
}
