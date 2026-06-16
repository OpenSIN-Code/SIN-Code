// Package prp implements the Product Requirement Prompt workflow, a Go port of
// ECC's prp-plan / prp-implement / prp-pr commands.
//
// A PRP is the plan-of-record for a change: a goal, context, decomposed tasks,
// and acceptance criteria, stored as reviewable Markdown under .sin/prp/. The
// Engine drives it through phases — draft -> planned -> implementing ->
// verifying -> ready -> shipped — persisting after every step so a run is
// interruptible and resumable.
//
// The Engine delegates the hard work to four collaborators wired by the host:
// Planner (decompose goal into tasks), Implementer (execute a task), Verifier
// (run the quality gate, reusing internal/verify), and PRController (open the
// pull request). Verification failure kicks the PRP back to implementing.
//
// SPDX-License-Identifier: MIT
package prp
