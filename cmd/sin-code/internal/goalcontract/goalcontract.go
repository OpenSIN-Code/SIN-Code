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

	// PostCompletionGoals are sub-goals auto-spawned AFTER the main goal
	// verifies. Each one runs as a child and the parent only truly finalizes
	// when all post-completion children are verified too. This is how the
	// loop ensures docs, CHANGELOG, MASTER_TODO, and README are ALWAYS
	// updated without a human reminder (loop-001).
	PostCompletionGoals []PostGoal `json:"post_completion_goals,omitempty"`

	// MinSubGoals, when > 0, is a semantic hint injected into the stop-gate
	// telling the LLM judge to confirm a large goal was decomposed into at
	// least this many child goals when the scope warranted it (loop-005).
	MinSubGoals int `json:"min_sub_goals,omitempty"`

	// DisablePostGoals persists a per-goal opt-out of auto-spawned
	// post-completion doc/changelog goals (loop-001), set via
	// `goal add --no-post-goals`.
	DisablePostGoals bool `json:"disable_post_goals,omitempty"`
}

// PostGoal is one automatically spawned follow-up goal (loop-001).
type PostGoal struct {
	// PromptTemplate is a Go text/template rendered with the parent Result as
	// its data (fields: .Summary, .SessionID, .Turns).
	PromptTemplate string `json:"prompt_template"`
	// Criteria are acceptance criteria for this post-goal's stop-gate.
	Criteria []string `json:"criteria,omitempty"`
	// OnlyIfChanged is a glob pattern; the post-goal is skipped when no files
	// matching the pattern were modified by the parent goal. Avoids spawning a
	// CHANGELOG update when no user-visible code changed.
	OnlyIfChanged string `json:"only_if_changed,omitempty"`
}

// IsEmpty reports whether the contract carries no acceptance criteria at all.
// An empty contract means the stop-gate falls back to the verify-gate result
// only (no extra deterministic checks, no semantic judging).
func (c *GoalContract) IsEmpty() bool {
	if c == nil {
		return true
	}
	return len(c.DeterministicChecks) == 0 && len(c.SemanticCriteria) == 0 &&
		c.MaxFilesChanged == 0 && c.MaxLinesChanged == 0 &&
		len(c.PostCompletionGoals) == 0 && c.MinSubGoals == 0 &&
		!c.DisablePostGoals
}

// Marshal serializes the contract to a compact JSON string for persistence in
// the goal queue. An empty contract marshals to "".
func (c *GoalContract) Marshal() (string, error) {
	if c.IsEmpty() {
		return "", nil
	}
	b, err := json.Marshal(c)
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
	// Prompt is the goal's instruction text, used for scope heuristics that
	// auto-inject decomposition criteria for large goals (loop-005).
	Prompt string
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
	// NoTestCriterion disables the auto-injected "tests must be written"
	// deterministic check + semantic criterion (loop-002).
	NoTestCriterion bool
	// NoDocCriterion disables the auto-injected doc/changelog freshness
	// deterministic checks + semantic criteria (loop-006).
	NoDocCriterion bool
	// NoPostGoals disables auto-detected post-completion goals (loop-001).
	NoPostGoals bool
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
		for _, ck := range autoDetectChecks(opts.Workspace, opts) {
			if !hasCheckNamed(c.DeterministicChecks, ck.Name) {
				c.DeterministicChecks = append(c.DeterministicChecks, ck)
			}
		}
		// (2a) Always-on semantic criteria — these are what stop the loop
		// from ever leaving tests or docs behind without a human reminder.
		isGo := fileExists(filepath.Join(opts.Workspace, "go.mod"))
		if isGo && !opts.NoTestCriterion {
			c.SemanticCriteria = appendUnique(c.SemanticCriteria,
				"New or updated _test.go files exist for every changed package, "+
					"cover the happy path and at least one error case, "+
					"and pass under `go test -race ./...`.")
		}
		if !opts.NoDocCriterion {
			c.SemanticCriteria = appendUnique(c.SemanticCriteria,
				"If the change affects user-visible behaviour, CLI flags, environment "+
					"variables, or public APIs: README.md, AGENTS.md (if it exists), and "+
					"all affected doc.md files were updated to reflect the change.")
			if fileExists(filepath.Join(opts.Workspace, "CHANGELOG.md")) {
				c.SemanticCriteria = appendUnique(c.SemanticCriteria,
					"CHANGELOG.md has a new entry under [Unreleased] describing what "+
						"was changed and why.")
			}
		}
		// (2b) Scope heuristics for large goals: force decomposition (loop-005).
		for _, cr := range autoDetectScopeHints(opts.Prompt) {
			c.SemanticCriteria = appendUnique(c.SemanticCriteria, cr)
		}
		// (2c) Post-completion goals: docs/CHANGELOG/MASTER_TODO follow-ups
		// auto-spawned after the parent verifies (loop-001).
		if !opts.NoPostGoals {
			c.PostCompletionGoals = append(c.PostCompletionGoals, autoDetectPostGoals(opts.Workspace)...)
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
func autoDetectChecks(workspace string, opt ResolveOptions) []orchestrator.Check {
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
		// loop-002: fail the stop-gate when non-test Go code changed but no
		// tests were written or updated. Forces tests without a human prompt.
		if !opt.NoTestCriterion {
			checks = append(checks, orchestrator.Check{
				Kind:    orchestrator.CheckPredicate,
				Name:    "new-test-coverage",
				Cmd:     []string{"sh", "-c", newTestCoverageScript},
				Timeout: 1 * time.Minute,
			})
		}
	}
	// loop-006: doc/changelog freshness gates, regardless of language.
	if !opt.NoDocCriterion {
		if fileExists(filepath.Join(workspace, "CHANGELOG.md")) {
			checks = append(checks, orchestrator.Check{
				Kind:    orchestrator.CheckPredicate,
				Name:    "changelog-updated",
				Cmd:     []string{"sh", "-c", changelogUpdatedScript},
				Timeout: 1 * time.Minute,
			})
		}
		if anyDocMd(workspace) {
			checks = append(checks, orchestrator.Check{
				Kind:    orchestrator.CheckPredicate,
				Name:    "doc-md-freshness",
				Cmd:     []string{"sh", "-c", docMdFreshnessScript},
				Timeout: 1 * time.Minute,
			})
		}
	}
	return checks
}

// autoDetectPostGoals returns the standard post-completion follow-ups for the
// workspace: record the change in CHANGELOG.md, tick off MASTER_TODO.md items,
// and refresh affected doc.md files. Each is skipped at runtime when its
// OnlyIfChanged glob matches nothing the parent goal touched (loop-001).
func autoDetectPostGoals(workspace string) []PostGoal {
	var goals []PostGoal
	if fileExists(filepath.Join(workspace, "CHANGELOG.md")) {
		goals = append(goals, PostGoal{
			PromptTemplate: "Update CHANGELOG.md to record the following completed work:\n" +
				"{{ .Summary }}\n" +
				"Add it under the [Unreleased] section with today's date. Follow the " +
				"existing format exactly. Ensure the build and tests still pass.",
			Criteria:      []string{"CHANGELOG.md updated with the completed work"},
			OnlyIfChanged: "*.go",
		})
	}
	if fileExists(filepath.Join(workspace, "MASTER_TODO.md")) {
		goals = append(goals, PostGoal{
			PromptTemplate: "Review MASTER_TODO.md and check off (change \"- [ ]\" to \"- [x]\") " +
				"any items that were completed by this work:\n{{ .Summary }}\n" +
				"Do not add new items. Ensure the build and tests still pass.",
			Criteria: []string{"all relevant MASTER_TODO items checked off"},
		})
	}
	if dirExists(filepath.Join(workspace, "docs")) {
		goals = append(goals, PostGoal{
			PromptTemplate: "Review all doc.md files under docs/ and cmd/ that relate to the " +
				"following completed work and update them to reflect any API, flag, or " +
				"behavioural changes:\n{{ .Summary }}\n" +
				"Do not change unrelated docs. Ensure the build and tests still pass.",
			Criteria:      []string{"all affected doc.md files reflect the change"},
			OnlyIfChanged: "*.go",
		})
	}
	return goals
}

// largeScopeSignals are phrases that suggest a goal spans many independent
// units of work and should be decomposed via spawn_subgoal (loop-005).
var largeScopeSignals = []string{
	"all packages", "entire", "full coverage", "comprehensive",
	"refactor", "migrate", "all tests", "every package", "across the codebase",
}

// autoDetectScopeHints returns a decomposition semantic criterion when the
// prompt contains large-scope signals; otherwise nil.
func autoDetectScopeHints(prompt string) []string {
	low := strings.ToLower(prompt)
	for _, hint := range largeScopeSignals {
		if strings.Contains(low, hint) {
			return []string{
				"The goal was large in scope. If it required 3+ independent units, " +
					"it was decomposed into sub-goals via spawn_subgoal, and all " +
					"sub-goals are verified before the parent finalizes.",
			}
		}
	}
	return nil
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

// newTestCoverageScript (loop-002) fails when the diff introduces or modifies
// non-test Go files but adds/updates no _test.go file, or leaves a changed
// package with no test file at all. It names the offending packages so the
// stop-gate re-injection can point the model straight at them. Outside a git
// repo it exits 0 (cannot judge, never block).
const newTestCoverageScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
changed=$( { git diff --name-only HEAD 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u )
[ -z "$changed" ] && exit 0
non_test=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' || true)
[ -z "$non_test" ] && exit 0   # only test files changed: fine
pkgs=$(echo "$non_test" | xargs -I{} dirname {} 2>/dev/null | sort -u || true)
missing=""
for pkg in $pkgs; do
  if [ -z "$(find "$pkg" -maxdepth 1 -name '*_test.go' 2>/dev/null)" ]; then
    missing="$missing $pkg"
  fi
done
if [ -n "$missing" ]; then
  echo "MISSING TESTS: the following changed packages have no _test.go file:"
  for p in $missing; do echo "  - $p"; done
  exit 1
fi
test_changed=$(echo "$changed" | grep '_test\.go$' || true)
if [ -z "$test_changed" ]; then
  echo "Non-test Go files were changed but no test file was added or modified."
  echo "Changed packages must have new or updated tests."
  exit 1
fi
exit 0`

// changelogUpdatedScript (loop-006) requires CHANGELOG.md to be touched when
// production (non-test) Go files were changed.
const changelogUpdatedScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
if [ ! -f CHANGELOG.md ]; then exit 0; fi
changed=$( { git diff --name-only HEAD 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u )
go_changed=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' || true)
[ -z "$go_changed" ] && exit 0
cl_changed=$(echo "$changed" | grep -i 'CHANGELOG' || true)
if [ -z "$cl_changed" ]; then
  echo "CHANGELOG.md was not updated. Production Go files were changed."
  echo "Add a bullet under [Unreleased] describing what changed."
  exit 1
fi
exit 0`

// docMdFreshnessScript (loop-006) flags package doc.md files that went stale:
// when a package's .go files changed but its sibling doc.md was not updated.
const docMdFreshnessScript = `set -e
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
changed=$( { git diff --name-only HEAD 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null; } | sort -u )
changed_dirs=$(echo "$changed" | grep '\.go$' | grep -v '_test\.go$' \
  | xargs -I{} dirname {} 2>/dev/null | sort -u || true)
[ -z "$changed_dirs" ] && exit 0
fail=0
for dir in $changed_dirs; do
  for doc in "$dir"/*.doc.md; do
    [ -e "$doc" ] || continue
    if ! echo "$changed" | grep -Fq "$doc"; then
      echo "STALE DOC: $doc was not updated despite changes in $dir"
      fail=1
    fi
  done
done
[ $fail -eq 1 ] && exit 1
exit 0`

// appendUnique appends s to list only when not already present.
func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func hasCheckNamed(checks []orchestrator.Check, name string) bool {
	for _, c := range checks {
		if c.Name == name {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// anyDocMd reports whether the workspace contains at least one *.doc.md file
// (bounded walk that skips vendored/VCS dirs and stops at the first match).
func anyDocMd(workspace string) bool {
	found := false
	_ = filepath.WalkDir(workspace, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "vendor", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), ".doc.md") {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
