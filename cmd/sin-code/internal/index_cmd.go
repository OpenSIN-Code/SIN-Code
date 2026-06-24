// SPDX-License-Identifier: MIT

// Purpose: index — persistent incremental code index management.
// Also contains: plugin CLI — install/list/info/enable/disable (merged from
// plugin_cmd.go).
// Also contains: `sin-code rules` CLI — path-scoped rule surface (merged from
// rules_cmd.go).
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/plugins"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/rules"
)

var indexAbsPath = filepath.Abs

var IndexCmd = &cobra.Command{
	Use:   "index",
	Short: "Manage persistent incremental code index",
	Long: `Builds, refreshes, and inspects a persistent gob-persisted
index at <root>/.sin-code/index.bin. Trigram + symbol table
for instant lookups.`,
}

func init() {
	IndexCmd.AddCommand(indexBuildCmd)
	IndexCmd.AddCommand(indexRefreshCmd)
	IndexCmd.AddCommand(indexStatusCmd)
	IndexCmd.AddCommand(indexWatchCmd)
	IndexCmd.AddCommand(indexClearCmd)
}

var indexBuildCmd = &cobra.Command{
	Use:   "build [root]",
	Short: "Build index from scratch",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := indexAbsPath(root)
		if err != nil {
			return err
		}
		idx, err := buildIndex(root)
		if err != nil {
			return err
		}
		if err := saveIndex(idx); err != nil {
			return err
		}
		setFileIndex(idx)
		fmt.Printf("Indexed %d files in %s\n", idx.len(), root)
		return nil
	},
}

var indexRefreshCmd = &cobra.Command{
	Use:   "refresh [root]",
	Short: "Incremental refresh (stat-based)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := indexAbsPath(root)
		if err != nil {
			return err
		}
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return err
		}
		if !existed {
			fmt.Println("No existing index. Run 'sin-code index build' first.")
			return nil
		}
		idx, added, removed, err := refreshIndex(idx)
		if err != nil {
			return err
		}
		if err := saveIndex(idx); err != nil {
			return err
		}
		setFileIndex(idx)
		fmt.Printf("Refreshed: +%d -%d files. Total %d\n", added, removed, idx.len())
		return nil
	},
}

var indexStatusCmd = &cobra.Command{
	Use:   "status [root]",
	Short: "Show index status",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := indexAbsPath(root)
		if err != nil {
			return err
		}
		idx, existed, err := getFileIndex(root)
		if err != nil {
			return err
		}
		if !existed || idx.len() == 0 {
			fmt.Println("No index found.")
			return nil
		}
		fmt.Printf("Index: %s\n", indexPath(root))
		fmt.Printf("Files: %d\n", idx.len())
		fmt.Printf("Created: %s\n", idx.createdAt.Format(time.RFC3339))
		return nil
	},
}

// indexWatchInterval and indexWatchMaxIterations are test hooks for the
// foreground daemon. indexWatchMaxIterations = -1 means loop forever.
var (
	indexWatchInterval      = 30 * time.Second
	indexWatchMaxIterations = -1
)

var indexWatchCmd = &cobra.Command{
	Use:   "watch [root]",
	Short: "Auto-refresh every 30s (foreground daemon)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := indexAbsPath(root)
		if err != nil {
			return err
		}
		for iter := 0; ; iter++ {
			if indexWatchMaxIterations >= 0 && iter >= indexWatchMaxIterations {
				return nil
			}
			idx, existed, err := getFileIndex(root)
			if err != nil {
				return err
			}
			if !existed {
				idx, err = buildIndex(root)
				if err != nil {
					return err
				}
			} else {
				idx, _, _, err = refreshIndex(idx)
				if err != nil {
					return err
				}
			}
			if err := saveIndex(idx); err != nil {
				return err
			}
			setFileIndex(idx)
			time.Sleep(indexWatchInterval)
		}
	},
}

var indexClearCmd = &cobra.Command{
	Use:   "clear [root]",
	Short: "Delete index file",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		root, err := indexAbsPath(root)
		if err != nil {
			return err
		}
		p := indexPath(root)
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		setFileIndex(nil)
		fmt.Println("Index cleared.")
		return nil
	},
}

// plugin CLI — install/list/info/enable/disable (merged from plugin_cmd.go).

var (
	pluginPath    string
	pluginNameArg string
	walkFn        = filepath.Walk
	relFn         = filepath.Rel

	loadPluginFn   = loadPlugin
	pluginReadDir  = os.ReadDir
	pluginLoad     = plugins.Load
	copyDirFn      = copyDir
	pluginMkdirAll = os.MkdirAll
	pluginRemove   = os.RemoveAll
	pluginEnable   = func(p *plugins.Plugin) error { return p.Enable() }
	pluginDisable  = func(p *plugins.Plugin) error { return p.Disable() }
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
		entries, err := pluginReadDir(pluginDir())
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
		p, err := loadPluginFn(pluginNameArg)
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
		p, err := pluginLoad(manifestPath)
		if err != nil {
			return err
		}
		dest := filepath.Join(pluginDir(), p.Name)
		if err := pluginMkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(dest); err == nil {
			return fmt.Errorf("plugin %s already installed at %s", p.Name, dest)
		}
		if err := copyDirFn(src, dest); err != nil {
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
		p, err := loadPluginFn(pluginNameArg)
		if err != nil {
			return err
		}
		if err := pluginRemove(p.Path); err != nil {
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
		p, err := loadPluginFn(pluginNameArg)
		if err != nil {
			return err
		}
		if err := pluginEnable(p); err != nil {
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
		p, err := loadPluginFn(pluginNameArg)
		if err != nil {
			return err
		}
		if err := pluginDisable(p); err != nil {
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
	return walkFn(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := relFn(src, path)
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
		return os.WriteFile(target, data, filemode.Default())
	})
}

// ============================================================================
// rules — path-scoped rule loader (.sin-code/rules/<topic>.md)
// ============================================================================

var (
	rulesWorkspace string
	rulesFormat    string

	rulesAbs   = filepath.Abs
	rulesGetwd = os.Getwd
)

var RulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "Path-scoped rule loader (.sin-code/rules/<topic>.md)",
	Long: `Loads every <name>.md file under <workspace>/.sin-code/rules/ with
a YAML frontmatter header. Each rule's 'paths:' glob list determines
which files the rule is lazy-injected for.

  list                    List every rule
  show <name>             Print a single rule (description, paths, body)
  path <abs-file-path>     Resolve which rules match a file path
  where                   Print the on-disk rules directory

Storage: ./.sin-code/rules/ (override with --workspace).`,
	SilenceUsage: true,
}

func init() {
	RulesCmd.PersistentFlags().StringVar(&rulesWorkspace, "workspace", ".", "workspace root")
	RulesCmd.PersistentFlags().StringVar(&rulesFormat, "format", "text", "text|json")
	RulesCmd.AddCommand(rulesListCmd, rulesShowCmd, rulesPathCmd, rulesWhereCmd)
}

var rulesListCmd = &cobra.Command{
	Use:   "list",
	Short: "List every rule",
	RunE: func(cmd *cobra.Command, args []string) error {
		abs, err := rulesAbs(rulesWorkspace)
		if err != nil {
			return err
		}
		s := rules.New(abs)
		if _, err := s.Load(); err != nil {
			return err
		}
		all := s.All()
		if rulesFormat == "json" {
			return encodeJSON(all)
		}
		if len(all) == 0 {
			fmt.Printf("no rules in %s/.sin-code/rules/\n", abs)
			return nil
		}
		fmt.Printf("%d rules in %s/.sin-code/rules/:\n", len(all), abs)
		for _, r := range all {
			marker := "         "
			if r.AlwaysOn {
				marker = "[always] "
			} else if len(r.Globs) == 0 {
				marker = "[unscpd] "
			} else {
				marker = fmt.Sprintf("[%d globs]", len(r.Globs))
			}
			desc := r.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Printf("  %s%-6s  %s — %s\n", marker, r.Name, filepath.Base(r.Source), truncate(desc, 60))
		}
		return nil
	},
}

var rulesShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print a single rule",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		abs, err := rulesAbs(rulesWorkspace)
		if err != nil {
			return err
		}
		s := rules.New(abs)
		if _, err := s.Load(); err != nil {
			return err
		}
		r, ok := s.Get(args[0])
		if !ok {
			return fmt.Errorf("rules: no such rule %q (try `sin-code rules list`)", args[0])
		}
		if rulesFormat == "json" {
			return encodeJSON(r)
		}
		fmt.Printf("# %s\n", r.Name)
		if r.AlwaysOn {
			fmt.Println("  (always-on)")
		}
		if r.Description != "" {
			fmt.Printf("  %s\n", r.Description)
		}
		if len(r.Globs) > 0 {
			fmt.Printf("  paths:\n")
			for _, g := range r.Globs {
				fmt.Printf("    - %s\n", g)
			}
		}
		fmt.Printf("\n%s\n", string(r.Body))
		return nil
	},
}

var rulesPathCmd = &cobra.Command{
	Use:   "path <abs-file-path>",
	Short: "Resolve which rules match a given file path.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		abs, err := rulesAbs(rulesWorkspace)
		if err != nil {
			return err
		}
		s := rules.New(abs)
		if _, err := s.Load(); err != nil {
			return err
		}
		// If the user gives a relative path, resolve against CWD.
		target := args[0]
		if !filepath.IsAbs(target) {
			if cwd, cerr := rulesGetwd(); cerr == nil {
				target = filepath.Join(cwd, target)
			}
		}
		matching := s.ForPath(target)
		if rulesFormat == "json" {
			return encodeJSON(struct {
				Path  string       `json:"path"`
				Rules []rules.Rule `json:"rules"`
			}{target, matching})
		}
		if len(matching) == 0 {
			fmt.Printf("no rules match %s\n", target)
			return nil
		}
		fmt.Printf("%s matches %d rule(s):\n", target, len(matching))
		for _, r := range matching {
			fmt.Printf("  - %s\n", r.Name)
		}
		return nil
	},
}

var rulesWhereCmd = &cobra.Command{
	Use:   "where",
	Short: "Print the on-disk rules directory",
	RunE: func(cmd *cobra.Command, args []string) error {
		abs, err := rulesAbs(rulesWorkspace)
		if err != nil {
			return err
		}
		fmt.Printf("%s/.sin-code/rules/\n", abs)
		return nil
	},
}

// encodeJSON marshals v to stdout with one trailing newline.
func encodeJSON(v interface{}) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
