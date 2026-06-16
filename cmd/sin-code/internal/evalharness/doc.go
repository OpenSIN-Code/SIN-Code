// Package evalharness implements eval-driven development for SIN-Code, a Go
// port of ECC's eval-harness skill.
//
// You define an EvalSet (named cases with prompts and optional expectations),
// run it against a Subject (an agent run, a verify gate, a model call) using a
// Scorer (exact/contains/success/LLM-judge/composite), and persist each Run.
// Compare diffs two runs case-by-case to surface improvements and regressions,
// which makes it a CI gate: `sin eval compare --fail-on-regress`.
//
// Storage is JSON on disk under
// $SIN_EVAL_DIR | $XDG_DATA_HOME/sin-code-eval | ~/.local/share/sin-code-eval.
//
// SPDX-License-Identifier: MIT
package evalharness
