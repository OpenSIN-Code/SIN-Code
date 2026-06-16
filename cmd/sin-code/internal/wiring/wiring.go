// SPDX-License-Identifier: MIT
// Purpose: builder for the full learning + dispatch + PRP pipeline.
// One call constructs everything that needs cross-package wiring
// against the real SIN-Code subsystems.
// Docs: wiring.doc.md
package wiring

import (
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/adapters"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dispatch"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/evalharness"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooklife"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/learning"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/prp"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// Deps is the full set of optional subsystems the wiring layer
// can plug into. Any field may be nil — wiring degrades to no-op
// for that subsystem.
type Deps struct {
	Workdir   string
	LLM       *llm.Client
	LLMModel  string
	Memory    *memory.Store
	Verify    *verify.Gate
	Prompts   dispatch.PromptSink
	Subagents dispatch.SubagentRunner
}

// Bundle is the fully-wired graph. The caller (chat command,
// daemon) holds one Bundle and threads its parts where needed.
type Bundle struct {
	Learner  *learning.Learner
	Dispatch *dispatch.Dispatcher
	Eval     func(name string) (evalharness.Subject, evalharness.Scorer, error)
	PRP      prp.Deps
}

// Build constructs the full Bundle.
func Build(d Deps) (*Bundle, error) {
	l, err := learning.New(learning.Options{
		Workdir:    d.Workdir,
		LLM:        d.LLM,
		Model:      d.LLMModel,
		Memory:     d.Memory,
		VerifyGate: d.Verify,
	})
	if err != nil {
		return nil, err
	}

	disp, err := BuildDispatcher("./skills/imported", d.Prompts, d.Subagents)
	if err != nil {
		// Soft-fail: dispatcher can be nil if no skill dir exists yet.
		disp = &dispatch.Dispatcher{Prompts: d.Prompts, Agents: d.Subagents}
	}

	var v hooklife.Verifier
	if d.Verify != nil {
		v = adapters.VerifyGate{Gate: d.Verify}
	}
	evalFactory := EvalSubjectFactory(v)
	prpDeps := PRPDeps(v, nil, nil, nil) // Planner/Implementer/PR are host-supplied

	return &Bundle{
		Learner:  l,
		Dispatch: disp,
		Eval:     evalFactory,
		PRP:      prpDeps,
	}, nil
}
