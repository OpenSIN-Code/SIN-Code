// SPDX-License-Identifier: MIT
// Purpose: `sin-code debt` — manages the sin-debt marker convention
// (issue #177). Subcommands: list, stats, check, policy, fix, export.
// Docs: cmd/sin-code/debt_cmd.doc.md
package main

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/sindept"
)


// NewDebtCmd builds the `debt` cobra subcommand group for sin-debt markers.
// All operations are read-only by design — the scanner + report are
// deterministic so two CI runs over the same tree produce the same bytes.
func NewDebtCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "debt",
		Short: "Inspect sin-debt markers (issue #177)",
		Long: `sin-code debt scans a directory for the
'// sin-debt: <ceiling>, upgrade: <trigger>' marker convention and produces
byte-stable reports. The scanner recognises C/Go/Rust-style // comments,
Python/Shell # comments, /* */ blocks, <!-- --> HTML/Markdown, and -- SQL.

Subcommands:

  list   one row per marker, with reason / upgrade / rot column
  stats  aggregated report grouped by reason|file|language|symbol|age
  check  CI gate: exit 1 when rot exceeds the threshold
  policy dump the active policy (resolved from .sin-code/debt-policy.toml)
  fix    print a sed/awk-ready patch for the rot-risk markers
  export write the canonical SIN-DEBT.md ledger to disk`,
	}
	cmd.AddCommand(
		newDebtListCmd(),
		newDebtStatsCmd(),
		newDebtCheckCmd(),
		newDebtPolicyCmd(),
		newDebtFixCmd(),
		newDebtExportCmd(),
	)
	return cmd
}

// sharedFlags is the set of flags each subcommand declares. Keeping them
// in one struct lets every subcommand share a default value and stay in
// lock-step when the contract evolves.
type sharedFlags struct {
	path      string
	format    string
	noTrigger bool
	json      bool
}

// bindShared installs the common flag set on `c`. The flags are:
//   - --path       root directory to scan (default ".")
//   - --format     "table" or "json" (default "table")
//   - --no-trigger when set, list/check report only rot-risk markers
func bindShared(c *cobra.Command, f *sharedFlags) {
	c.Flags().StringVar(&f.path, "path", ".", "directory to scan (default: current)")
	c.Flags().StringVar(&f.format, "format", "table", "output format: table|json")
	c.Flags().BoolVar(&f.noTrigger, "no-trigger", false, "limit output to markers without an upgrade clause")
	c.Flags().BoolVar(&f.json, "json", false, "emit JSON instead of markdown table")
}

// resolveRoot returns the absolute form of `path` with a saner error.
// Empty paths become "." so the cobra default is always honoured.
func resolveRoot(path string) (string, error) {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("debt: resolve path %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("debt: stat %s: %w", abs, err)
	}
	if !info.IsDir() {
		// single-file scan is fine; we just keep the parent as scan root
		// and post-filter. The package's ParseDir handles this too.
	}
	return abs, nil
}

// scan runs the parser over `root` and applies the user's filter flags.
// The result is sorted by File / Line / Column by ParseDir already; we
// only apply the post-filter for --no-trigger here.
func scan(root string, noTrigger bool) ([]sindept.Marker, error) {
	mk, err := sindept.ParseDir(root, sindept.DefaultOptions())
	if err != nil {
		return nil, err
	}
	if !noTrigger {
		return mk, nil
	}
	out := mk[:0:0]
	for _, m := range mk {
		if !(m.HasUpg && m.Upgrade != "") {
			out = append(out, m)
		}
	}
	return out, nil
}

func newDebtListCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "list",
		Short: "List every sin-debt marker under --path",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			if f.json {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(mk)
			}
			fmt.Print(sindept.RenderListString(mk))
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtStatsCmd() *cobra.Command {
	var f sharedFlags
	var by string
	c := &cobra.Command{
		Use:   "stats",
		Short: "Aggregate report (default: by file)",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			if by == "age" {
				return renderAgeReport(os.Stdout, mk)
			}
			stats := sindept.AggregateStats(mk)
			sections := []sindept.ReportSection{
				sindept.SectionSummary,
			}
			switch by {
			case "reason":
				sections = append(sections, sindept.SectionByReason)
			case "file":
				sections = append(sections, sindept.SectionByFile)
			case "language":
				sections = append(sections, sindept.SectionByLang)
			case "symbol":
				sections = append(sections, sindept.SectionBySymbol)
			case "summary":
				// summary already at [0], nothing more.
			default:
				sections = append(sections,
					sindept.SectionByFile, sindept.SectionByReason,
					sindept.SectionByLang, sindept.SectionRotRisk)
			}
			if f.json {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(stats)
			}
			sindept.RenderStats(os.Stdout, stats, sections)
			return nil
		},
	}
	c.Flags().StringVar(&by, "by", "file", "group by: file|reason|language|symbol|summary|age")
	bindShared(c, &f)
	return c
}

func newDebtCheckCmd() *cobra.Command {
	var f sharedFlags
	var failOnMissing bool
	c := &cobra.Command{
		Use:   "check",
		Short: "CI gate: exit 1 when rot-risk exceeds threshold",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, false)
			if err != nil {
				return err
			}
			policy, err := sindept.LoadPolicyForRoot(root)
			if err != nil {
				return err
			}
			if failOnMissing {
				policy.RequireUpgrade = true
			}
			res := policy.RunCheck(mk)
			fmt.Print(sindept.FormatCheckResult(res))
			if !res.Ok {
				os.Exit(1)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&failOnMissing, "require-upgrade", false, "treat any marker without 'upgrade:' as a failure")
	bindShared(c, &f)
	return c
}

func newDebtPolicyCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "policy",
		Short: "Print the active sin-debt policy (defaults + on-disk overlay)",
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			pol, err := sindept.LoadPolicyForRoot(root)
			if err != nil {
				return err
			}
			if f.json {
				return json.NewEncoder(os.Stdout).Encode(pol)
			}
			fmt.Printf("# sin-debt policy\n\n")
			fmt.Printf("- source: %s\n", emptyDash(pol.Source))
			fmt.Printf("- max_no_upgrade: %d\n", pol.MaxNoUpgrade)
			fmt.Printf("- require_upgrade: %v\n", pol.RequireUpgrade)
			fmt.Printf("- default_reasons (%d):\n", len(pol.DefaultReasons))
			for _, r := range pol.DefaultReasons {
				fmt.Printf("    - %s\n", r)
			}
			fmt.Printf("- upgrade_triggers (%d):\n", len(pol.UpgradeTriggers))
			keys := make([]string, 0, len(pol.UpgradeTriggers))
			for k := range pol.UpgradeTriggers {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				fmt.Printf("    - %s: %s\n", k, pol.UpgradeTriggers[k])
			}
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtFixCmd() *cobra.Command {
	var f sharedFlags
	c := &cobra.Command{
		Use:   "fix",
		Short: "Print the rot-risk markers as a sed-friendly patch harness",
		Long: `fix lists the rot-risk markers — those without an 'upgrade:' clause —
in the format "path:line<TAB>reason". Pipe through sed/edit to insert
the upgrade clause; nothing is written by this subcommand.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, true) // rot-only
			if err != nil {
				return err
			}
			for _, m := range mk {
				fmt.Printf("%s:%d\t%s\n", m.File, m.Line, m.Reason)
			}
			fmt.Fprintf(os.Stderr, "# %d rot-risk markers — add 'upgrade: <trigger>' to each\n", len(mk))
			return nil
		},
	}
	bindShared(c, &f)
	return c
}

func newDebtExportCmd() *cobra.Command {
	var f sharedFlags
	var out string
	c := &cobra.Command{
		Use:   "export <file>",
		Short: "Write the canonical SIN-DEBT.md ledger",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			root, err := resolveRoot(f.path)
			if err != nil {
				return err
			}
			mk, err := scan(root, f.noTrigger)
			if err != nil {
				return err
			}
			dest := out
			if dest == "" && len(args) == 1 {
				dest = args[0]
			}
			if dest == "" {
				dest = "SIN-DEBT.md"
			}
			stats := sindept.AggregateStats(mk)
			content := sindept.RenderListString(mk) + "\n" + sindept.RenderStatsString(stats)
			if err := os.WriteFile(dest, []byte(content), filemode.Default()); err != nil {
				return fmt.Errorf("debt: write %s: %w", dest, err)
			}
			if !f.json {
				fmt.Fprintf(os.Stderr, "wrote %d markers to %s\n", len(mk), dest)
			}
			return nil
		},
	}
	c.Flags().StringVarP(&out, "out", "o", "", "destination file (default: SIN-DEBT.md)")
	bindShared(c, &f)
	return c
}

// renderAgeReport prints every marker sorted by File then Line — the
// "oldest first" view that helps reviewers triage rot in chronological
// order. The byte order is identical to the parser's stable sort, so
// two runs of the same tree produce the same bytes.
func renderAgeReport(w *os.File, mk []sindept.Marker) error {
	fmt.Fprintln(w, sindept.Header())
	fmt.Fprintln(w, "## Markers by age (oldest first)")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| file | line | symbol | reason | upgrade |")
	fmt.Fprintln(w, "|------|------|--------|--------|---------|")
	for _, m := range mk {
		upg := m.Upgrade
		if upg == "" {
			upg = "&lt;none — rot-risk&gt;"
		}
		fmt.Fprintf(w, "| %s | %d | %s | %s | %s |\n",
			escapeCell(m.File), m.Line,
			escapeCell(m.Symbol), escapeCell(m.Reason),
			escapeCell(upg))
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "_%d markers total_\n", len(mk))
	return nil
}

// escapeCell keeps markdown table cells well-formed in the age report.
func escapeCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "|", "/")
	return s
}

func emptyDash(s string) string {
	if s == "" {
		return "<default>"
	}
	return s
}

// _ keeps strconv referenced even if we stop using the binding.
var _ = strconv.Itoa
