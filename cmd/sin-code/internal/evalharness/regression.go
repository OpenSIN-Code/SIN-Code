// SPDX-License-Identifier: MIT
// Purpose: compare two Runs case-by-case, surface improvements and
// regressions. The CLI exposes this as a CI gate via --fail-on-regress.
// Docs: regression.doc.md
package evalharness

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

// Compare diffs two runs case-by-case. epsilon ignores tiny float noise.
func Compare(baseline, candidate Run, epsilon float64) Comparison {
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
