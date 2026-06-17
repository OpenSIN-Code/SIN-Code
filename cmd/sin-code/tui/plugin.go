// SPDX-License-Identifier: MIT
package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	tea "charm.land/bubbletea/v2"
)

type TUIPlugin interface {
	Name() string
	Render(styles Styles, width, height int) string
	Update(msg tea.Msg) (handled bool)
	Keybindings() []HintPair
	SidebarItem() SidebarItem
}

type PluginManager struct {
	mu      sync.RWMutex
	plugins map[string]TUIPlugin
	order   []string
}

func NewPluginManager() *PluginManager {
	return &PluginManager{plugins: make(map[string]TUIPlugin)}
}

func (pm *PluginManager) Register(plugin TUIPlugin) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	name := plugin.Name()
	if _, exists := pm.plugins[name]; !exists {
		pm.order = append(pm.order, name)
	}
	pm.plugins[name] = plugin
}

func (pm *PluginManager) Unregister(name string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.plugins, name)
	for i, n := range pm.order {
		if n == name {
			pm.order = append(pm.order[:i], pm.order[i+1:]...)
			break
		}
	}
}

func (pm *PluginManager) Get(name string) (TUIPlugin, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	p, ok := pm.plugins[name]
	return p, ok
}

func (pm *PluginManager) All() []TUIPlugin {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	result := make([]TUIPlugin, 0, len(pm.order))
	for _, name := range pm.order {
		if p, ok := pm.plugins[name]; ok {
			result = append(result, p)
		}
	}
	return result
}

func (pm *PluginManager) Count() int {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	return len(pm.plugins)
}

type PluginManifest struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Builtin     string            `json:"builtin"`
	Config      map[string]string `json:"config,omitempty"`
}

func (pm *PluginManager) LoadFromDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read plugin dir %q: %w", dir, err)
	}
	var manifests []PluginManifest
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(dir, entry.Name(), "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var m PluginManifest
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		if m.Name == "" {
			m.Name = entry.Name()
		}
		manifests = append(manifests, m)
	}
	sort.Slice(manifests, func(i, j int) bool { return manifests[i].Name < manifests[j].Name })
	for _, m := range manifests {
		if m.Builtin == "" {
			continue
		}
		p := pm.createBuiltin(m)
		if p != nil {
			if m.Name != p.Name() {
				p = &namedPlugin{inner: p, name: m.Name}
			}
			pm.Register(p)
		}
	}
	return nil
}

type namedPlugin struct {
	inner TUIPlugin
	name  string
}

func (n *namedPlugin) Name() string                                   { return n.name }
func (n *namedPlugin) Render(s Styles, w, h int) string               { return n.inner.Render(s, w, h) }
func (n *namedPlugin) Update(msg tea.Msg) (handled bool)              { return n.inner.Update(msg) }
func (n *namedPlugin) Keybindings() []HintPair                        { return n.inner.Keybindings() }
func (n *namedPlugin) SidebarItem() SidebarItem                       { return n.inner.SidebarItem() }

var (
	builtinFactories = make(map[string]func(map[string]string) TUIPlugin)
	builtinMu        sync.RWMutex
)

func RegisterBuiltin(name string, factory func(map[string]string) TUIPlugin) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	builtinFactories[name] = factory
}

func getBuiltinFactory(name string) func(map[string]string) TUIPlugin {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	return builtinFactories[name]
}

func (pm *PluginManager) createBuiltin(m PluginManifest) TUIPlugin {
	factory := getBuiltinFactory(m.Builtin)
	if factory == nil {
		return nil
	}
	return factory(m.Config)
}

func (pm *PluginManager) RenderAll(styles Styles, width, height int) string {
	plugins := pm.All()
	if len(plugins) == 0 {
		return ""
	}
	perHeight := height / len(plugins)
	if perHeight < 3 {
		perHeight = 3
	}
	var b []byte
	for _, p := range plugins {
		rendered := p.Render(styles, width, perHeight)
		b = append(b, rendered...)
		b = append(b, '\n')
	}
	return string(b)
}

func (pm *PluginManager) UpdateAll(msg tea.Msg) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, p := range pm.plugins {
		p.Update(msg)
	}
}

func (pm *PluginManager) AllSidebarItems() []SidebarItem {
	plugins := pm.All()
	items := make([]SidebarItem, 0, len(plugins))
	for _, p := range plugins {
		items = append(items, p.SidebarItem())
	}
	return items
}

func (pm *PluginManager) AllKeybindings() []HintPair {
	plugins := pm.All()
	var hints []HintPair
	for _, p := range plugins {
		hints = append(hints, p.Keybindings()...)
	}
	return hints
}
