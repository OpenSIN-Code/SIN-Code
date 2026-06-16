// SPDX-License-Identifier: MIT
// Purpose: `sin eval ...` subcommand tree — `run`, `list`, `compare`.
// Provide a subject factory so different runtimes can be evaluated by
// the same CLI.
// Docs: cli.doc.md
package evalharness

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// NewCommand returns `sin eval ...`. Provide a factory that builds
// the Subject (your agent/verify runner) for a given subject name,
// so `eval run` can target different runtimes.
func NewCommand(subjectFactory func(name string) (Subject, Scorer, error)) *cobra.Command {
	root := &cobra.Command{Use: "evalset", Short: "Eval-driven development harness (EvalSets, Runs, regression compare)"}

	var subjectName string
	run := &cobra.Command{
		Use:   "run [set]",
		Short: "Run an eval set and record the results",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			store := NewStore("")
			set, err := store.LoadSet(args[0])
			if err != nil {
				return err
			}
			subj, scorer, err := subjectFactory(subjectName)
			if err != nil {
				return err
			}
			runner := Runner{
				Subject: subj, Scorer: scorer, SubjectName: subjectName,
				Progress: func(done, total int, last Result) {
					fmt.Printf("  [%d/%d] %-20s score=%.2f pass=%v\n", done, total, last.CaseID, last.Score, last.Passed)
				},
			}
			result, err := runner.Execute(context.Background(), set)
			if err != nil {
				return err
			}
			if err := store.SaveRun(result); err != nil {
				return err
			}
			score, pass := result.Aggregate()
			fmt.Printf("\nRun %s — score=%.3f pass-rate=%.0f%% (%d cases)\n", result.ID, score, pass*100, len(result.Results))
			return nil
		},
	}
	run.Flags().StringVar(&subjectName, "subject", "agent", "subject to evaluate")

	list := &cobra.Command{
		Use:   "list [set]",
		Short: "List recorded runs",
		RunE: func(c *cobra.Command, args []string) error {
			store := NewStore("")
			setName := ""
			if len(args) == 1 {
				setName = args[0]
			}
			runs, err := store.ListRuns(setName)
			if err != nil {
				return err
			}
			for _, r := range runs {
				score, pass := r.Aggregate()
				fmt.Printf("  %-28s %s score=%.3f pass=%.0f%%\n", r.ID, r.StartedAt.Format("2006-01-02 15:04"), score, pass*100)
			}
			return nil
		},
	}

	var failOnRegress bool
	compare := &cobra.Command{
		Use:   "compare [baseline-run] [candidate-run]",
		Short: "Compare two runs for regressions",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			store := NewStore("")
			base, err := store.LoadRun(args[0])
			if err != nil {
				return err
			}
			cand, err := store.LoadRun(args[1])
			if err != nil {
				return err
			}
			cmp := Compare(base, cand, 0.001)
			fmt.Printf("score: %.3f -> %.3f  (improved=%d regressed=%d)\n", cmp.OldScore, cmp.NewScore, cmp.Improved, cmp.Regressed)
			for _, d := range cmp.Deltas {
				if d.Kind == "improved" || d.Kind == "regressed" {
					fmt.Printf("  %-9s %-20s %.2f -> %.2f (%+.2f)\n", d.Kind, d.CaseID, d.OldScore, d.NewScore, d.Change)
				}
			}
			if failOnRegress && cmp.HasRegressions() {
				return fmt.Errorf("regressions detected")
			}
			return nil
		},
	}
	compare.Flags().BoolVar(&failOnRegress, "fail-on-regress", false, "exit non-zero if regressions found (CI gate)")

	root.AddCommand(run, list, compare)
	return root
}
