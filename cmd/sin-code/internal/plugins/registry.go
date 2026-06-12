// SPDX-License-Identifier: MIT
// Purpose: plugin registry — discovers plugins on disk, exposes their
// subcommands via cobra, their agents via the orchestrator registry, and
// their tools via the MCP server. SOTA pattern: gh-extensions, oh-my-zsh.
package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/spf13/cobra"
)

type Registry struct {
	mu      sync.RWMutex
	plugins map[string]*Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: map[string]*Plugin{}}
}

func (r *Registry) LoadFromDir(dir string) error {
	if dir == "" {
		dir = ResolvePluginDir("")
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	plugins, err := LoadDir(dir)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, p := range plugins {
		if p.Enabled {
			r.plugins[p.Name] = p
		}
	}
	return nil
}

// ResolvePluginDir returns the plugin dir to use. Empty arg → env override
// (SIN_CODE_CONFIG_DIR) → default user config dir. Lets testscripts redirect
// discovery to a tmp dir without touching HOME.
func ResolvePluginDir(override string) string {
	if override != "" {
		return override
	}
	if cfg := os.Getenv("SIN_CODE_CONFIG_DIR"); cfg != "" {
		return filepath.Join(cfg, "sin-code", "plugins")
	}
	return DefaultPluginDir()
}

func (r *Registry) List() []*Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	return out
}

func (r *Registry) Get(name string) (*Plugin, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[name]
	return p, ok
}

func (r *Registry) Register(p *Plugin) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.plugins[p.Name] = p
}

// AddSubcommandsTo attaches all enabled plugin subcommands to the given
// root cobra command. Each subcommand becomes a real cobra.Command that
// exec's the plugin binary.
func (r *Registry) AddSubcommandsTo(root *cobra.Command) {
	for _, p := range r.List() {
		for _, sc := range p.Subcommands {
			pName, scName, scBin, scArgs, scEnv := p.Name, sc.Name, sc.Binary, sc.Args, sc.Env
			pluginPath := p.Path
			cmd := &cobra.Command{
				Use:                scName,
				Short:              fmt.Sprintf("[plugin %s] %s", pName, sc.Description),
				Long:               sc.Description,
				DisableFlagParsing: true,
				RunE: func(cmd *cobra.Command, args []string) error {
					fullArgs := append([]string{}, scArgs...)
					fullArgs = append(fullArgs, args...)
					return runPlugin(pluginPath, scBin, fullArgs, scEnv)
				},
			}
			root.AddCommand(cmd)
		}
	}
}

// AgentConfigs returns the agents contributed by all enabled plugins,
// converted to orchestrator.AgentConfig.
func (r *Registry) AgentConfigs() []orchestrator.AgentConfig {
	var out []orchestrator.AgentConfig
	for _, p := range r.List() {
		for _, a := range p.Agents {
			out = append(out, orchestrator.AgentConfig{
				Name:        "plugin-" + p.Name + "-" + a.Name,
				Description: fmt.Sprintf("[plugin %s] type=%s model=%s", p.Name, a.Type, a.Model),
				Type:        orchestrator.TaskType(a.Type),
				Model:       a.Model,
				Provider:    a.Provider,
				SystemFile:  filepath.Join(p.Path, a.System),
				MemoryNS:    "plugin-" + p.Name,
			})
		}
	}
	return out
}

// MCPToolDef describes one plugin tool to be exposed via the MCP server.
// The Name uses the canonical "sin_plugin_<plugin>_<tool>" form, the
// Description comes from the manifest, and Schema is built from the
// manifest's Args slice (each arg → JSON Schema string property).
type MCPToolDef struct {
	Name        string
	Description string
	Plugin      string
	PluginPath  string
	Tool        string
	Binary      string
	Args        []string
	Timeout     int
	Schema      map[string]any
}

// MCPTools returns one MCPToolDef per enabled plugin tool, named
// "sin_plugin_<plugin>_<tool>" so they are easily greppable and don't
// collide with the built-in sin_* MCP tools.
func (r *Registry) MCPTools() []MCPToolDef {
	var out []MCPToolDef
	for _, p := range r.List() {
		for _, t := range p.Tools {
			props := map[string]any{}
			for _, a := range t.Args {
				props[a] = map[string]any{
					"type":        "string",
					"description": "Argument passed as --" + a + " to the plugin binary",
				}
			}
			schema := map[string]any{
				"type":       "object",
				"properties": props,
			}
			if len(t.Args) > 0 {
				schema["required"] = t.Args
			}
			desc := t.Description
			if desc == "" {
				desc = fmt.Sprintf("[plugin %s] tool %s", p.Name, t.Name)
			} else {
				desc = fmt.Sprintf("[plugin %s] %s", p.Name, desc)
			}
			out = append(out, MCPToolDef{
				Name:        "sin_plugin_" + p.Name + "_" + t.Name,
				Description: desc,
				Plugin:      p.Name,
				PluginPath:  p.Path,
				Tool:        t.Name,
				Binary:      t.Binary,
				Args:        t.Args,
				Timeout:     t.Timeout,
				Schema:      schema,
			})
		}
	}
	return out
}

// HookDef is a plugin-registered todo event hook. Plugin hooks are run
// as subprocesses with the same SIN_TODO_* env vars the built-in todo
// hooks receive.
type HookDef struct {
	Plugin  string
	Event   string
	Command string
	Timeout int
}

// HooksFor returns all enabled plugin hooks matching the given event name
// (e.g. "post_complete"). Unknown events yield no hooks.
func (r *Registry) HooksFor(event string) []HookDef {
	var out []HookDef
	for _, p := range r.List() {
		for _, h := range p.Hooks {
			if h.Event == event {
				out = append(out, HookDef{
					Plugin:  p.Name,
					Event:   h.Event,
					Command: h.Command,
					Timeout: h.Timeout,
				})
			}
		}
	}
	return out
}

func runPlugin(pluginDir, binary string, args, env []string) error {
	fullPath := binary
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(pluginDir, binary)
	}
	cmd := exec.Command(fullPath, args...)
	cmd.Dir = pluginDir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
