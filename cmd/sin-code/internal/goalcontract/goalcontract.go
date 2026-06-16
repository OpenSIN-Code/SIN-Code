// SPDX-License-Identifier: MIT
// Purpose: Goal Contract — a machine-checkable Definition-of-Done for an
// autonomous goal. A contract bundles deterministic checks (build/test/lint/
// predicate/diff-scope, reused from internal/orchestrator) with non-mechanical
// semantic criteria that an LLM judge evaluates in the stop-gate
// (internal/stopgate). Completion authority is thereby decoupled from the
// worker: a goal is only "done" when its contract is satisfied, not when the
// model stops emitting tool calls.
//
// Docs: goalcontract.doc.md
package goalcontract

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// GoalContract is the Definition-of-Done for one goal. The zero value is a
// valid, empty contract (no criteria) that the stop-gate treats as
// "verify-gate only" — i.e. backwards-compatible with the pre-contract loop.
type GoalContract struct {
	GoalID              string               `json:"goal_id,omitempty"`
	DeterministicChecks []orchestrator.Check `json:"deterministic_checks,omitempty"`
	// SemanticCriteria are natural-language acceptance criteria that cannot
	// be checked mechanically (e.g. "documentation updated", "goal fully
	// addressed"). They are evaluated by the LLM judge in the stop-gate.
	SemanticCriteria []string `json:"semantic_criteria,omitempty"`
	MaxFilesChanged  int      `json:"max_files_changed,omitempty"`
	MaxLinesChanged  int      `json:"max_lines_changed,omitempty"`
}

// IsEmpty reports whether the contract carries no acceptance criteria at all.
// An empty contract means the stop-gate falls back to the verify-gate result
// only (no extra deterministic checks, no semantic judging).
func (c *GoalContract) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.DeterministicChecks) == 0 && len(c.SemanticCriteria) == 0 &&
		c.MaxFilesChanged == 0 && c.MaxLinesChanged == 0
}

// jsonMarshal is a test seam around json.Marshal.
var jsonMarshal = json.Marshal

// Marshal serializes the contract to a compact JSON string for persistence in
// the goal queue. An empty contract marshals to "".
func (c *GoalContract) Marshal() (string, error) {
	if c.IsEmpty() {
		return "", nil
	}
	b, err := jsonMarshal(c)
	if err != nil {
		return "", fmt.Errorf("goalcontract: marshal: %w", err)
	}
	return string(b), nil
}

// Unmarshal parses a contract previously produced by Marshal. An empty or
// blank string yields an empty (non-nil) contract so callers never have to
// nil-check.
func Unmarshal(s string) (*GoalContract, error) {
	c := &GoalContract{}
	if strings.TrimSpace(s) == "" {
		return c, nil
	}
	if err := json.Unmarshal([]byte(s), c); err != nil {
		return nil, fmt.Errorf("goalcontract: unmarshal: %w", err)
	}
	return c, nil
}

// ResolveOptions configures contract resolution for a goal.
type ResolveOptions struct {
	Workspace string
	GoalID    string
	// ContractFile, when set, loads an explicit JSON contract (highest
	// priority). Relative paths resolve against Workspace.
	ContractFile string
	// Criteria are inline semantic acceptance criteria (highest priority,
	// merged with everything else).
	Criteria []string
	// DoneWhen is a shell predicate that must exit 0 for the goal to be
	// considered done. Added as a deterministic predicate check.
	DoneWhen string
	// VerifyCmd is the fallback deterministic check when nothing else is
	// configured and the workspace is not auto-detectable.
	VerifyCmd string
	// AutoDetect enables language/repo auto-detection (Go repo -> go build/
	// test/vet plus a "no new TODO/FIXME" guard). Defaults on via Resolve.
	AutoDetect bool
	// IncludeBaseline merges the always-on SinCode Loop System Definition-of-
	// Done baseline (tests for new behavior, no debug scaffolding, goal fully
	// addressed, docs kept in sync) into the contract. It is additive and
	// deduped against everything else. Callers gate this on
	// BaselineEnabled(--no-baseline) so it is ON by default everywhere.
	IncludeBaseline bool
}

// Resolve builds a GoalContract from the configured sources, in priority
// order: (1) explicit file + inline criteria + done-when predicate, then
// (2) auto-detected repo checks, and finally (3) the verify-cmd fallback.
// Sources are additive — an explicit file does not suppress auto-detection,
// it augments it — so the strictest reasonable contract is produced.
func Resolve(opts ResolveOptions) (*GoalContract, error) {
	c := &GoalContract{GoalID: opts.GoalID}

	// (1) Explicit contract file.
	if strings.TrimSpace(opts.ContractFile) != "" {
		path := opts.ContractFile
		if !filepath.IsAbs(path) {
			path = filepath.Join(opts.Workspace, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("goalcontract: read %s: %w", path, err)
		}
		loaded, err := Unmarshal(string(data))
		if err != nil {
			return nil, err
		}
		c.DeterministicChecks = append(c.DeterministicChecks, loaded.DeterministicChecks...)
		c.SemanticCriteria = append(c.SemanticCriteria, loaded.SemanticCriteria...)
		if loaded.MaxFilesChanged > 0 {
			c.MaxFilesChanged = loaded.MaxFilesChanged
		}
		if loaded.MaxLinesChanged > 0 {
			c.MaxLinesChanged = loaded.MaxLinesChanged
		}
	}

	// (1b) Inline semantic criteria.
	for _, cr := range opts.Criteria {
		if s := strings.TrimSpace(cr); s != "" {
			c.SemanticCriteria = append(c.SemanticCriteria, s)
		}
	}

	// (1c) done-when predicate.
	if dw := strings.TrimSpace(opts.DoneWhen); dw != "" {
		c.DeterministicChecks = append(c.DeterministicChecks, orchestrator.Check{
			Kind:    orchestrator.CheckPredicate,
			Name:    "done-when",
			Cmd:     []string{"sh", "-c", dw},
			Timeout: 5 * time.Minute,
		})
	}

	// (2) Auto-detected repo checks (additive).
	if opts.AutoDetect {
		for _, ck := range autoDetectChecks(opts.Workspace) {
			if !hasCheckNamed(c.DeterministicChecks, ck.Name) {
				c.DeterministicChecks = append(c.DeterministicChecks, ck)
			}
		}
	}

	// (2b) Always-on SinCode Loop System baseline (additive, deduped). This is
	// what makes "write tests / debug / update docs / finish the job" implicit
	// for every goal — the user never has to ask for it again.
	if opts.IncludeBaseline {
		base := Baseline(opts.Workspace)
		for _, ck := range base.DeterministicChecks {
			if !hasCheckNamed(c.DeterministicChecks, ck.Name) {
				c.DeterministicChecks = append(c.DeterministicChecks, ck)
			}
		}
		for _, cr := range base.SemanticCriteria {
			if !hasCriterion(c.SemanticCriteria, cr) {
				c.SemanticCriteria = append(c.SemanticCriteria, cr)
			}
		}
	}

	// (3) Fallback: verify-cmd as the single deterministic check when we
	// still have no deterministic coverage at all.
	if len(c.DeterministicChecks) == 0 && strings.TrimSpace(opts.VerifyCmd) != "" {
		c.DeterministicChecks = append(c.DeterministicChecks, orchestrator.Check{
			Kind:    orchestrator.CheckPredicate,
			Name:    "verify-cmd",
			Cmd:     []string{"sh", "-c", opts.VerifyCmd},
			Timeout: 10 * time.Minute,
		})
	}

	return c, nil
}

// autoDetectChecks returns deterministic checks inferred from the workspace
// contents. Today this covers Go repositories; the list is intentionally
// easy to extend per ecosystem.
func autoDetectChecks(workspace string) []orchestrator.Check {
	var checks []orchestrator.Check
	if fileExists(filepath.Join(workspace, "go.mod")) {
		checks = append(checks, orchestrator.DefaultGoChecks()...)
		// "No new TODO/FIXME/XXX in the working tree diff" — a cheap guard
		// against the agent leaving its work half-finished and declaring done.
		checks = append(checks, orchestrator.Check{
			Kind:    orchestrator.CheckPredicate,
			Name:    "no-new-todos",
			Cmd:     []string{"sh", "-c", noNewTodosScript},
			Timeout: 1 * time.Minute,
		})
	}
	return checks
}

// noNewTodosScript fails (exit 1) when the staged+unstaged diff against HEAD
// introduces a new TODO/FIXME/XXX marker. It is best-effort: outside a git
// repo it exits 0 (cannot judge, never block).
const noNewTodosScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
added=$(git diff HEAD | grep -E '^\+' | grep -Ei 'TODO|FIXME|XXX' || true)
if [ -n "$added" ]; then
  echo "introduced new TODO/FIXME/XXX markers:"; echo "$added"; exit 1
fi
exit 0`

func hasCheckNamed(checks []orchestrator.Check, name string) bool {
	for _, c := range checks {
		if c.Name == name {
			return true
		}
	}
	return false
}

func hasCriterion(criteria []string, want string) bool {
	want = strings.TrimSpace(want)
	for _, c := range criteria {
		if strings.TrimSpace(c) == want {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
