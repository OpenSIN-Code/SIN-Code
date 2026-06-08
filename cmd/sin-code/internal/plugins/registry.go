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

	"github.com/OpenSIN-Code/SIN-Code-Bundle/cmd/sin-code/internal/orchestrator"
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
		dir = DefaultPluginDir()
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
