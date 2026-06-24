// SPDX-License-Identifier: MIT
// Purpose: built-in Arm constructors for the three-arm (well: four-arm)
// eval harness (issue #171). The IDs are reserved:
//
//	__baseline__    — no system prompt. The "control" for delta-vs-no-ask.
//	__terse__       — "Answer concisely." The honest control for delta-vs-skill.
//	__lazy_skill__  — system prompt rendered by skill-code-lazy (issue #178).
//	__user_skill__  — the skill named in the --skill CLI flag.
//	<verbosity>     — one of `ultra`, `terse`, `normal`, `verbose`, `default`.
//
// All non-trivial arms are appended to the standard terse prefix so
// the per-arm delta isolates the *skill's own contribution* from
// the generic "be terse" effect (caveman evals/README.md §3).
//
// Docs: arms.doc.md
package evalharness

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TersePrefix is the canonical "be terse" instruction prepended to
// every skill / verbosity arm. Exported so test fixtures can
// assert exact byte content of the rendered system prompt.
const TersePrefix = "Answer concisely."

// StandardTerseArm returns the static terse instruction.
func StandardTerseArm() Arm {
	return Arm{ID: "__terse__", SystemPrompt: TersePrefix, PricingName: "stub"}
}

// NoSystemPromptArm is the legacy single-arm baseline. Empty
// system prompt so the LLM sees only the user turn.
func NoSystemPromptArm() Arm {
	return Arm{ID: "__baseline__", SystemPrompt: "", PricingName: "stub"}
}

// LazySkillName is the canonical name for the lazy-skill arm. Issue
// #178 introduces skill-code-lazy; we keep the ID stable at
// "__lazy_skill__" so the snapshot schema is forward-compatible.
const LazySkillName = "skill-code-lazy"

// LazySkillArm returns the arm whose SystemPrompt is the rendered
// skill-code-lazy body. When the bundled skill is unavailable
// (issue #178 not yet landed), the comparator emits a clear,
// deterministic placeholder so the row stays byte-stable.
func LazySkillArm(skillReader func() (string, error)) Arm {
	body, err := safeReadSkill(skillReader)
	return Arm{
		ID:           "__lazy_skill__",
		SkillName:    LazySkillName,
		SystemPrompt: renderSkillPrompt(body, err),
		PricingName:  "stub",
	}
}

// SkillArm wraps a skill body with the terse prefix. skillReader
// returns the SKILL.md body. When the body is empty or unreadable,
// we fall back to a single-line placeholder so the snapshot diffs
// cleanly. Exported because the comparator and the CLI both build
// skill arms through this helper to keep the prefix logic in one
// place.
func SkillArm(name string, skillReader func() (string, error)) Arm {
	if strings.TrimSpace(name) == "" {
		return Arm{ID: "__user_skill__", SystemPrompt: TersePrefix, PricingName: "stub",
			Setup: func(EvalCase) error { return errors.New("skill arm: empty name") }}
	}
	body, err := safeReadSkill(skillReader)
	return Arm{
		ID:           name,
		SkillName:    name,
		SystemPrompt: renderSkillPrompt(body, err),
		PricingName:  "stub",
	}
}

// VerbosityArm constructs an arm whose SystemPrompt is the bytes
// rendered by the verbosity-level mode. The mode names mirror the
// config schema in AGENTS.md §7 (`default`, `verbose`, `normal`,
// `terse`, `ultra`). For this issue #171 we keep the renderer
// dependency-free: "default" and "verbose" are no-ops, the rest
// are the caveman-described cave-speak ruleset minus pleasantries.
// The reader callback is provided by the wiring layer — default
// nil → empty (the comparator treats empty as "no extra prompting").
func VerbosityArm(level string, reader func() (string, error)) Arm {
	if level == "" {
		level = "default"
	}
	var body string
	if reader != nil {
		if b, err := reader(); err == nil {
			body = b
		}
	}
	sp := body
	if sp != "" && level != "__terse__" {
		sp = TersePrefix + "\n\n" + sp
	}
	return Arm{
		ID:           level,
		Verbosity:    level,
		SystemPrompt: sp,
		PricingName:  "stub",
	}
}

// FusionArm returns a copy of base with FusionEnabled set to true.
// The caller is expected to supply a unique ID when stacking onto
// an existing arm (e.g. a terse-derived fusion arm) so TotalsByArm
// keys don't collide with the non-fusion control.
func FusionArm(base Arm) Arm {
	base.FusionEnabled = true
	return base
}

// DefaultArms is the canonical 4-arm harness referenced by the CLI
// flag `--arm baseline,terse,lazy_skill,<user>`. The fourth slot is
// filled in by the CLI from the user-supplied --skill (or skill-code-create
// when none is given).
func DefaultArms(skillName string) []Arm {
	return []Arm{
		NoSystemPromptArm(),
		StandardTerseArm(),
		LazySkillArm(func() (string, error) {
			return readBundledSkillBody(LazySkillName)
		}),
		SkillArm(skillName, func() (string, error) {
			if skillName == "" {
				return "", nil
			}
			return readBundledSkillBody(skillName)
		}),
	}
}

// safeReadSkill swallows a nil reader and surfaces its error to the
// caller with a useful prefix when applicable.
func safeReadSkill(reader func() (string, error)) (string, error) {
	if reader == nil {
		return "", nil
	}
	return reader()
}

// renderSkillPrompt produces the arm's SystemPrompt field. When the
// skill body loads, we prepend the terse prefix. When it doesn't,
// we keep the terse prefix anyway so the arm still drives the LLM —
// better an honest empty than a panic that hides the harness
// failure. Used by SkillArm and LazySkillArm in lockstep.
func renderSkillPrompt(body string, err error) string {
	if err != nil || strings.TrimSpace(body) == "" {
		if err != nil {
			return TersePrefix + "\n\n[skill unavailable: " + err.Error() + "]"
		}
		return TersePrefix + "\n\n[skill unavailable: not on disk]"
	}
	return TersePrefix + "\n\n" + strings.TrimSpace(body)
}

// readBundledSkillBody looks up the skill's SKILL.md on the local
// filesystem. The comparator MUST NOT take a hard dependency on
// the embedded `skills` package (cmd/sin-code/skills/) — that would
// couple eval-harness to the agent tool registry and re-import the
// entire embedded blob. We instead walk the canonical layout:
//
//	$SIN_SKILLS_DIR/<category>-skills/<skill>/SKILL.md
//
// with the repo root or /usr/local/share/sin-code/skills as
// fallbacks. The helper is best-effort: an empty string + nil error
// means "not on disk" so the caller can render the placeholder
// arm without panicking.
//
// Exported as ReadBundledSkillBody so the CLI layer can build
// arms without re-implementing the search path.
func readBundledSkillBody(skillName string) (string, error) {
	if skillName == "" {
		return "", nil
	}
	for _, root := range bundledSkillsRoots() {
		loc, err := locateSKILL(root, skillName)
		if err != nil || loc == "" {
			continue
		}
		data, err := os.ReadFile(loc) // #nosec G304 — read-only SKILL.md from skill dir
		if err != nil {
			return "", fmt.Errorf("skill body: %s: %w", loc, err)
		}
		return string(data), nil
	}
	return "", nil
}

// sin-debt: shrink, upgrade: inline when callers are consolidated or test seam is removed

// ReadBundledSkillBody is the exported alias for readBundledSkillBody.
// Public so the CLI (eval_cmd.go) can layer its own arm defaults
// on top of the comparator's discovery path.
func ReadBundledSkillBody(skillName string) (string, error) {
	return readBundledSkillBody(skillName)
}

// bundledSkillsRoots returns the on-disk roots searched for a SKILL.md.
// Order matters — earlier entries win. We accept every standard SIN
// skill location so the harness works in fresh CI sandboxes as well
// as dev workstations.
func bundledSkillsRoots() []string {
	var roots []string
	if d := os.Getenv("SIN_SKILLS_DIR"); d != "" {
		roots = append(roots, d)
	}
	// Repo-root skills/ (relative to cwd) is the most common layout
	// in development. We resolve to an absolute path.
	if cwd, err := os.Getwd(); err == nil {
		roots = append(roots, filepath.Join(cwd, "skills"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		roots = append(roots, filepath.Join(home, ".local", "share", "sin-code", "skills"))
	}
	return roots
}

// locateSKILL scans root for "<any>-skills/<name>/SKILL.md". The
// category directory is opaque on purpose — naming rules forbid
// putting a skill at the root of skills/. Returns the absolute path
// of the first match, or "" when nothing matched. Errors only when
// the search was structurally impossible (e.g. root is not a
// directory); missing files are silent.
func locateSKILL(root, skillName string) (string, error) {
	if root == "" || skillName == "" {
		return "", nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if !strings.HasSuffix(e.Name(), "-skills") {
			continue
		}
		candidate := filepath.Join(root, e.Name(), skillName, "SKILL.md")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", nil
}
