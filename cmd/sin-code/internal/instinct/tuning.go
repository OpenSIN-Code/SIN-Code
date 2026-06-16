// SPDX-License-Identifier: MIT
// Purpose: package-wide, overridable tuning values. Set once at startup via
// ApplyConfig and read lock-free on every Reinforce/Contradict. Defaults
// match the original hardcoded constants, so behavior is unchanged until a
// Config is applied.
// Docs: tuning.doc.md
package instinct

import "sync/atomic"

// tuning holds the live, configurable math parameters.
var tuning atomic.Value // stores tuningParams

type tuningParams struct {
	activation     float64
	evolve         float64
	reinforceStep  float64
	contradictStep float64
}

func init() {
	tuning.Store(tuningParams{
		activation:     ActivationThreshold,
		evolve:         EvolveThreshold,
		reinforceStep:  0.25,
		contradictStep: 0.40,
	})
}

// ApplyConfig threads a Config into the package-level math. Call once at
// startup (e.g. from NewManager) before any instinct mutation.
func ApplyConfig(c Config) {
	tuning.Store(tuningParams{
		activation:     c.ActivationThreshold,
		evolve:         c.EvolveThreshold,
		reinforceStep:  c.ReinforceStep,
		contradictStep: c.ContradictStep,
	})
}

func currentTuning() tuningParams { return tuning.Load().(tuningParams) }
