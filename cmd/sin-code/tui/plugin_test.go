// SPDX-License-Identifier: MIT
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

type mockPlugin struct {
	name      string
	renderOut string
	handled   bool
	hints     []HintPair
	sidebar   SidebarItem
}

func (m *mockPlugin) Name() string                                   { return m.name }
func (m *mockPlugin) Render(styles Styles, width, height int) string { return m.renderOut }
func (m *mockPlugin) Update(msg tea.Msg) (handled bool)              { return m.handled }
func (m *mockPlugin) Keybindings() []HintPair                        { return m.hints }
func (m *mockPlugin) SidebarItem() SidebarItem                       { return m.sidebar }

func TestPluginRegisterUnregister(t *testing.T) {
	pm := NewPluginManager()
	p1 := &mockPlugin{name: "alpha", renderOut: "alpha-panel"}
	p2 := &mockPlugin{name: "beta", renderOut: "beta-panel"}
	pm.Register(p1)
	if pm.Count() != 1 {
		t.Fatalf("expected 1 plugin, got %d", pm.Count())
	}
	pm.Register(p2)
	if pm.Count() != 2 {
		t.Fatalf("expected 2 plugins, got %d", pm.Count())
	}
	pm.Unregister("alpha")
	if pm.Count() != 1 {
		t.Fatalf("expected 1 after unregister, got %d", pm.Count())
	}
	if _, ok := pm.Get("alpha"); ok {
		t.Error("alpha should be gone")
	}
	pm.Unregister("nonexistent")
	pm.Unregister("beta")
	if pm.Count() != 0 {
		t.Fatalf("expected 0, got %d", pm.Count())
	}
}

func TestPluginGet(t *testing.T) {
	pm := NewPluginManager()
	p := &mockPlugin{name: "gamma", renderOut: "gamma"}
	pm.Register(p)
	got, ok := pm.Get("gamma")
	if !ok {
		t.Fatal("expected to find gamma")
	}
	if got.Name() != "gamma" {
		t.Errorf("expected gamma, got %s", got.Name())
	}
	if _, ok := pm.Get("nonexistent"); ok {
		t.Error("should not find nonexistent")
	}
}

func TestPluginAll(t *testing.T) {
	pm := NewPluginManager()
	pm.Register(&mockPlugin{name: "delta"})
	pm.Register(&mockPlugin{name: "epsilon"})
	all := pm.All()
	if len(all) != 2 {
		t.Fatalf("expected 2, got %d", len(all))
	}
	if all[0].Name() != "delta" || all[1].Name() != "epsilon" {
		t.Errorf("order mismatch")
	}
	pm.Register(&mockPlugin{name: "delta"})
	all = pm.All()
	if len(all) != 2 {
		t.Errorf("re-register should not duplicate, got %d", len(all))
	}
}

func TestPluginRender(t *testing.T) {
	pm := NewPluginManager()
	styles := NewStyles(Themes[0])
	pm.Register(&mockPlugin{name: "zeta", renderOut: "zeta-content"})
	rendered := pm.RenderAll(styles, 80, 24)
	if rendered == "" {
		t.Fatal("expected non-empty render")
	}
	if !strings.Contains(rendered, "zeta-content") {
		t.Errorf("expected zeta-content, got %q", rendered)
	}
	if NewPluginManager().RenderAll(styles, 80, 24) != "" {
		t.Error("empty manager should render empty")
	}
}

func TestPluginUpdate(t *testing.T) {
	pm := NewPluginManager()
	pm.Register(&mockPlugin{name: "eta", handled: true})
	pm.UpdateAll(nil)
}

func TestPluginKeybindings(t *testing.T) {
	pm := NewPluginManager()
	hints := []HintPair{{Key: "ctrl+r", Label: "refresh"}, {Key: "ctrl+w", Label: "close"}}
	pm.Register(&mockPlugin{name: "theta", hints: hints})
	got := pm.AllKeybindings()
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Key != "ctrl+r" {
		t.Errorf("expected ctrl+r, got %s", got[0].Key)
	}
}

func TestPluginSidebar(t *testing.T) {
	pm := NewPluginManager()
	item := SidebarItem{Icon: "W", Label: "Word Count", Shortcut: "w"}
	pm.Register(&mockPlugin{name: "iota", sidebar: item})
	items := pm.AllSidebarItems()
	if len(items) != 1 {
		t.Fatalf("expected 1, got %d", len(items))
	}
	if items[0].Label != "Word Count" {
		t.Errorf("expected Word Count, got %q", items[0].Label)
	}
}

func TestPluginManifestLoad(t *testing.T) {
	RegisterBuiltin("wordcount", func(config map[string]string) TUIPlugin {
		return &mockPlugin{
			name:      "wordcount",
			renderOut: "wc-panel",
			hints:     []HintPair{{Key: "ctrl+r", Label: "refresh"}},
			sidebar:   SidebarItem{Icon: "W", Label: "Word Count", Shortcut: "w"},
		}
	})
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "mywordcount")
	os.MkdirAll(pluginDir, 0o755)
	testFile := filepath.Join(dir, "test.txt")
	os.WriteFile(testFile, []byte("hello world\ntest\n"), 0o644)
	manifest := `{"name":"mywordcount","description":"wc","builtin":"wordcount","config":{"file":"` + testFile + `"}}`
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o644)
	pm := NewPluginManager()
	if err := pm.LoadFromDir(dir); err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if pm.Count() != 1 {
		t.Fatalf("expected 1, got %d", pm.Count())
	}
	p, ok := pm.Get("mywordcount")
	if !ok {
		t.Fatal("expected mywordcount")
	}
	if p.Name() != "mywordcount" {
		t.Errorf("expected mywordcount, got %s", p.Name())
	}
}

func TestPluginManifestLoadEmptyDir(t *testing.T) {
	pm := NewPluginManager()
	if err := pm.LoadFromDir(t.TempDir()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pm.Count() != 0 {
		t.Errorf("expected 0, got %d", pm.Count())
	}
}

func TestPluginManifestLoadNonexistentDir(t *testing.T) {
	pm := NewPluginManager()
	if err := pm.LoadFromDir("/nonexistent/path"); err == nil {
		t.Error("expected error")
	}
}

func TestPluginExampleWordCount(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "sample.go")
	content := "package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n"
	os.WriteFile(testFile, []byte(content), 0o644)

	RegisterBuiltin("wordcount", func(config map[string]string) TUIPlugin {
		return &mockPlugin{
			name:      "wordcount",
			renderOut: "Words: 5 · Lines: 4 · Chars: 50",
			hints:     []HintPair{{Key: "ctrl+r", Label: "refresh"}},
			sidebar:   SidebarItem{Icon: "W", Label: "Word Count", Shortcut: "w"},
		}
	})
	factory := getBuiltinFactory("wordcount")
	if factory == nil {
		t.Fatal("wordcount not registered")
	}
	plugin := factory(map[string]string{"file": testFile})
	if plugin == nil {
		t.Fatal("expected non-nil plugin")
	}
	if plugin.Name() != "wordcount" {
		t.Errorf("expected wordcount, got %s", plugin.Name())
	}
	styles := NewStyles(Themes[0])
	rendered := plugin.Render(styles, 80, 10)
	if !strings.Contains(rendered, "Words:") {
		t.Errorf("expected Words:, got %q", rendered)
	}
	hints := plugin.Keybindings()
	if len(hints) == 0 {
		t.Error("expected keybindings")
	}
	item := plugin.SidebarItem()
	if item.Label != "Word Count" {
		t.Errorf("expected Word Count, got %q", item.Label)
	}
}

func TestPluginConcurrent(t *testing.T) {
	pm := NewPluginManager()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pm.Register(&mockPlugin{name: "c-" + itoa(n)})
			_ = pm.All()
			_, _ = pm.Get("c-" + itoa(n))
		}(i)
	}
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pm.Unregister("c-" + itoa(n))
			_ = pm.All()
		}(i)
	}
	wg.Wait()
}

func TestAccessibilityDefaults(t *testing.T) {
	a := NewAccessibilityMode()
	if a.HighContrast() {
		t.Error("default HC should be false")
	}
	if a.ScreenReader() {
		t.Error("default SR should be false")
	}
	if a.KeyboardOnlyMode() {
		t.Error("default KO should be false")
	}
	if a.LargeText() {
		t.Error("default LT should be false")
	}
	if a.ReducedMotion() {
		t.Error("default RM should be false")
	}
}

func TestAccessibilityHighContrast(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetHighContrast(true)
	if !a.HighContrast() {
		t.Error("HC should be true")
	}
	a.SetHighContrast(false)
	if a.HighContrast() {
		t.Error("HC should be false")
	}
	theme := HighContrastTheme()
	if theme.Name != "HighContrast" {
		t.Errorf("expected HighContrast, got %q", theme.Name)
	}
	if theme.Background != "#000000" {
		t.Errorf("expected black, got %q", theme.Background)
	}
	if theme.Text != "#FFFFFF" {
		t.Errorf("expected white, got %q", theme.Text)
	}
	if theme.Accent != "#FFFF00" {
		t.Errorf("expected yellow, got %q", theme.Accent)
	}
	if theme.Error != "#FF0000" {
		t.Errorf("expected red, got %q", theme.Error)
	}
	if theme.Success != "#00FF00" {
		t.Errorf("expected green, got %q", theme.Success)
	}
}

func TestAccessibilityScreenReader(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetScreenReader(true)
	if !a.ScreenReader() {
		t.Error("SR should be true")
	}
	desc := a.Describe(ViewChat)
	if !strings.Contains(desc, "Chat view") {
		t.Errorf("expected Chat view, got %q", desc)
	}
	if !strings.Contains(desc, "Ctrl+S") {
		t.Errorf("expected Ctrl+S, got %q", desc)
	}
	descTools := a.Describe(ViewTools)
	if !strings.Contains(descTools, "Tools view") {
		t.Errorf("expected Tools view, got %q", descTools)
	}
}

func TestAccessibilityDescribeMessage(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetScreenReader(true)
	msg := ChatMessage{Kind: chatUser, Text: "Fix the bug in main.go"}
	desc := a.DescribeMessage(msg)
	if !strings.Contains(desc, "User message") {
		t.Errorf("expected User message, got %q", desc)
	}
	if !strings.Contains(desc, "Fix the bug") {
		t.Errorf("expected text, got %q", desc)
	}
	toolMsg := ChatMessage{Kind: chatTool, Tool: "sin_edit", Result: true}
	td := a.DescribeMessage(toolMsg)
	if !strings.Contains(td, "Tool call") {
		t.Errorf("expected Tool call, got %q", td)
	}
	if !strings.Contains(td, "sin_edit") {
		t.Errorf("expected sin_edit, got %q", td)
	}
	if !strings.Contains(td, "success") {
		t.Errorf("expected success, got %q", td)
	}
}

func TestAccessibilityKeyboardOnly(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetKeyboardOnlyMode(true)
	if !a.KeyboardOnlyMode() {
		t.Error("KO should be true")
	}
	if a.ShouldShowMouseCursor() {
		t.Error("should not show mouse cursor")
	}
	a.SetKeyboardOnlyMode(false)
	if !a.ShouldShowMouseCursor() {
		t.Error("should show mouse cursor")
	}
	shortcuts := a.AllKeyboardShortcuts()
	if len(shortcuts) == 0 {
		t.Error("expected shortcuts")
	}
	sv := a.AllKeyboardShortcutsForView(ViewChat)
	if len(sv) <= len(shortcuts) {
		t.Error("expected more shortcuts for view")
	}
}

func TestAccessibilityLargeText(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetLargeText(true)
	if !a.LargeText() {
		t.Error("LT should be true")
	}
	if !a.BoldAll() {
		t.Error("BoldAll should be true")
	}
	if a.ExtraPadding() != 2 {
		t.Errorf("expected 2, got %d", a.ExtraPadding())
	}
	a.SetLargeText(false)
	if a.BoldAll() {
		t.Error("BoldAll should be false")
	}
	if a.ExtraPadding() != 0 {
		t.Errorf("expected 0, got %d", a.ExtraPadding())
	}
}

func TestAccessibilityReducedMotion(t *testing.T) {
	a := NewAccessibilityMode()
	a.SetReducedMotion(true)
	if !a.ReducedMotion() {
		t.Error("RM should be true")
	}
	if a.ShouldAnimate() {
		t.Error("should not animate")
	}
	text := a.SpinnerText(5 * time.Second)
	if !strings.Contains(text, "thinking") {
		t.Errorf("expected thinking, got %q", text)
	}
	if !strings.Contains(text, "5s") {
		t.Errorf("expected 5s, got %q", text)
	}
	a.SetReducedMotion(false)
	if !a.ShouldAnimate() {
		t.Error("should animate")
	}
	if a.SpinnerText(time.Second) != "" {
		t.Error("expected empty spinner text")
	}
}

func TestAccessibilityStatusText(t *testing.T) {
	a := NewAccessibilityMode()
	cases := []struct{ s, w string }{{"running", "[RUNNING] "}, {"passed", "[PASSED] "}, {"failed", "[FAILED] "}, {"idle", "[IDLE] "}, {"pending", "[PENDING] "}, {"custom", "[CUSTOM] "}}
	for _, c := range cases {
		if got := a.StatusText(c.s); got != c.w {
			t.Errorf("StatusText(%q)=%q, want %q", c.s, got, c.w)
		}
	}
}

func TestAccessibilityApplyToConfig(t *testing.T) {
	a := NewAccessibilityMode()
	a.ApplyToConfig(map[string]bool{"high_contrast": true, "screen_reader": true, "reduced_motion": true, "large_text": true})
	if !a.HighContrast() {
		t.Error("HC should be true")
	}
	if !a.ScreenReader() {
		t.Error("SR should be true")
	}
	if !a.ReducedMotion() {
		t.Error("RM should be true")
	}
	if !a.LargeText() {
		t.Error("LT should be true")
	}
}

func TestAccessibilityApplyTheme(t *testing.T) {
	a := NewAccessibilityMode()
	base := Themes[0]
	if a.ApplyTheme(base).Name != base.Name {
		t.Errorf("expected base theme")
	}
	a.SetHighContrast(true)
	if a.ApplyTheme(base).Name != "HighContrast" {
		t.Errorf("expected HighContrast")
	}
}

func TestAccessibilityApplyToStyles(t *testing.T) {
	a := NewAccessibilityMode()
	base := NewStyles(Themes[0])
	if a.ApplyToStyles(base).Theme.Name != base.Theme.Name {
		t.Errorf("expected base theme")
	}
	a.SetHighContrast(true)
	if a.ApplyToStyles(base).Theme.Name != "HighContrast" {
		t.Errorf("expected HighContrast")
	}
}

func TestAccessibilityConcurrent(t *testing.T) {
	a := NewAccessibilityMode()
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			a.SetHighContrast(i%2 == 0)
			_ = a.HighContrast()
			_ = a.ShouldAnimate()
			_ = a.Describe(ViewChat)
		}
	}()
	for i := 0; i < 100; i++ {
		a.SetScreenReader(i%2 == 0)
		_ = a.ScreenReader()
		_ = a.BoldAll()
		_ = a.DescribeMessage(ChatMessage{Kind: chatUser, Text: "test"})
	}
	<-done
}
