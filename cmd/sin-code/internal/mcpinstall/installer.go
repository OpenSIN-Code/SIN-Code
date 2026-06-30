// SPDX-License-Identifier: MIT
package mcpinstall

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// cmdRunner is the interface used to execute package-install commands.
// Tests swap in a fake to avoid real npm/pip invocations.
type cmdRunner interface {
	Run(name string, args ...string) error
}

// realRunner delegates to os/exec.
type realRunner struct {
	stdout io.Writer
	stderr io.Writer
}

func (r *realRunner) Run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr
	return cmd.Run()
}

// Installer handles MCP server package installation and config management.
type Installer struct {
	HomeDir string
	runner  cmdRunner
}

// NewInstaller creates an Installer using the real home directory and real
// command runner.
func NewInstaller() (*Installer, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Installer{
		HomeDir: home,
		runner:  &realRunner{stdout: os.Stdout, stderr: os.Stderr},
	}, nil
}

// newInstallerWith is a test constructor that injects a custom runner and
// overrides the home directory.
func newInstallerWith(homeDir string, runner cmdRunner) *Installer {
	return &Installer{HomeDir: homeDir, runner: runner}
}

// MCPConfig is the JSON structure of ~/.config/sin-code/mcp.json.
type MCPConfig struct {
	Servers map[string]MCPServerConfig `json:"mcpServers"`
}

// MCPServerConfig is the per-server entry inside mcp.json.
type MCPServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env,omitempty"`
}

// ConfigPath returns the full path to the mcp.json config file.
func (i *Installer) ConfigPath() string {
	return filepath.Join(i.HomeDir, ".config", "sin-code", "mcp.json")
}

// Install installs the MCP server package and adds it to the config.
func (i *Installer) Install(info MCPServerInfo) error {
	if err := i.installPackage(info.Package); err != nil {
		return err
	}
	return i.addToConfig(info)
}

func (i *Installer) installPackage(pkg MCPPackage) error {
	switch pkg.Type {
	case "npm":
		if err := i.runner.Run("npm", "install", "-g", pkg.Name); err != nil {
			return fmt.Errorf("npm install %s: %w", pkg.Name, err)
		}
	case "pip":
		if err := i.runner.Run("pip", "install", pkg.Name); err != nil {
			return fmt.Errorf("pip install %s: %w", pkg.Name, err)
		}
	case "go":
		if err := i.runner.Run("go", "install", pkg.Name+"@latest"); err != nil {
			return fmt.Errorf("go install %s: %w", pkg.Name, err)
		}
	case "binary":
		// Pre-built binary — no package manager step needed.
	}
	return nil
}

func (i *Installer) addToConfig(info MCPServerInfo) error {
	configPath := i.ConfigPath()
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	var config MCPConfig
	if data, err := os.ReadFile(configPath); err == nil {
		_ = json.Unmarshal(data, &config)
	}
	if config.Servers == nil {
		config.Servers = make(map[string]MCPServerConfig)
	}

	config.Servers[info.Name] = MCPServerConfig{
		Command: info.Package.Command,
		Args:    info.Package.Args,
		Env:     info.Package.Env,
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, filemode.Default())
}

// Uninstall removes the MCP server from the config file.
func (i *Installer) Uninstall(name string) error {
	configPath := i.ConfigPath()
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("read config: %w", err)
	}
	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return fmt.Errorf("parse config: %w", err)
	}
	if _, exists := config.Servers[name]; !exists {
		return fmt.Errorf("server %q not found in config", name)
	}
	delete(config.Servers, name)

	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath, data, filemode.Default())
}

// IsInstalled checks whether the server is already in the config.
func (i *Installer) IsInstalled(name string) bool {
	data, err := os.ReadFile(i.ConfigPath())
	if err != nil {
		return false
	}
	var config MCPConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return false
	}
	_, exists := config.Servers[name]
	return exists
}
