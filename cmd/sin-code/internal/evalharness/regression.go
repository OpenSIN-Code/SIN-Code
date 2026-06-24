// SPDX-License-Identifier: MIT
// Purpose: compare two Runs case-by-case, surface improvements and
// regressions. The CLI exposes this as a CI gate via --fail-on-regress.
// Docs: regression.doc.md
package evalharness

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

// Delta describes how one case changed between two runs.
type Delta struct {
	CaseID   string
	OldScore float64
	NewScore float64
	Change   float64 // NewScore - OldScore
	Kind     string  // "improved" | "regressed" | "unchanged" | "added" | "removed"
}

// Comparison summarizes a baseline-vs-candidate diff.
type Comparison struct {
	BaselineRun  string
	CandidateRun string
	OldScore     float64
	NewScore     float64
	Deltas       []Delta
	Improved     int
	Regressed    int
}

// CompareRuns diffs two runs case-by-case. epsilon ignores tiny float noise.
//
// Renamed from Compare → CompareRuns in issue #171 to free the
// Compare name for the new four-arm (baseline / terse / lazy /
// <skill>) comparator. The old signature is preserved verbatim.
func CompareRuns(baseline, candidate Run, epsilon float64) Comparison {
	if epsilon <= 0 {
		epsilon = 0.001
	}
	oldByID := indexResults(baseline)
	newByID := indexResults(candidate)

	cmp := Comparison{BaselineRun: baseline.ID, CandidateRun: candidate.ID}
	cmp.OldScore, _ = baseline.Aggregate()
	cmp.NewScore, _ = candidate.Aggregate()

	seen := map[string]bool{}
	for id, nr := range newByID {
		seen[id] = true
		or, existed := oldByID[id]
		switch {
		case !existed:
			cmp.Deltas = append(cmp.Deltas, Delta{CaseID: id, NewScore: nr.Score, Change: nr.Score, Kind: "added"})
		default:
			change := nr.Score - or.Score
			kind := "unchanged"
			if change > epsilon {
				kind = "improved"
				cmp.Improved++
			} else if change < -epsilon {
				kind = "regressed"
				cmp.Regressed++
			}
			cmp.Deltas = append(cmp.Deltas, Delta{
				CaseID: id, OldScore: or.Score, NewScore: nr.Score, Change: change, Kind: kind,
			})
		}
	}
	for id, or := range oldByID {
		if !seen[id] {
			cmp.Deltas = append(cmp.Deltas, Delta{CaseID: id, OldScore: or.Score, Change: -or.Score, Kind: "removed"})
		}
	}
	return cmp
}

// HasRegressions reports whether the candidate regressed overall or
// per-case.
func (c Comparison) HasRegressions() bool {
	return c.Regressed > 0 || c.NewScore < c.OldScore
}

func indexResults(r Run) map[string]Result {
	m := make(map[string]Result, len(r.Results))
	for _, res := range r.Results {
		m[res.CaseID] = res
	}
	return m
}

// ── CLI subcommand tree (evalset run / list / compare) ─────────────────

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
			cmp := CompareRuns(base, cand, 0.001)
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
