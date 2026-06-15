// SPDX-License-Identifier: MIT
// Purpose: `sin-code skills` — list and install bundled project-local skills.
// Uses github.com/Songmu/skillsmith on the embedded skills.SkillsFS.
// Docs: cmd/sin-code/skills_cmd.go.doc.md
package main

import (
	"encoding/json"
	"fmt"
	"io/fs"

	"github.com/spf13/cobra"

	"github.com/Songmu/skillsmith"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal"
	"github.com/OpenSIN-Code/SIN-Code/skills"
)

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
		Short: "List and install bundled project-local skills",
		Long: `The skills subcommand discovers the agent skills bundled in the
sin-code binary and can install them into the user's agent config directory
(typically ~/.claude/skills/ or ~/.agents/skills/).`,
	}

	var jsonOut bool
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List bundled skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			smith, err := skillsNewSmithHook("sin-code", resolveSkillsVersion(), skills.SkillsFS)
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
			smith, err := skillsNewSmithHook("sin-code", resolveSkillsVersion(), skills.SkillsFS)
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
