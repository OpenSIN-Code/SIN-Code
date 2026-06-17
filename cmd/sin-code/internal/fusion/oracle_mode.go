// SPDX-License-Identifier: MIT
// Purpose: Oracle-mode tournament support for SIN Fusion (issue #344).
//
// PoC mode (the default) uses a deterministic verify gate — compile/test/cmd —
// as the arbiter. Oracle mode uses an LLM-as-judge for tasks that cannot be
// verified mechanically (design reviews, refactoring decisions, documentation).
//
// Safety guardrails (prevent "judge race to the bottom"):
//  1. No first-pass-wins — all candidates complete before judging.
//  2. Judge cannot score its own output (self-exclusion).
//  3. Judge must provide reasoning for each score.
//  4. Scores validated: [0,1] range, no all-zero or all-one.
//  5. If judge fails: deterministic fallback (longest verified output wins).
//
// Thread-safe (mandate M7).
package fusion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// ErrNoEligibleAttempts is returned when all attempts are from the judge
// model, leaving no candidates to evaluate.
var ErrNoEligibleAttempts = errors.New("fusion: no eligible attempts (all from judge model)")

// Task represents a coding task to be evaluated by the tournament.
type Task struct {
	Prompt    string
	Workspace string
}

// Attempt is one provider's completed output for a task.
type Attempt struct {
	Model    string
	Output   string
	Verified bool
}

// ModelScore is the judge's evaluation of one model's output.
type ModelScore struct {
	Model     string  `json:"model"`
	Score     float64 `json:"score"`
	Reasoning string  `json:"reasoning"`
}

// OracleResult is the outcome of an oracle tournament.
type OracleResult struct {
	Winner         string       `json:"winner"`
	Score          float64      `json:"score"`
	JudgeReasoning string       `json:"judge_reasoning"`
	AllScores      []ModelScore `json:"all_scores"`
	FallbackUsed   bool         `json:"fallback_used"`
}

// JudgeFunc is the injected LLM-judge function. It receives all eligible
// attempts (excluding the judge model's own output) and returns a score
// for each, along with an overall reasoning string. If it returns an
// error, the tournament falls back to deterministic scoring.
type JudgeFunc func(ctx context.Context, attempts []Attempt) ([]ModelScore, string, error)

// OracleTournament runs a tournament with an LLM judge instead of a PoC gate.
// All candidates complete their full output before the judge evaluates them
// together — there is no first-pass-wins.
type OracleTournament struct {
	Pool       *ProviderPool
	JudgeModel string
	JudgeFn    JudgeFunc
	MaxCostUSD float64
	mu         sync.Mutex
}

// NewOracleTournament creates an OracleTournament with the given provider
// pool and judge model name. The default cost cap is $2.00 (tighter than
// PoC mode's $5.00) because oracle mode is strictly more expensive.
func NewOracleTournament(pool *ProviderPool, judgeModel string) *OracleTournament {
	return &OracleTournament{
		Pool:       pool,
		JudgeModel: judgeModel,
		MaxCostUSD: 2.0,
	}
}

// Run evaluates all attempts using the judge and returns the winner.
// The judge model's own output is excluded (self-exclusion safety rule).
// If the judge fails or produces invalid scores, the tournament falls
// back to deterministic scoring (longest verified output wins).
func (t *OracleTournament) Run(ctx context.Context, task *Task, attempts []Attempt) (*OracleResult, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Safety rule 2: exclude judge model's own output.
	var eligible []Attempt
	for _, a := range attempts {
		if a.Model != t.JudgeModel {
			eligible = append(eligible, a)
		}
	}
	if len(eligible) == 0 {
		return nil, ErrNoEligibleAttempts
	}

	var scores []ModelScore
	var reasoning string
	var fallbackUsed bool

	if t.JudgeFn != nil {
		var err error
		scores, reasoning, err = t.JudgeFn(ctx, eligible)
		if err != nil || t.ValidateJudge(scores) != nil {
			fallbackUsed = true
			scores, reasoning = deterministicScoring(eligible)
		}
	} else {
		fallbackUsed = true
		scores, reasoning = deterministicScoring(eligible)
	}

	// Find the winner with deterministic tie-break: score → model name.
	winnerIdx := 0
	for i := 1; i < len(scores); i++ {
		if scores[i].Score > scores[winnerIdx].Score {
			winnerIdx = i
		} else if scores[i].Score == scores[winnerIdx].Score && scores[i].Model < scores[winnerIdx].Model {
			winnerIdx = i
		}
	}

	return &OracleResult{
		Winner:         scores[winnerIdx].Model,
		Score:          scores[winnerIdx].Score,
		JudgeReasoning: reasoning,
		AllScores:      scores,
		FallbackUsed:   fallbackUsed,
	}, nil
}

// ValidateJudge validates judge scores against the safety rules:
//   - Each score must be in [0, 1].
//   - No all-zero scores (judge not paying attention).
//   - No all-one scores (judge rubber-stamping).
//   - Each score must have non-empty reasoning.
//
// Returns nil if valid, an error describing the violation otherwise.
func (t *OracleTournament) ValidateJudge(scores []ModelScore) error {
	if len(scores) == 0 {
		return errors.New("fusion: no scores from judge")
	}
	allZero := true
	allOne := true
	for _, s := range scores {
		if s.Score < 0 || s.Score > 1 {
			return fmt.Errorf("fusion: score %f for %s out of range [0,1]", s.Score, s.Model)
		}
		if s.Reasoning == "" {
			return fmt.Errorf("fusion: missing reasoning for %s", s.Model)
		}
		if s.Score != 0 {
			allZero = false
		}
		if s.Score != 1 {
			allOne = false
		}
	}
	if allZero {
		return errors.New("fusion: all scores are zero (judge not discriminating)")
	}
	if allOne {
		return errors.New("fusion: all scores are one (judge rubber-stamping)")
	}
	return nil
}

// deterministicScoring is the fallback when the judge fails: the longest
// verified output wins. Unverified outputs receive a score of 0. Scores
// are normalized to [0, 1] relative to the longest verified output.
func deterministicScoring(attempts []Attempt) ([]ModelScore, string) {
	scores := make([]ModelScore, len(attempts))
	maxLen := 0
	for i, a := range attempts {
		score := 0.0
		if a.Verified {
			score = float64(len(a.Output))
			if len(a.Output) > maxLen {
				maxLen = len(a.Output)
			}
		}
		scores[i] = ModelScore{
			Model:     a.Model,
			Score:     score,
			Reasoning: fmt.Sprintf("deterministic fallback: verified=%v, output_len=%d", a.Verified, len(a.Output)),
		}
	}
	if maxLen > 0 {
		for i := range scores {
			scores[i].Score = scores[i].Score / float64(maxLen)
		}
	}
	sort.SliceStable(scores, func(i, j int) bool {
		if scores[i].Score != scores[j].Score {
			return scores[i].Score > scores[j].Score
		}
		return scores[i].Model < scores[j].Model
	})
	return scores, "deterministic fallback: longest verified output wins"
}
