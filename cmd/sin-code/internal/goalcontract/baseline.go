// SPDX-License-Identifier: MIT
// Purpose: SinCode Loop System — the always-on Definition-of-Done baseline.
// This encodes the "self-evident" work that EVERY goal implies but a human
// should never have to spell out again: write/extend tests, leave no debug
// scaffolding, finish the work completely, and keep the docs (README,
// CHANGELOG, MASTER_TODO/backlog, AGENTS.md, per-package .doc.md CoDocs) in
// sync. The baseline is merged additively into every resolved GoalContract so
// the stop-gate enforces it independently of the worker.
//
// Design: HYBRID, matching the stop-gate. Mechanical, fail-closed predicate
// checks catch the cheap-to-verify omissions (a code change with no test
// change, an untouched CHANGELOG, a package missing its CoDoc); the richer,
// judgement-heavy expectations ("tests actually exercise the new behavior",
// "docs are meaningfully updated") are semantic criteria for the LLM judge.
// Every predicate is best-effort and FAIL-OPEN outside a git repo so the
// baseline can never trap a non-git workspace.
//
// Docs: baseline.doc.md
package goalcontract

import (
	"os"
	"strings"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// baselineEnvVar toggles the whole baseline off when set to a falsey value
// (off/0/false/no). Anything else (or unset) leaves it ON — the loop system is
// on by default, by design.
const baselineEnvVar = "SIN_BASELINE"

// BaselineEnabled reports whether the always-on Definition-of-Done baseline
// should be injected. It is ON unless the caller passes disable=true (e.g. a
// --no-baseline flag) OR the SIN_BASELINE env var is set to a falsey value.
// This is the single source of truth shared by every loop entrypoint
// (daemon, auto, swarm, serve, tui) so behavior is identical everywhere.
func BaselineEnabled(disable bool) bool {
	if disable {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(baselineEnvVar))) {
	case "off", "0", "false", "no", "disable", "disabled":
		return false
	default:
		return true
	}
}

// BaselineSemanticCriteria are the non-mechanical acceptance criteria that the
// stop-gate's LLM judge confirms for every goal. They are deliberately phrased
// as completion requirements so the judge rejects half-finished work.
//
// Exported so the worker prompt preamble (loopbuilder) can quote the exact
// criteria the agent will be held to — telling the agent the rubric up front
// is what makes it do the work proactively instead of being told to.
func BaselineSemanticCriteria() []string {
	return []string{
		"Automated tests cover the new or changed behavior and actually exercise the new code paths (not just compile); they would fail if the change were reverted.",
		"The full test suite passes and the project builds cleanly; no test was skipped, weakened, or deleted just to make the gate green.",
		"No debugging scaffolding is left behind: no stray debug prints/logs, commented-out code, dead code, or temporary files introduced by this work.",
		"The goal is fully and completely addressed end-to-end — no partial implementation, stubs, placeholders, or TODO/FIXME markers left for later.",
		"Documentation is updated to reflect the change wherever it applies: README.md (features/usage/CLI), CHANGELOG.md (an entry under Unreleased), MASTER_TODO.md / backlog (items checked off or follow-ups added), AGENTS.md (when mandates, architecture, or tooling change), and the per-package .doc.md CoDoc for every package whose behavior changed.",
		"Any necessary related work the change implies is also done (callers updated, configuration/flags wired, error handling and edge cases covered) so nothing is left in a broken or inconsistent state.",
	}
}

// BaselineChecks returns the mechanical, fail-closed predicate checks for the
// baseline. They are scoped to Go repositories (a go.mod must exist) and each
// one exits 0 (pass) when it cannot judge — outside git, or when the diff is
// empty, or when the change is docs-only — so the baseline never blocks work
// it has no business blocking. The check NAMES are stable so Resolve can
// dedupe them against any explicitly-configured contract.
func BaselineChecks(workspace string) []orchestrator.Check {
	if !fileExists(joinWorkspace(workspace, "go.mod")) {
		return nil
	}
	return []orchestrator.Check{
		{
			Kind:    orchestrator.CheckPredicate,
			Name:    "baseline-tests-changed-with-code",
			Cmd:     []string{"sh", "-c", baselineTestsScript},
			Timeout: 1 * time.Minute,
		},
		{
			Kind:    orchestrator.CheckPredicate,
			Name:    "baseline-changelog-updated",
			Cmd:     []string{"sh", "-c", baselineChangelogScript},
			Timeout: 1 * time.Minute,
		},
		{
			Kind:    orchestrator.CheckPredicate,
			Name:    "baseline-codoc-present",
			Cmd:     []string{"sh", "-c", baselineCoDocScript},
			Timeout: 1 * time.Minute,
		},
	}
}

// Baseline returns the full always-on contract: semantic criteria plus the
// fail-open mechanical predicates. It carries no GoalID; Resolve merges it into
// the per-goal contract.
func Baseline(workspace string) *GoalContract {
	return &GoalContract{
		DeterministicChecks: BaselineChecks(workspace),
		SemanticCriteria:    BaselineSemanticCriteria(),
	}
}

// Preamble renders a contract's semantic criteria into a Definition-of-Done
// briefing for the worker prompt. It returns "" when the contract has no
// semantic criteria so non-contract loops are completely unaffected.
func Preamble(c GoalContract) string {
	if len(c.SemanticCriteria) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("DEFINITION OF DONE (SinCode Loop System) — you are NOT done until ALL of these hold. ")
	b.WriteString("An independent stop-gate will verify them and send you back to work if any are unmet, so address them proactively as part of this goal:\n")
	for _, cr := range c.SemanticCriteria {
		if s := strings.TrimSpace(cr); s != "" {
			b.WriteString("- ")
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// joinWorkspace is a tiny helper so this file does not need path/filepath just
// for one join; it mirrors filepath.Join semantics for the common case.
func joinWorkspace(workspace, name string) string {
	if workspace == "" {
		return name
	}
	if strings.HasSuffix(workspace, "/") {
		return workspace + name
	}
	return workspace + "/" + name
}

// changedCodeFiles lists non-test, non-vendored .go files in the working-tree
// diff against HEAD. Shared shell prelude used by the predicates below.
const changedCodeFilesPrelude = `
if ! git rev-parse --git-dir >/dev/null 2>&1; then exit 0; fi
changed=$(git diff HEAD --name-only 2>/dev/null; git diff --cached --name-only 2>/dev/null; git ls-files --others --exclude-standard 2>/dev/null)
changed=$(printf '%s\n' "$changed" | sort -u | sed '/^$/d')
code=$(printf '%s\n' "$changed" | grep -E '\.go$' | grep -vE '(_test\.go$|/vendor/|^vendor/)' || true)
`

// baselineTestsScript fails when production .go files changed but NO _test.go
// file changed alongside them — the cheap, mechanical half of "new behavior is
// tested". Fail-open: empty diff or no code change → pass.
const baselineTestsScript = `set -e` + changedCodeFilesPrelude + `
if [ -z "$code" ]; then exit 0; fi
tests=$(printf '%s\n' "$changed" | grep -E '_test\.go$' || true)
if [ -z "$tests" ]; then
  echo "code changed but no *_test.go was added or modified:"; echo "$code"
  echo "Add or extend tests that exercise the new behavior (SinCode loop baseline)."
  exit 1
fi
exit 0`

// baselineChangelogScript fails when production .go files changed but
// CHANGELOG.md was not touched. Fail-open: no code change → pass; no CHANGELOG
// file in the repo → pass (cannot require what does not exist).
const baselineChangelogScript = `set -e` + changedCodeFilesPrelude + `
if [ -z "$code" ]; then exit 0; fi
if [ ! -f CHANGELOG.md ]; then exit 0; fi
if printf '%s\n' "$changed" | grep -qx 'CHANGELOG.md'; then exit 0; fi
echo "code changed but CHANGELOG.md was not updated."
echo "Add an entry under the Unreleased section (SinCode loop baseline)."
exit 1`

// baselineCoDocScript fails when a package directory gains a production .go
// change but contains no *.doc.md CoDoc at all. It only requires that the
// CoDoc EXISTS (mechanical); whether its CONTENT was meaningfully updated is
// left to the semantic judge. Fail-open as usual.
const baselineCoDocScript = `set -e` + changedCodeFilesPrelude + `
if [ -z "$code" ]; then exit 0; fi
missing=""
for f in $code; do
  dir=$(dirname "$f")
  if ! ls "$dir"/*.doc.md >/dev/null 2>&1; then
    missing="$missing $dir"
  fi
done
missing=$(printf '%s\n' $missing | sort -u | sed '/^$/d')
if [ -n "$missing" ]; then
  echo "package(s) changed without a .doc.md CoDoc:"; echo "$missing"
  echo "Add a <package>.doc.md describing the package (SinCode loop baseline)."
  exit 1
fi
exit 0`
