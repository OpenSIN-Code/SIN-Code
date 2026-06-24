// SPDX-License-Identifier: MIT
// Purpose: `sin-code status` — readiness snapshot / status report with
// markdown and JSON output. Issue #326.
// Also hosts `sin-code skills` — list and install bundled project-local skills
// using github.com/Songmu/skillsmith on the embedded skills.SkillsFS.
// Docs: cmd/sin-code/skills_cmd.go.doc.md
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/Songmu/skillsmith"
	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/status"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/summary"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

// NewStatusCmd builds the `status` cobra subcommand.
func NewStatusCmd() *cobra.Command {
	var (
		workspace string
		outPath   string
		markdown  bool
		jsonOut   bool
		sinceStr  string
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Print a readiness snapshot of the local SIN-Code state",
		Long: `sin-code status reads the goal queue, todo store, session store,
ledger, sin-debt markers, and skill status and produces a deterministic
readiness report. Missing or empty stores are reported as "No data yet"
instead of failing the command.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if markdown && jsonOut {
				return fmt.Errorf("--markdown and --json are mutually exclusive")
			}
			if workspace == "" {
				var err error
				workspace, err = os.Getwd()
				if err != nil {
					return fmt.Errorf("resolve workspace: %w", err)
				}
			}
			workspace, err := filepath.Abs(workspace)
			if err != nil {
				return fmt.Errorf("resolve workspace: %w", err)
			}

			cfg := status.Config{
				Workspace: workspace,
				Markdown:  markdown,
				JSON:      jsonOut,
				OutPath:   outPath,
			}
			if sinceStr != "" {
				since, err := time.Parse(time.RFC3339, sinceStr)
				if err != nil {
					return fmt.Errorf("invalid --since: %w", err)
				}
				cfg.Since = since
			}

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			rep, err := status.Collect(ctx, cfg)
			if err != nil {
				return err
			}

			var output []byte
			if jsonOut {
				output, err = status.RenderJSON(rep)
				if err != nil {
					return err
				}
			} else {
				output = []byte(status.RenderMarkdown(rep))
			}

			if outPath != "" {
				if err := os.WriteFile(outPath, output, filemode.Default()); err != nil {
					return fmt.Errorf("write output: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Status report written to %s\n", outPath)
				return nil
			}
			_, _ = cmd.OutOrStdout().Write(output)
			return nil
		},
	}
	cmd.Flags().StringVar(&workspace, "workspace", "", "Workspace directory to scan (default: current working directory)")
	cmd.Flags().StringVar(&outPath, "out", "", "Write report to this file instead of stdout")
	cmd.Flags().BoolVar(&markdown, "markdown", false, "Render markdown output (default)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "Render JSON output")
	cmd.Flags().StringVar(&sinceStr, "since", "", "Ledger time filter in RFC3339 (e.g. 2026-01-01T00:00:00Z)")
	return cmd
}

// skillsVersionHook is overridden by tests to avoid depending on the real
// build-time Version value.
var skillsVersionHook = func() string { return internal.Version }

// skillsNewSmithHook is overridden by tests to inject a fake skillsmith.Smith.
var skillsNewSmithHook = func(name, version string, fs fs.FS) (*skillsmith.Smith, error) {
	return skillsmith.New(name, version, fs)
}

// skillsInstallDirHook is overridden by tests to use a temporary directory.
var skillsInstallDirHook = skillsmith.InstallDirForScope

func resolveSkillsVersion() string {
	v := skillsVersionHook()
	if v == "" || v == "dev" {
		return "v0.0.0-dev"
	}
	return v
}

func NewSkillsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Manage bundled project-local skills embedded in the binary (install, list)",
		Long: `The skills subcommand discovers the agent skills bundled in the
sin-code binary and can install them into the user's agent config directory
(typically ~/.claude/skills/ or ~/.agents/skills/).`,
	}

	var jsonOut bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List bundled skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			listFS, err := skills.ListFS()
			if err != nil {
				return err
			}
			smith, err := skillsNewSmithHook("sin-code", resolveSkillsVersion(), listFS)
			if err != nil {
				return err
			}
			skillList, err := smith.List(cmd.Context())
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(skillList)
			}
			for _, s := range skillList {
				cmd.Printf("%-30s %s\n", s.Name, s.Description)
			}
			return nil
		},
	}
	listCmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")

	var dryRun, force bool
	var scope string
	installCmd := &cobra.Command{
		Use:   "install <skill-name>",
		Short: "Install a bundled skill into the agent skills directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			listFS, err := skills.ListFS()
			if err != nil {
				return err
			}
			smith, err := skillsNewSmithHook("sin-code", resolveSkillsVersion(), listFS)
			if err != nil {
				return err
			}
			destDir, err := skillsInstallDirHook(scope)
			if err != nil {
				return err
			}
			res, err := smith.Install(cmd.Context(), skillsmith.Options{
				Prefix: args[0],
				Scope:  scope,
				DryRun: dryRun,
				Force:  force,
			})
			if err != nil {
				return err
			}
			installed := res.Installed()
			cmd.Printf("installed %d skill(s) to %s\n", len(installed), destDir)
			for _, action := range installed {
				cmd.Printf("  - %s\n", action.Dir)
			}
			return nil
		},
	}
	installCmd.Flags().BoolVar(&dryRun, "dry-run", false, "simulate install")
	installCmd.Flags().BoolVar(&force, "force", false, "overwrite existing install")
	installCmd.Flags().StringVar(&scope, "scope", "claude", "agent scope (claude, agents, etc.)")

	cmd.AddCommand(listCmd, installCmd)
	return cmd
}

// _unusedSkillsImport prevents accidental removal of the skills import by goimports.
var _unusedSkillsImport = skills.SkillsFS

// _unusedFormat prevents fmt import removal during edits.
var _unusedFormat = fmt.Sprintf

// ============================================================================
// summary — build a deterministic session summary from the ledger
// ============================================================================

// NewSummaryCmd builds the `summary` cobra subcommand.
func NewSummaryCmd() *cobra.Command {
	var evidence bool
	cmd := &cobra.Command{
		Use:   "summary <session-id>",
		Short: "Build a deterministic summary from the session ledger",
		Long: `sin-code summary reads the semantic ledger for a session and
produces a markdown summary plus an optional one-line evidence string. The
summary is deterministic and does not call an LLM. It includes the
verification status, tools used, and a one-liner.`,
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			path := ledger.DefaultPath()
			if env := os.Getenv("SIN_CODE_LEDGER"); env != "" {
				path = env
			}
			store, err := ledger.Open(path)
			if err != nil {
				return err
			}
			defer store.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			sum, err := summary.Build(ctx, store, args[0])
			if err != nil {
				return err
			}
			if evidence {
				fmt.Println(summary.Evidence(sum))
				return nil
			}
			fmt.Print(summary.Format(sum))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&evidence, "evidence", "e", false, "Print one-line evidence string instead of markdown")
	return cmd
}
