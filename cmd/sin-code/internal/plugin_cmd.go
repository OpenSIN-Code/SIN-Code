// SPDX-License-Identifier: MIT
// Purpose: sin-code plugin CLI — install/list/info/enable/disable.
package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
)

var (
	pluginPath    string
	pluginNameArg string
)

var PluginCmd = &cobra.Command{
	Use:   "plugin",
	Short: "Manage user-installed plugins (subcommands, agents, tools, hooks)",
	Long: `Plugins extend sin-code without forking. Install from a local path or
git URL, and sin-code will:
  - Register their subcommands under sin-code (e.g. sin-code my-plugin-cmd)
  - Register their agents with the orchestrator (prefixed plugin-<name>-<agent>)
  - Register their tools with the MCP server (prefixed sin_plugin_<name>_<tool>)
  - Wire their hooks into todo events

Discovery: ~/.local/share/sin-code/plugins/<name>/ with plugin.toml manifest.`,
	SilenceUsage: true,
}

func init() {
	PluginCmd.PersistentFlags().StringVar(&pluginPath, "path", "", "Override plugin directory (default: ~/.local/share/sin-code/plugins)")

	PluginCmd.AddCommand(pluginListCmd)
	PluginCmd.AddCommand(pluginInfoCmd)
	PluginCmd.AddCommand(pluginInstallCmd)
	PluginCmd.AddCommand(pluginUninstallCmd)
	PluginCmd.AddCommand(pluginEnableCmd)
	PluginCmd.AddCommand(pluginDisableCmd)
}

func pluginDir() string {
	if pluginPath != "" {
		return pluginPath
	}
	return plugins.DefaultPluginDir()
}

var pluginListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed plugins",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := os.ReadDir(pluginDir())
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("(no plugins directory; install one with 'sin-code plugin install <path>')")
				return nil
			}
			return err
		}
		loaded, _ := plugins.LoadDir(pluginDir())
		byName := map[string]*plugins.Plugin{}
		for _, p := range loaded {
			byName[p.Name] = p
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "NAME\tVERSION\tSTATUS\tDESCRIPTION")
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p, ok := byName[e.Name()]
			status := "enabled"
			if _, err := os.Stat(filepath.Join(pluginDir(), e.Name(), ".disabled")); err == nil {
				status = "disabled"
			}
			if !ok {
				status = "broken"
			}
			if p == nil {
				fmt.Fprintf(tw, "%s\t-\t%s\t(invalid manifest)\n", e.Name(), status)
				continue
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", p.Name, p.Version, status, truncate(p.Description, 50))
		}
		return tw.Flush()
	},
}

var pluginInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show details for a plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginNameArg = args[0]
		p, err := loadPlugin(pluginNameArg)
		if err != nil {
			return err
		}
		fmt.Printf("Name:         %s\n", p.Name)
		fmt.Printf("Version:      %s\n", p.Version)
		if p.Author != "" {
			fmt.Printf("Author:       %s\n", p.Author)
		}
		if p.Homepage != "" {
			fmt.Printf("Homepage:     %s\n", p.Homepage)
		}
		if p.License != "" {
			fmt.Printf("License:      %s\n", p.License)
		}
		if p.MinSinCode != "" {
			fmt.Printf("Min sin-code: %s\n", p.MinSinCode)
		}
		if p.Description != "" {
			fmt.Printf("Description:  %s\n", p.Description)
		}
		if len(p.Capabilities) > 0 {
			fmt.Printf("Capabilities: %v\n", p.Capabilities)
		}
		fmt.Printf("Path:         %s\n", p.Path)
		fmt.Printf("Enabled:      %v\n", p.Enabled)
		if len(p.Subcommands) > 0 {
			fmt.Println("\nSubcommands:")
			for _, s := range p.Subcommands {
				fmt.Printf("  %-20s  binary=%s  desc=%s\n", s.Name, s.Binary, s.Description)
			}
		}
		if len(p.Agents) > 0 {
			fmt.Println("\nAgents:")
			for _, a := range p.Agents {
				fmt.Printf("  plugin-%s-%-12s  type=%s  model=%s\n", p.Name, a.Name, a.Type, a.Model)
			}
		}
		if len(p.Tools) > 0 {
			fmt.Println("\nTools:")
			for _, t := range p.Tools {
				fmt.Printf("  sin_plugin_%s_%s  binary=%s\n", p.Name, t.Name, t.Binary)
			}
		}
		if len(p.Hooks) > 0 {
			fmt.Println("\nHooks:")
			for _, h := range p.Hooks {
				fmt.Printf("  %s  command=%s\n", h.Event, h.Command)
			}
		}
		return nil
	},
}

var pluginInstallCmd = &cobra.Command{
	Use:   "install <path-or-name>",
	Short: "Install a plugin from a local path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		src := args[0]
		manifestPath := filepath.Join(src, plugins.ManifestFile)
		if _, err := os.Stat(manifestPath); err != nil {
			return fmt.Errorf("not a plugin: %s (no %s found)", src, plugins.ManifestFile)
		}
		p, err := plugins.Load(manifestPath)
		if err != nil {
			return err
		}
		dest := filepath.Join(pluginDir(), p.Name)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("plugin %s already installed at %s", p.Name, dest)
		}
		if err := copyDir(src, dest); err != nil {
			return err
		}
		fmt.Printf("Installed %s v%s to %s\n", p.Name, p.Version, dest)
		fmt.Println("Restart sin-code (or reload plugins) to activate.")
		return nil
	},
}

var pluginUninstallCmd = &cobra.Command{
	Use:   "uninstall <name>",
	Short: "Remove a plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginNameArg = args[0]
		p, err := loadPlugin(pluginNameArg)
		if err != nil {
			return err
		}
		if err := os.RemoveAll(p.Path); err != nil {
			return err
		}
		fmt.Printf("Uninstalled %s\n", p.Name)
		return nil
	},
}

var pluginEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable a previously-disabled plugin",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginNameArg = args[0]
		p, err := loadPlugin(pluginNameArg)
		if err != nil {
			return err
		}
		if err := p.Enable(); err != nil {
			return err
		}
		fmt.Printf("Enabled %s\n", p.Name)
		return nil
	},
}

var pluginDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a plugin (without uninstalling)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginNameArg = args[0]
		p, err := loadPlugin(pluginNameArg)
		if err != nil {
			return err
		}
		if err := p.Disable(); err != nil {
			return err
		}
		fmt.Printf("Disabled %s (reload sin-code to take effect)\n", p.Name)
		return nil
	},
}

func loadPlugin(name string) (*plugins.Plugin, error) {
	manifestPath := filepath.Join(pluginDir(), name, plugins.ManifestFile)
	p, err := plugins.Load(manifestPath)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(filepath.Join(p.Path, ".disabled")); err == nil {
		p.Enabled = false
	}
	return p, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}
