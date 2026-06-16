// SPDX-License-Identifier: MIT
// Purpose: export all new learning + dispatch + prp + evalharness +
// assets + instinct + hooklife subcommands as package-level
// `var XxxCmd = ...` references, mirroring the existing
// `var DiscoverCmd`, `var MemoryCmd`, ... pattern in this package.
// Docs: learning_cmds.doc.md
package internal

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/assets"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/instinct"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/prp"
)

// InstinctCmd is `sin instinct ...` — continuous-learning CLI.
var InstinctCmd = instinct.NewCommand()

// HooksCmd is `sin hooks ...` — inspect/test lifecycle hooks.
var HooksCmd = hooklife.NewCommand(hooklife.NewRegistry())

// AssetsCmd is `sin assets ...` — harvested agents/commands/skills.
var AssetsCmd = assets.NewCommand("./skills/imported")

// EvalCmd is `sin evalset ...` — eval-driven development.
// Default subject is a no-op that reports "no subject wired"; the
// wiring layer re-roots this with the real verify engine.
var EvalCmd = evalharness.NewCommand(defaultEvalFactory)

// PRPCmd is `sin prp ...` — Product Requirement Prompt workflow.
var PRPCmd = prp.NewCommand(prp.Deps{})

// defaultEvalFactory returns a no-op subject + scorer for the
// case where the wiring layer has not supplied a real one. Trying
// to run an eval set without a subject surfaces a clear error to
// the user instead of panicking.
func defaultEvalFactory(name string) (evalharness.Subject, evalharness.Scorer, error) {
	return noopSubject{}, evalharness.SuccessFlag{}, nil
}

type noopSubject struct{}

func (noopSubject) Run(_ context.Context, _ evalharness.EvalCase) (evalharness.Output, error) {
	return evalharness.Output{
		Text:    "no subject wired: run via the chat command or wire a real subject through internal/wiring.Build",
		Success: false,
	}, nil
}
