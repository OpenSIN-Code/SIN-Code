// SPDX-License-Identifier: MIT
// Code extracted from commands.go — Compress section.

package main

// sin-debt: shrink, upgrade: when a second compress-related command is added, merge into a shared file

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/compress"
)

// ============================================================================
// Compress command (sin-code compress)
// ============================================================================

// NewCompressCmd builds the `compress` cobra subcommand. Pattern mirrors
// NewHubCmd: root alias + 3 sub-commands (plan / apply / rollback) so
// each verb has a stable, scriptable form.
func NewCompressCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "compress",
		Short: "Lossless compaction for lessons / instincts / summaries / AGENTS.md",
		Long: `sin-code compress runs deterministic (dedupe + byte-budget + sort)
compaction over SIN-Code's long-lived stores, with an opt-in LLM summarization
step (Strategy=llm|hybrid). Every compaction is lossless: dropped entries are
preserved verbatim in a JSON snapshot under
~/.local/share/sin-code/compress-snapshots/<plan-id>.json. Use
'sin-code compress rollback <id>' to restore.

Targets:  lessons | instincts | summaries | memory | agents_md | all
Strategy: deterministic (default) | llm | hybrid
Examples:
  sin-code compress plan --target all --strategy deterministic
  sin-code compress plan --target lessons --keep-bytes 4096
  sin-code compress apply --target all --dry-run
  sin-code compress apply --target memory --keep-bytes 8192 --no-llm
  sin-code compress rollback plan-3a8e57b7c8f1fc11`,
	}
	cmd.AddCommand(newCompressPlanCmd())
	cmd.AddCommand(newCompressApplyCmd())
	cmd.AddCommand(newCompressRollbackCmd())
	return cmd
}

// compressCommon groups the flags shared by plan + apply. Both subcommands
// have `--target`, `--strategy`, `--keep-bytes`, `--keep`, `--recent-days`,
// `--lessons-db`, `--instinct-dir`, `--memory-db`, `--agents-md`,
// `--json` (machine-readable output).
type compressCommon struct {
	target      string
	strategy    string
	keepBytes   int
	keepMax     int
	recentDays  int
	lessonsDB   string
	instinctDir string
	memoryDB    string
	agentsMD    string
	asJSON      bool
}

// addCommonFlags wires the shared flags onto `cmd`.
func addCommonFlags(cmd *cobra.Command, c *compressCommon) {
	cmd.Flags().StringVar(&c.target, "target", "all",
		"target store: lessons | instincts | summaries | memory | agents_md | all")
	cmd.Flags().StringVar(&c.strategy, "strategy", "deterministic",
		"compaction strategy: deterministic | llm | hybrid")
	cmd.Flags().IntVar(&c.keepBytes, "keep-bytes", 4096,
		"byte budget per target — entries beyond this are dropped")
	cmd.Flags().IntVar(&c.keepMax, "keep", 0,
		"max kept entries per target — 0 means no cap")
	cmd.Flags().IntVar(&c.recentDays, "recent-days", 0,
		"drop entries older than this many days — 0 disables the age filter")
	cmd.Flags().StringVar(&c.lessonsDB, "lessons-db", "",
		"override path to the lessons.db (default: ~/.local/share/sin-code/lessons.db)")
	cmd.Flags().StringVar(&c.instinctDir, "instinct-dir", "",
		"override the instinct base directory (default: ~/.local/share/sin-code/instinct)")
	cmd.Flags().StringVar(&c.memoryDB, "memory-db", "",
		"override path to the memory bbolt db (default: os.UserConfigDir()/sin-code/memory.db)")
	cmd.Flags().StringVar(&c.agentsMD, "agents-md", "",
		"override path to the AGENTS.md file (default: walk up from cwd)")
	cmd.Flags().BoolVar(&c.asJSON, "json", false,
		"print machine-readable JSON instead of human-readable summary")
}

// toPaths turns the common bundle into a compress.Paths value.
func (c *compressCommon) toPaths() compress.Paths {
	return compress.Paths{
		LessonsDB: c.lessonsDB,
		Instinct:  c.instinctDir,
		Memory:    c.memoryDB,
		AgentsMD:  c.agentsMD,
	}
}

// toPlanOptions turns the common bundle into a compress.PlanOptions value.
func (c *compressCommon) toPlanOptions() compress.PlanOptions {
	return compress.PlanOptions{
		KeepBudgetBytes: c.keepBytes,
		KeepMaxEntries:  c.keepMax,
		KeepRecentDays:  c.recentDays,
	}
}

// parseTarget normalizes a --target flag value into a compress.Target.
// Empty / "all" → TargetAll. Unknown → error.
func parseTarget(s string) (compress.Target, error) {
	t := compress.Target(strings.ToLower(strings.TrimSpace(s)))
	if t == "" {
		return compress.TargetAll, nil
	}
	if !t.IsValid() {
		return "", fmt.Errorf("unknown target %q (use: lessons|instincts|summaries|memory|agents_md|all)", s)
	}
	return t, nil
}

// parseStrategy normalizes a --strategy flag value into a compress.Strategy.
// Empty / "deterministic" → StrategyDeterministic. Unknown → error.
func parseStrategy(s string) (compress.Strategy, error) {
	st := compress.Strategy(strings.ToLower(strings.TrimSpace(s)))
	if st == "" {
		return compress.StrategyDeterministic, nil
	}
	if !st.IsValid() {
		return "", fmt.Errorf("unknown strategy %q (use: deterministic|llm|hybrid)", s)
	}
	return st, nil
}

// newCompressPlanCmd builds the `plan` subcommand. plan is read-only; it
// builds the Plan, prints it, and exits.
func newCompressPlanCmd() *cobra.Command {
	c := &compressCommon{}
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute a compaction Plan and print its projected impact (no writes)",
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := parseTarget(c.target)
			if err != nil {
				return err
			}
			strategy, err := parseStrategy(c.strategy)
			if err != nil {
				return err
			}
			p, err := compress.BuildPlan(target, strategy, c.toPaths(), c.toPlanOptions())
			if err != nil {
				return err
			}
			return renderPlan(p, c.asJSON)
		},
	}
	addCommonFlags(cmd, c)
	return cmd
}

// newCompressApplyCmd builds the `apply` subcommand. apply is the only
// path that writes; it stops at the snapshot step (atomic) and the
// per-target rewrites. --dry-run stops before the snapshot.
func newCompressApplyCmd() *cobra.Command {
	c := &compressCommon{}
	var dryRun, noLLM bool
	cmd := &cobra.Command{
		Use:   "apply",
		Short: "Compute a Plan, snapshot the dropped entries, and rewrite the target",
		RunE: func(_ *cobra.Command, _ []string) error {
			target, err := parseTarget(c.target)
			if err != nil {
				return err
			}
			strategy, err := parseStrategy(c.strategy)
			if err != nil {
				return err
			}
			if noLLM && (strategy == compress.StrategyLLM || strategy == compress.StrategyHybrid) {
				strategy = compress.StrategyDeterministic
				fmt.Fprintln(os.Stderr, "compress: --no-llm downgrades strategy=llm|hybrid to deterministic")
			}
			p, err := compress.BuildPlan(target, strategy, c.toPaths(), c.toPlanOptions())
			if err != nil {
				return err
			}
			rep, err := compress.Apply(p, c.toPaths(), compress.ApplyOptions{DryRun: dryRun, Reason: "sin-code compress apply"})
			if err != nil {
				return err
			}
			return renderReport(p, rep, c.asJSON)
		},
	}
	addCommonFlags(cmd, c)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "snapshot-current would write a snapshot —skip the snapshot + writes")
	cmd.Flags().BoolVar(&noLLM, "no-llm", false, "force Strategy=deterministic even if --strategy=llm|hybrid")
	// Re-add the dry-run default description to be friendlier.
	cmd.Flag("dry-run").Usage = "plan-only: print the projected impact, do not write"
	return cmd
}

// newCompressRollbackCmd builds the `rollback` subcommand. Rollback
// reads the snapshot file from ~/.local/share/sin-code/compress-snapshots/<id>.json
// and restores the original entries byte-for-byte.
func newCompressRollbackCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "rollback <snapshot-id>",
		Short: "Restore dropped entries from a snapshot file",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if err := compress.Rollback(args[0]); err != nil {
				return err
			}
			fmt.Printf("rollback %s: ok\n", args[0])
			return nil
		},
	}
}

// renderPlan prints a Plan in human or JSON form.
func renderPlan(p compress.Plan, asJSON bool) error {
	if asJSON {
		b, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("compress plan (id=%s hash=%s)\n", p.ID, p.PlanHash)
	fmt.Printf("  target   : %s\n", p.Target)
	fmt.Printf("  strategy : %s\n", p.Strategy)
	fmt.Printf("  entries  : %d → %d (drops=%d, merges=%d)\n",
		p.Stats.OriginalEntries, p.Stats.ProjectedEntries,
		p.Stats.Drops, p.Stats.Merges)
	fmt.Printf("  bytes    : %d → %d (ratio %.2f)\n",
		p.Stats.OriginalBytes, p.Stats.ProjectedBytes, p.Stats.ProjectedRatio)
	if len(p.Warnings) > 0 {
		fmt.Println("  warnings :")
		for _, w := range p.Warnings {
			fmt.Println("    - " + w)
		}
	}
	return nil
}

// renderReport prints an ApplyReport in human or JSON form.
func renderReport(p compress.Plan, rep compress.ApplyReport, asJSON bool) error {
	if asJSON {
		b, err := json.MarshalIndent(rep, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("compress report (plan=%s snap=%s)\n", rep.PlanID, rep.SnapshotID)
	fmt.Printf("  original : %d bytes\n", rep.OriginalBytes)
	fmt.Printf("  kept     : %d bytes  (ratio %.2f)\n", rep.KeptBytes, rep.Ratio)
	if snap := rep.SnapshotPath; snap != "" {
		fmt.Println("  snapshot :", relOrSame(snap))
	}
	for _, tr := range rep.PerTarget {
		fmt.Printf("  %-10s : %d → %d entries, %d → %d bytes\n",
			string(tr.Target), tr.BeforeEntries, tr.AfterEntries,
			tr.BeforeBytes, tr.AfterBytes)
	}
	if len(rep.Warnings) > 0 {
		fmt.Printf("  warnings : %d (use --json for details)\n", len(rep.Warnings))
	}
	if len(p.Warnings) > 0 {
		fmt.Printf("  plan warnings : %d (use --json for details)\n", len(p.Warnings))
	}
	_ = p
	return nil
}

// relOrSame returns the relative path of `abs` if it's under
// cwd; this avoids CWD-prefix noise in the snapshot path output
// without erasing the basename.
func relOrSame(abs string) string {
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
			return rel
		}
	}
	return abs
}
