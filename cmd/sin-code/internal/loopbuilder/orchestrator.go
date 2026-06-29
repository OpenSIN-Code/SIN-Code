// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: consolidate when loopbuilder is refactored
// Purpose: standalone orchestrator wiring (DeepPlanner, PreWarm, PatternDB)
// extracted from builder.go to keep each file ≤500 lines.
package loopbuilder

import (
	"context"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
)

// OrchestratorDeps holds the standalone orchestrator components wired by
// WireOrchestrator. Callers use these to build a Dispatcher with
// pre-warming, feed patterns to the DeepPlanner, and record completed
// plans into the PatternDB.
type OrchestratorDeps struct {
	DeepPlanner *orchestrator.DeepPlanner
	PreWarm     *orchestrator.PreWarmManager
	PatternDB   *orchestrator.PatternDB
}

// WireOrchestrator builds and wires the standalone orchestrator components
// (DeepPlanner, PreWarmManager, PatternDB) based on the Config flags. Returns
// an OrchestratorDeps struct; fields are nil when the corresponding feature
// is not enabled (backward compat: everything is opt-in).
//
// When DeepPlannerEnabled is true, a DeepPlanner replaces the legacy linear
// Planner. When PatternLearningEnabled is true, a PatternDB is created and
// injected into the DeepPlanner via SetPatternDB so learned patterns refine
// probability scores. When PreWarmEnabled is true, a PreWarmManager is
// created for use with a Dispatcher.
func WireOrchestrator(cfg Config, registry *orchestrator.Registry) *OrchestratorDeps {
	deps := &OrchestratorDeps{}
	if !cfg.DeepPlannerEnabled {
		return deps
	}
	agents := orchestrator.DefaultAgents()
	if registry != nil {
		agents = registry.List()
	}
	deps.DeepPlanner = orchestrator.NewDeepPlanner(agents)

	if cfg.PatternLearningEnabled {
		deps.PatternDB, _ = orchestrator.NewPatternDB(nil)
		deps.DeepPlanner.SetPatternDB(deps.PatternDB)
	}

	if cfg.PreWarmEnabled && registry != nil {
		deps.PreWarm = orchestrator.NewPreWarmManager(registry, 0, 0)
	}

	return deps
}

// RecordPlanCompletion records a completed plan into the PatternDB if
// pattern learning is enabled. This should be called after a plan's
// dispatch completes (success or failure). No-op when deps.PatternDB
// is nil (pattern learning disabled).
func (deps *OrchestratorDeps) RecordPlanCompletion(ctx context.Context, plan *orchestrator.Plan) {
	if deps == nil || deps.PatternDB == nil || plan == nil {
		return
	}
	_ = deps.PatternDB.RecordSequence(ctx, plan)
}
