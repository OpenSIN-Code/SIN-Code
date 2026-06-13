// SPDX-License-Identifier: MIT
// Purpose: sin update — top-level subcommand for the full update flow.
// Docs: self-update.doc.md
// Issue #33.
package internal

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
)

var UpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update SIN-Code stack (Python packages, Go binaries, skills)",
	Long: `update performs a complete self-update of the SIN-Code stack:
  • Python packages (pipx upgrade sin-*)
  • Go binaries (rebuild 7 standalone tools from local source)
  • Skills (pipx-managed sin-* ecosystem packages)

Each phase creates a snapshot in ~/.local/state/sin-code/updates/<ts>/
for rollback. After the update, sin-code doctor runs as a non-fatal
health check.

Examples:
  sin-code update                      # Full update
  sin-code update --check              # Show what would be updated
  sin-code update --python-only        # Only pipx packages
  sin-code update --go-only --force    # Force-rebuild all Go tools
  sin-code update --rollback           # Restore previous snapshot
  sin-code update --dry-run            # Print plan, do not execute`,
	RunE: runUpdate,
}

func init() {
	UpdateCmd.Flags().Bool("python-only", false, "Only upgrade Python packages (pipx)")
	UpdateCmd.Flags().Bool("go-only", false, "Only rebuild Go binaries")
	UpdateCmd.Flags().Bool("skills-only", false, "Only upgrade skills")
	UpdateCmd.Flags().Bool("check", false, "Check for updates without applying")
	UpdateCmd.Flags().BoolP("dry-run", "n", false, "Show what would be done, do not modify")
	UpdateCmd.Flags().Bool("force", false, "Force rebuild even if versions match")
	UpdateCmd.Flags().Bool("rollback", false, "Rollback to previous snapshot")
	UpdateCmd.Flags().Bool("skip-doctor", false, "Skip post-update doctor check")
	UpdateCmd.Flags().String("state-root", "", "Override ~/.local/state/sin-code")
	UpdateCmd.Flags().Int("keep-snapshots", 10, "How many snapshots to retain (0 = unlimited)")
}

type UpdateOptions struct {
	PythonOnly    bool
	GoOnly        bool
	SkillsOnly    bool
	CheckOnly     bool
	DryRun        bool
	Force         bool
	Rollback      bool
	SkipDoctor    bool
	StateRoot     string
	KeepSnapshots int
}

func parseUpdateFlags(cmd *cobra.Command) (UpdateOptions, error) {
	py, _ := cmd.Flags().GetBool("python-only")
	goOnly, _ := cmd.Flags().GetBool("go-only")
	sk, _ := cmd.Flags().GetBool("skills-only")
	ch, _ := cmd.Flags().GetBool("check")
	dr, _ := cmd.Flags().GetBool("dry-run")
	fo, _ := cmd.Flags().GetBool("force")
	rb, _ := cmd.Flags().GetBool("rollback")
	sd, _ := cmd.Flags().GetBool("skip-doctor")
	sr, _ := cmd.Flags().GetString("state-root")
	ks, _ := cmd.Flags().GetInt("keep-snapshots")

	count := 0
	if py { count++ }
	if goOnly { count++ }
	if sk { count++ }
	if count > 1 {
		return UpdateOptions{}, fmt.Errorf("--python-only, --go-only, --skills-only are mutually exclusive")
	}
	return UpdateOptions{
		PythonOnly: py, GoOnly: goOnly, SkillsOnly: sk,
		CheckOnly: ch, DryRun: dr, Force: fo, Rollback: rb,
		SkipDoctor: sd, StateRoot: sr, KeepSnapshots: ks,
	}, nil
}

func runUpdate(cmd *cobra.Command, args []string) error {
	opts, err := parseUpdateFlags(cmd)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if opts.Rollback {
		_, err := runRollback(ctx, opts)
		return err
	}

	if opts.CheckOnly || opts.DryRun {
		return runCheck(ctx, opts)
	}

	bm, err := NewBackupManager()
	if err != nil {
		return err
	}
	if opts.StateRoot != "" {
		bm.StateRoot = opts.StateRoot
	}
	snapshotDir, err := bm.Create()
	if err != nil {
		return fmt.Errorf("create snapshot: %w", err)
	}
	manifest := NewManifest(currentVersion)
	if err := manifest.Write(snapshotDir); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}

	results := []*PhaseResult{}
	runPy := !opts.GoOnly && !opts.SkillsOnly
	runGo := !opts.PythonOnly && !opts.SkillsOnly

	if runPy {
		r, err := RunPythonPhase(ctx, opts)
		if err != nil { return err }
		results = append(results, r)
	}
	if runGo {
		r, err := RunGoPhase(ctx, opts)
		if err != nil { return err }
		results = append(results, r)
	}

	printPhaseSummary(results)

	manifest.Success = true
	manifest.Write(snapshotDir)

	if !opts.SkipDoctor {
		if err := runDoctorNonFatal(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[warn] doctor: %v\n", err)
		}
	}

	if err := bm.Prune(opts.KeepSnapshots); err != nil {
		fmt.Fprintf(os.Stderr, "[warn] prune snapshots: %v\n", err)
	}
	return nil
}
