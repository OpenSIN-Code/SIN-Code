// SPDX-License-Identifier: MIT
// Purpose: SIN Fusion v1 — Verify-Tournament (issue #290).
//
// When the sacred verify-gate (M3) fails, instead of retrying with the
// same model (insanity: same blind spots, same result), fan the task out
// to N configured providers in parallel. Each provider runs an independent
// agent loop in a fresh session fork. The FIRST candidate to pass the
// verify-gate wins — the gate IS the arbiter, no separate judge model.
// Losers are cancelled via context.WithCancel and their transcripts
// recorded as labeled negatives in the lessons DB.
//
// The tournament is only valid when verify_mode == "poc" (deterministic
// predicate: compile/test/cmd). In oracle mode (LLM judge), "first to
// pass" is selection on judge noise — the tournament is disabled.
//
// Race-free (mandate M7): sync.WaitGroup + context.WithCancel +
// sync.Mutex for cost tracking. All shared state is guarded.
package fusion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// ErrInsufficientQuorum is returned when fewer than MinQuorum providers
// are available to run the tournament. The caller should fall back to
// the legacy single-model retry path.
var ErrInsufficientQuorum = errors.New("fusion: insufficient quorum for tournament")

// ErrAllProvidersFailed is returned when every provider's candidate
// failed the verify-gate. The caller should fall back to the legacy
// single-model retry path.
var ErrAllProvidersFailed = errors.New("fusion: all providers failed verification")

// ErrCostCeilingExceeded is returned when cumulative USD spend across
// all providers exceeded MaxCostUSD before any candidate passed. The
// tournament is aborted and the caller falls back to legacy retry.
var ErrCostCeilingExceeded = errors.New("fusion: cost ceiling exceeded, tournament aborted")

// ProviderConfig describes one tournament participant. Each profile
// in profiles/*.toml maps to one ProviderConfig.
type ProviderConfig struct {
	Name        string  // profile name (e.g. "fireworks", "qwen-relay")
	Model       string  // model slug
	BaseURL     string  // LLM API endpoint
	APIKey      string  // API key (from env or profile)
	InputPer1M  float64 // USD per 1M input (prompt) tokens
	OutputPer1M float64 // USD per 1M output (completion) tokens
	MaxTokens   int     // per-run token cap for this provider
	Vision      bool    // supports image input
	Thinking    bool    // supports reasoning/thinking mode
}

// RunFunc is the injected loop-runner. The tournament calls it once per
// provider in a goroutine. The implementation is responsible for building
// a provider-specific llm.Client, constructing a Loop, and running it
// against the forked session. This abstraction keeps the tournament
// package free of loopbuilder/llm dependencies — it only orchestrates.
type RunFunc func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error)

// ForkFunc creates a fresh session fork for a provider. The tournament
// needs independent sessions so each provider starts from the same
// history but diverges. Injected for testability.
type ForkFunc func(srcSessionID string, turn int) (*session.Session, error)

// Candidate is one provider's completed attempt.
type Candidate struct {
	Provider     string        `json:"provider"`
	SessionID    string        `json:"session_id"`
	Output       string        `json:"output"`
	TokensUsed   int           `json:"tokens_used"`
	LatencyMs    int64         `json:"latency_ms"`
	VerifyResult verify.Result `json:"verify_result"`
	CostUSD      float64       `json:"cost_usd"`
}

// Result is the tournament outcome.
type Result struct {
	Winner       *Candidate     `json:"winner,omitempty"`
	Losers       []Candidate    `json:"losers,omitempty"`
	AllFailed    bool           `json:"all_failed"`
	TotalCostUSD float64        `json:"total_cost_usd"`
	DurationMs   int64          `json:"duration_ms"`
	Plans        []PlanCandidate `json:"plans,omitempty"`
	MergedPlan   string          `json:"merged_plan,omitempty"`
	Mode         Mode            `json:"mode,omitempty"`
	Success      bool            `json:"success"`
	Verified     bool            `json:"verified"`
	Error        string          `json:"error,omitempty"`
	Duration     time.Duration   `json:"duration,omitempty"`
	VerifyResult verify.Result   `json:"verify_result,omitempty"`
}

// Tournament orchestrates a multi-provider verify-tournament.
type Tournament struct {
	Providers          []ProviderConfig
	RunFunc            RunFunc
	ForkFunc           ForkFunc
	VerifyFn           func(ctx context.Context, workspace string) verify.Result
	OracleJudge        OracleJudgeFn
	PlanMergeJudge     PlanMergeJudgeFn
	Mode               Mode
	MaxCostUSD         float64
	MinQuorum          int
	PerProviderTimeout time.Duration
	Workspace          string
	Prompt             string
	SourceSessionID    string
	Lessons            *lessons.Store
	Ledger             *ledger.Store
	Hooks              *hooks.Engine
	HookSessionID      string
}

// Run executes the tournament. It fans out N goroutines (one per
// provider), each forking the source session, running an independent
// loop, and then running the verify-gate against the workspace. The
// first candidate whose VerifyResult passes wins; all others are
// cancelled. Returns ErrAllProvidersFailed if none pass.
func (t *Tournament) Run(ctx context.Context) (*Result, error) {
	if t.Mode == ModeOracle {
		return t.runOracle(ctx)
	}
	if t.Mode == ModePlanMerge {
		return t.runPlanMerge(ctx)
	}
	return t.runPoC(ctx)
}

func (t *Tournament) runPoC(ctx context.Context) (*Result, error) {
	start := time.Now()

	if len(t.Providers) < t.MinQuorum {
		return nil, ErrInsufficientQuorum
	}
	if t.RunFunc == nil {
		return nil, fmt.Errorf("fusion: RunFunc not wired")
	}
	if t.ForkFunc == nil {
		return nil, fmt.Errorf("fusion: ForkFunc not wired")
	}

	t.fireHook(ctx, "fusion.dispatch", map[string]any{
		"providers": len(t.Providers),
		"mode":      "poc",
	})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	ch := make(chan Candidate, len(t.Providers))
	var wg sync.WaitGroup

	var costMu sync.Mutex
	var cumulativeCost float64
	costExceeded := false

	// safeCost reads cumulativeCost under the mutex (race-free).
	safeCost := func() float64 {
		costMu.Lock()
		defer costMu.Unlock()
		return cumulativeCost
	}

	for _, prov := range t.Providers {
		wg.Add(1)
		go func(prov ProviderConfig) {
			defer wg.Done()

			provCtx, provCancel := context.WithTimeout(ctx, t.perProviderTimeout())
			defer provCancel()

			fork, err := t.ForkFunc(t.SourceSessionID, -1)
			if err != nil {
				return
			}

			provStart := time.Now()
			res, err := t.RunFunc(provCtx, prov, fork, t.Prompt)
			if err != nil {
				return
			}
			latency := time.Since(provStart).Milliseconds()

			vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "skipped"}
			if t.VerifyFn != nil {
				vr = t.VerifyFn(provCtx, t.Workspace)
			}

			cost := (prov.InputPer1M + prov.OutputPer1M) * float64(res.Tokens) / 2_000_000

			costMu.Lock()
			cumulativeCost += cost
			exceeded := t.MaxCostUSD > 0 && cumulativeCost > t.MaxCostUSD
			if exceeded {
				costExceeded = true
			}
			costMu.Unlock()

			if exceeded {
				cancel()
				return
			}

			ch <- Candidate{
				Provider:     prov.Name,
				SessionID:    fork.ID,
				Output:       res.Summary,
				TokensUsed:   res.Tokens,
				LatencyMs:    latency,
				VerifyResult: vr,
				CostUSD:      cost,
			}
		}(prov)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var losers []Candidate
	for c := range ch {
		if c.VerifyResult.Passed {
			cancel()
			remaining := drainChannel(ch)
			passers := []Candidate{c}
			for _, r := range remaining {
				if r.VerifyResult.Passed {
					passers = append(passers, r)
				} else {
					r.VerifyResult = verify.Result{Passed: false, Mode: verify.ModePoC, Report: "cancelled (winner already found)"}
					losers = append(losers, r)
				}
			}
			winner := t.tieBreak(passers)
			for _, p := range passers {
				if p.Provider != winner.Provider {
					p.VerifyResult = verify.Result{Passed: true, Mode: verify.ModePoC, Report: "tied: lost tie-break"}
					losers = append(losers, p)
				}
			}
			result := &Result{
				Winner:       winner,
				Losers:       losers,
				TotalCostUSD: safeCost(),
				DurationMs:   time.Since(start).Milliseconds(),
			}
			t.recordOutcome(ctx, result)
			return result, nil
		}
		losers = append(losers, c)
	}

	costMu.Lock()
	exceeded := costExceeded
	costMu.Unlock()

	result := &Result{
		Losers:       losers,
		AllFailed:    true,
		TotalCostUSD: safeCost(),
		DurationMs:   time.Since(start).Milliseconds(),
	}
	t.recordOutcome(ctx, result)

	if exceeded {
		return result, ErrCostCeilingExceeded
	}
	return result, ErrAllProvidersFailed
}

// drainChannel non-blocking drains remaining candidates after a winner
// is found. These become labelled losers without running the verify-gate
// (they were cancelled mid-run).
func drainChannel(ch <-chan Candidate) []Candidate {
	var out []Candidate
	timeout := time.After(10 * time.Millisecond)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, c)
		case <-timeout:
			return out
		}
	}
}

func (t *Tournament) tieBreak(candidates []Candidate) *Candidate {
	if len(candidates) == 0 {
		return nil
	}
	sorted := make([]Candidate, len(candidates))
	copy(sorted, candidates)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].CostUSD != sorted[j].CostUSD {
			return sorted[i].CostUSD < sorted[j].CostUSD
		}
		if sorted[i].LatencyMs != sorted[j].LatencyMs {
			return sorted[i].LatencyMs < sorted[j].LatencyMs
		}
		return sorted[i].Provider < sorted[j].Provider
	})
	return &sorted[0]
}

func (t *Tournament) perProviderTimeout() time.Duration {
	if t.PerProviderTimeout > 0 {
		return t.PerProviderTimeout
	}
	return 120 * time.Second
}

func (t *Tournament) fireHook(ctx context.Context, event string, data map[string]any) {
	if t.Hooks == nil {
		return
	}
	_ = t.Hooks.Fire(ctx, hooks.Payload{
		Event:     event,
		SessionID: t.HookSessionID,
		Workspace: t.Workspace,
		Data:      data,
	})
}

func (t *Tournament) recordOutcome(ctx context.Context, result *Result) {
	if t.Ledger != nil && t.HookSessionID != "" {
		data := map[string]any{
			"total_cost_usd": result.TotalCostUSD,
			"duration_ms":    result.DurationMs,
			"all_failed":     result.AllFailed,
		}
		providersCount := len(result.Losers)
		if result.Winner != nil {
			providersCount++
		}
		data["providers_count"] = providersCount
		if result.Winner != nil {
			data["winner_provider"] = result.Winner.Provider
			data["winner_session"] = result.Winner.SessionID
			data["winner_cost_usd"] = result.Winner.CostUSD
			data["winner_duration_ms"] = result.Winner.LatencyMs
			data["winner_verified"] = result.Winner.VerifyResult.Passed
		}
		loserNames := make([]string, len(result.Losers))
		for i, l := range result.Losers {
			loserNames[i] = l.Provider
		}
		data["loser_providers"] = loserNames
		summary := "fusion tournament: all failed"
		if result.Winner != nil {
			summary = fmt.Sprintf("fusion tournament: winner=%s", result.Winner.Provider)
		}
		_, _ = t.Ledger.Record(ctx, ledger.Entry{
			SessionID: t.HookSessionID,
			Type:      ledger.TypeFusionTournament,
			Data:      data,
			Summary:   summary,
		})
	}

	if t.Lessons != nil && t.Workspace != "" {
		if result.Winner != nil {
			_ = t.Lessons.Record(ctx, lessons.Entry{
				Type:      lessons.TypeSuccessPattern,
				Workspace: t.Workspace,
				Context:   map[string]any{"provider": result.Winner.Provider, "tokens": result.Winner.TokensUsed},
				Lesson:    fmt.Sprintf("Tournament winner: %s passed verify-gate (%d tokens, %dms)", result.Winner.Provider, result.Winner.TokensUsed, result.Winner.LatencyMs),
			})
		}
		for _, l := range result.Losers {
			_ = t.Lessons.Record(ctx, lessons.Entry{
				Type:      lessons.TypeFailedVerification,
				Workspace: t.Workspace,
				Context:   map[string]any{"provider": l.Provider, "tokens": l.TokensUsed, "report": l.VerifyResult.Report},
				Lesson:    fmt.Sprintf("Tournament loser: %s failed verify-gate (%s)", l.Provider, l.VerifyResult.Report),
			})
		}
	}
}
