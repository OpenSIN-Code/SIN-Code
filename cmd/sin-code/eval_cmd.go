// SPDX-License-Identifier: MIT
// Purpose: `sin-code eval` command tree — Golden Dataset evaluation,
// four-arm comparator, snapshot/diff, and SWE-bench harness.
// Split from main.go for single-responsibility file layout.
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package main

import (
	"github.com/spf13/cobra"
)

// ── eval command tree ─────────────────────────────────────────────

// NewEvalCmd returns the `sin-code eval` cobra command tree.
func NewEvalCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "eval",
		Short: "Run Golden Dataset evaluation suites",
		Long: `sin-code eval runs a Golden Dataset (JSON) against the agent loop
and reports the pass rate. Common CI pattern:

    sin-code eval run \
        --dataset evals/critical.json \
        --min-pass-rate 0.95 \
        --json

The four-arm comparator (issue #171) is opt-in via --arm:

    sin-code eval run --dataset evals/three-arm-example.json \
        --arm baseline,terse,lazy_skill,skill-code-create

Shortcuts:

    sin-code eval compare --dataset evals/three-arm-example.json
    sin-code eval snapshot --dataset evals/three-arm-example.json --out snap.json
    sin-code eval diff --snapshot snap-a.json --snapshot snap-b.json

Tracing is opt-in via --trace and ships to the chosen exporter.`,
	}
	cmd.AddCommand(
		newEvalRunCmd(),
		newEvalListCmd(),
		newEvalCompareCmd(),
		newEvalSnapshotCmd(),
		newEvalDiffCmd(),
		newEvalSWEBenchCmd(),
		newEvalSwebenchCmd(),
	)
	return cmd
}
