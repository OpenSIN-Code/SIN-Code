// SPDX-License-Identifier: MIT
// Purpose: `sin-code rules` CLI — path-scoped rule surface. Mirrors
// Claude Code's `.claude/rules/<topic>.md` (Anthropic release v2.1),
// keeping the byte-stable lookup & pattern matching behind `--paths`
// lazy-loaded when the agent touches a matching file.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/rules"
)

var (
	rulesWorkspace string
	rulesFormat    string
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
		abs, err := filepath.Abs(rulesWorkspace)
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
		abs, err := filepath.Abs(rulesWorkspace)
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
		abs, err := filepath.Abs(rulesWorkspace)
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
			if cwd, cerr := os.Getwd(); cerr == nil {
				target = filepath.Join(cwd, target)
			}
		}
		matching := s.ForPath(target)
		if rulesFormat == "json" {
			return encodeJSON(struct {
				Path  string      `json:"path"`
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
		abs, err := filepath.Abs(rulesWorkspace)
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
