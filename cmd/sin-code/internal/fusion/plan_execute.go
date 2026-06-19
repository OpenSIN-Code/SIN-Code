// SPDX-License-Identifier: MIT
// Purpose: SIN Fusion — Multi-model plan+execute tournament (issue #321).
//
// Unlike the verify-fail tournament (issue #290, reactive), this is
// PROACTIVE: multiple models plan in parallel, an arbiter picks the best
// plan, then multiple models execute in parallel and the arbiter picks
// the best result. This mirrors ECC's /multi-plan and /multi-execute
// commands.
//
// Race-free (mandate M7): sync.WaitGroup + context.WithCancel +
// channels for collection.
package fusion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ProviderPool wraps a slice of ProviderConfig and provides filtered
// access by model name. It is the shared pool type used by both the
// verify-tournament and the plan+execute tournament.
type ProviderPool struct {
	Providers []ProviderConfig
}

// NewProviderPool creates a pool from the given provider configs.
func NewProviderPool(providers []ProviderConfig) *ProviderPool {
	return &ProviderPool{Providers: append([]ProviderConfig(nil), providers...)}
}

// Get returns the providers whose Name matches one of the given model
// names. If modelNames is empty, all providers are returned.
func (p *ProviderPool) Get(modelNames []string) []ProviderConfig {
	if p == nil {
		return nil
	}
	if len(modelNames) == 0 {
		return append([]ProviderConfig(nil), p.Providers...)
	}
	filter := make(map[string]bool, len(modelNames))
	for _, n := range modelNames {
		filter[strings.TrimSpace(n)] = true
	}
	var out []ProviderConfig
	for _, prov := range p.Providers {
		if filter[prov.Name] {
			out = append(out, prov)
		}
	}
	return out
}

// PlanCandidate is one model's plan output.
type PlanCandidate struct {
	Model string
	Plan  string
}

// ResultCandidate is one model's execution output.
type ResultCandidate struct {
	Model    string
	Output   string
	Verified bool
}

// BestPlan is the arbiter's chosen plan.
type BestPlan struct {
	Plan      string  `json:"plan"`
	Model     string  `json:"model"`
	Score     float64 `json:"score"`
	Rationale string  `json:"rationale"`
}

// BestResult is the arbiter's chosen execution result.
type BestResult struct {
	Output   string  `json:"output"`
	Model    string  `json:"model"`
	Score    float64 `json:"score"`
	Verified bool    `json:"verified"`
}

// Arbiter selects the best plan from candidates and the best result from
// candidates. Implementations may use heuristics (longest plan, verified
// result) or an LLM judge.
type Arbiter interface {
	PickPlan(plans []PlanCandidate) (*BestPlan, error)
	PickResult(results []ResultCandidate) (*BestResult, error)
}

// SimpleArbiter picks the longest (most-detailed) plan and the verified
// result with the longest output. If no result is verified, it picks the
// longest output.
type SimpleArbiter struct{}

// PickPlan selects the plan with the most content (by byte length). Ties
// are broken alphabetically by model name for determinism.
func (a *SimpleArbiter) PickPlan(plans []PlanCandidate) (*BestPlan, error) {
	if len(plans) == 0 {
		return nil, errors.New("fusion: no plan candidates")
	}
	sorted := make([]PlanCandidate, len(plans))
	copy(sorted, plans)
	sort.SliceStable(sorted, func(i, j int) bool {
		if len(sorted[i].Plan) != len(sorted[j].Plan) {
			return len(sorted[i].Plan) > len(sorted[j].Plan)
		}
		return sorted[i].Model < sorted[j].Model
	})
	winner := sorted[0]
	score := float64(len(winner.Plan))
	return &BestPlan{
		Plan:      winner.Plan,
		Model:     winner.Model,
		Score:     score,
		Rationale: fmt.Sprintf("longest plan (%d bytes) from %s", len(winner.Plan), winner.Model),
	}, nil
}

// PickResult selects the first verified result (longest output among
// verified). If none verified, selects the longest output overall.
func (a *SimpleArbiter) PickResult(results []ResultCandidate) (*BestResult, error) {
	if len(results) == 0 {
		return nil, errors.New("fusion: no result candidates")
	}
	sorted := make([]ResultCandidate, len(results))
	copy(sorted, results)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Verified != sorted[j].Verified {
			return sorted[i].Verified
		}
		if len(sorted[i].Output) != len(sorted[j].Output) {
			return len(sorted[i].Output) > len(sorted[j].Output)
		}
		return sorted[i].Model < sorted[j].Model
	})
	winner := sorted[0]
	score := float64(len(winner.Output))
	if winner.Verified {
		score += 1000
	}
	return &BestResult{
		Output:   winner.Output,
		Model:    winner.Model,
		Score:    score,
		Verified: winner.Verified,
	}, nil
}

// PlanFunc is the injected plan-generation function. The tournament calls
// it once per model in a goroutine.
type PlanFunc func(ctx context.Context, prov ProviderConfig, prompt string) (string, error)

// ExecuteFunc is the injected execution function. The tournament calls it
// once per model in a goroutine with the chosen plan.
type ExecuteFunc func(ctx context.Context, prov ProviderConfig, plan *BestPlan) (string, error)

// VerifyFunc checks whether an execution output is correct. Returns true
// if the output passes verification. Optional — nil means all outputs are
// treated as unverified (the arbiter then picks by output length).
type VerifyFunc func(ctx context.Context, output string) bool

// PlanExecuteTournament orchestrates a proactive multi-model plan and
// execute cycle. Multiple models plan in parallel; the arbiter picks the
// best plan. Then multiple models execute that plan in parallel; the
// arbiter picks the best result.
type PlanExecuteTournament struct {
	Pool               *ProviderPool
	PlanFunc           PlanFunc
	ExecuteFunc        ExecuteFunc
	VerifyFunc         VerifyFunc
	Arbiter            Arbiter
	MaxCostUSD         float64
	PerProviderTimeout time.Duration

	mu      sync.Mutex
	costUSD float64
}

// NewPlanExecuteTournament creates a tournament with the given provider
// pool. The caller must set PlanFunc, ExecuteFunc, and Arbiter before
// calling Plan or Execute.
func NewPlanExecuteTournament(pool *ProviderPool) *PlanExecuteTournament {
	return &PlanExecuteTournament{
		Pool:    pool,
		Arbiter: &SimpleArbiter{},
	}
}

// Plan fans out the prompt to the given models in parallel, collects their
// plans, and asks the arbiter to pick the best one.
func (t *PlanExecuteTournament) Plan(ctx context.Context, prompt string, models []string) (*BestPlan, error) {
	if t == nil || t.Pool == nil {
		return nil, errors.New("fusion: no provider pool")
	}
	if t.PlanFunc == nil {
		return nil, errors.New("fusion: PlanFunc not wired")
	}
	providers := t.Pool.Get(models)
	if len(providers) == 0 {
		return nil, errors.New("fusion: no providers matched model filter")
	}
	arbiter := t.Arbiter
	if arbiter == nil {
		arbiter = &SimpleArbiter{}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type planResult struct {
		candidate PlanCandidate
		err       error
	}
	ch := make(chan planResult, len(providers))
	var wg sync.WaitGroup

	for _, prov := range providers {
		wg.Add(1)
		go func(prov ProviderConfig) {
			defer wg.Done()
			provCtx, provCancel := context.WithTimeout(ctx, t.perProviderTimeout())
			defer provCancel()
			plan, err := t.PlanFunc(provCtx, prov, prompt)
			if err != nil {
				ch <- planResult{err: err}
				return
			}
			ch <- planResult{candidate: PlanCandidate{Model: prov.Name, Plan: plan}}
		}(prov)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var candidates []PlanCandidate
	for pr := range ch {
		if pr.err != nil {
			continue
		}
		candidates = append(candidates, pr.candidate)
	}

	if len(candidates) == 0 {
		return nil, errors.New("fusion: all plan providers failed")
	}
	return arbiter.PickPlan(candidates)
}

// Execute fans out the chosen plan to the given models in parallel,
// verifies each output, and asks the arbiter to pick the best result.
func (t *PlanExecuteTournament) Execute(ctx context.Context, plan *BestPlan, models []string) (*BestResult, error) {
	if t == nil || t.Pool == nil {
		return nil, errors.New("fusion: no provider pool")
	}
	if t.ExecuteFunc == nil {
		return nil, errors.New("fusion: ExecuteFunc not wired")
	}
	if plan == nil {
		return nil, errors.New("fusion: nil plan")
	}
	providers := t.Pool.Get(models)
	if len(providers) == 0 {
		return nil, errors.New("fusion: no providers matched model filter")
	}
	arbiter := t.Arbiter
	if arbiter == nil {
		arbiter = &SimpleArbiter{}
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type execResult struct {
		candidate ResultCandidate
		err       error
	}
	ch := make(chan execResult, len(providers))
	var wg sync.WaitGroup

	for _, prov := range providers {
		wg.Add(1)
		go func(prov ProviderConfig) {
			defer wg.Done()
			provCtx, provCancel := context.WithTimeout(ctx, t.perProviderTimeout())
			defer provCancel()
			output, err := t.ExecuteFunc(provCtx, prov, plan)
			if err != nil {
				ch <- execResult{err: err}
				return
			}
			verified := false
			if t.VerifyFunc != nil {
				verified = t.VerifyFunc(provCtx, output)
			}
			ch <- execResult{candidate: ResultCandidate{
				Model:    prov.Name,
				Output:   output,
				Verified: verified,
			}}
		}(prov)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var candidates []ResultCandidate
	for er := range ch {
		if er.err != nil {
			continue
		}
		candidates = append(candidates, er.candidate)
	}

	if len(candidates) == 0 {
		return nil, errors.New("fusion: all execute providers failed")
	}
	return arbiter.PickResult(candidates)
}

// perProviderTimeout returns the configured per-provider timeout, or a
// 120s default.
func (t *PlanExecuteTournament) perProviderTimeout() time.Duration {
	if t.PerProviderTimeout > 0 {
		return t.PerProviderTimeout
	}
	return 120 * time.Second
}
