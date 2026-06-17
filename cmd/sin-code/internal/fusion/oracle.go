// SPDX-License-Identifier: MIT
// Purpose: Oracle-mode tournament support for SIN Fusion v1 (issue #344).
//
// Oracle mode differs from PoC mode in one critical way: the verify function
// is an LLM judge, which is subjective. First-pass-wins would select for the
// model that produces the most optimistic judge prompt, not the best output.
// Therefore oracle mode runs ALL candidates to completion, presents their
// outputs to a single judge in randomized order, and picks the highest-scoring
// candidate by a structured rubric. Tie-break follows the same deterministic
// rule as PoC mode: cost → latency → name.
//
// Race-free (mandate M7): the judge runs after all goroutines finish, and the
// cost accumulator is guarded by the same mutex used in PoC mode.
package fusion

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// Mode selects the tournament verification strategy.
type Mode string

const (
	ModePoC    Mode = "poc"
	ModeOracle Mode = "oracle"
)

// OracleScore is the structured rubric a judge returns for one candidate.
type OracleScore struct {
	Correctness  int    `json:"correctness_score"`
	Completeness int    `json:"completeness_score"`
	Risk         int    `json:"risk_score"`
	Total        int    `json:"total_score"`
	Reasoning    string `json:"reasoning"`
}

// OracleVerdict is the judge's complete evaluation of all candidates.
type OracleVerdict struct {
	WinnerProvider string                 `json:"winner_candidate_id"`
	Scores         map[string]OracleScore `json:"scores"`
	Reasoning      string                 `json:"reasoning"`
}

// OracleJudgeFn evaluates all candidates together and returns a verdict.
// The judge MUST NOT see candidate identifiers in a fixed order; the caller
// is responsible for randomizing order to reduce position bias.
type OracleJudgeFn func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error)

// DefaultOracleMaxScore is the maximum value per rubric dimension.
const DefaultOracleMaxScore = 10

// LLMOracleJudge is the default judge implementation backed by an LLM client.
// It requests a structured JSON rubric and parses the response. A nil client
// or empty model returns an error on every call — this is fail-closed.
type LLMOracleJudge struct {
	Client    *llm.Client
	ModelName string
	MaxScore  int
	Rubric    string
}

// NewLLMOracleJudge creates a fail-closed judge. If client is nil or modelName
// is empty, the returned judge returns an error on every call.
func NewLLMOracleJudge(client *llm.Client, modelName string) *LLMOracleJudge {
	return &LLMOracleJudge{
		Client:    client,
		ModelName: modelName,
		MaxScore:  DefaultOracleMaxScore,
		Rubric:    defaultOracleRubric(),
	}
}

func defaultOracleRubric() string {
	return "You are a strict, reproducible evaluator. Several candidate solutions were generated for the same prompt. Evaluate each candidate independently on a 0-10 scale for:\n" +
		"- correctness_score: does it actually solve the task?\n" +
		"- completeness_score: does it cover all requirements and edge cases?\n" +
		"- risk_score: how likely is this solution to introduce regressions, bugs, or security issues? (lower is safer)\n\n" +
		"Return ONLY valid JSON in this exact shape:\n" +
		"{\n" +
		"  \"scores\": {\n" +
		"    \"candidate-0\": {\"correctness_score\": 8, \"completeness_score\": 7, \"risk_score\": 2, \"reasoning\": \"...\"},\n" +
		"    ...\n" +
		"  },\n" +
		"  \"winner_candidate_id\": \"candidate-0\",\n" +
		"  \"reasoning\": \"short summary of why the winner is best\"\n" +
		"}\n\n" +
		"Do not include any markdown, explanation, or commentary outside the JSON."
}


// Judge evaluates all candidates and returns a verdict. It randomizes the
// candidate order presented to the LLM and maps the LLM's candidate IDs back
// to provider names before returning.
func (j *LLMOracleJudge) Judge(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
	if j.Client == nil || j.Client.BaseURL == "" || j.Client.APIKey == "" || j.ModelName == "" {
		return OracleVerdict{}, errors.New("fusion: oracle judge not configured (client/model missing)")
	}
	if len(candidates) == 0 {
		return OracleVerdict{}, errors.New("fusion: no candidates to judge")
	}

	maxScore := j.MaxScore
	if maxScore <= 0 {
		maxScore = DefaultOracleMaxScore
	}

	// Randomize order to mitigate position bias.
	shuffled := make([]Candidate, len(candidates))
	copy(shuffled, candidates)
	for i := len(shuffled) - 1; i > 0; i-- {
		jBig, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			continue
		}
		j := int(jBig.Int64())
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	userPrompt := buildOracleJudgePrompt(prompt, shuffled, maxScore)
	resp, err := j.Client.Chat(ctx, llm.ChatRequest{
		Model:       j.resolveModel(ctx),
		Messages:    []llm.Message{{Role: "system", Content: j.Rubric}, {Role: "user", Content: userPrompt}},
		MaxTokens:   4096,
		Temperature: 0.0,
	})
	if err != nil {
		return OracleVerdict{}, fmt.Errorf("fusion: oracle judge LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return OracleVerdict{}, errors.New("fusion: oracle judge returned empty response")
	}

	return parseOracleVerdict(resp.Choices[0].Message.Content, shuffled, maxScore)
}

func (j *LLMOracleJudge) resolveModel(_ context.Context) string {
	return j.ModelName
}

func buildOracleJudgePrompt(prompt string, candidates []Candidate, maxScore int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task prompt:\n%s\n\nEvaluate the following %d candidate solutions:\n\n", prompt, len(candidates))
	for i, c := range candidates {
		fmt.Fprintf(&b, "--- candidate-%d ---\n%s\n\n", i, c.Output)
	}
	fmt.Fprintf(&b, "Score each candidate 0-%d. Return ONLY the required JSON object.", maxScore)
	return b.String()
}

func parseOracleVerdict(raw string, candidates []Candidate, maxScore int) (OracleVerdict, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") {
		lines := strings.Split(raw, "\n")
		var inside []string
		inBlock := false
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				inside = append(inside, line)
			}
		}
		raw = strings.Join(inside, "\n")
	}

	var payload struct {
		Scores map[string]struct {
			Correctness  int    `json:"correctness_score"`
			Completeness int    `json:"completeness_score"`
			Risk         int    `json:"risk_score"`
			Reasoning    string `json:"reasoning"`
		} `json:"scores"`
		Winner string `json:"winner_candidate_id"`
		Reason string `json:"reasoning"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return OracleVerdict{}, fmt.Errorf("fusion: oracle judge returned invalid JSON: %w", err)
	}

	idToProvider := make(map[string]string, len(candidates))
	for i, c := range candidates {
		idToProvider[fmt.Sprintf("candidate-%d", i)] = c.Provider
	}

	verdict := OracleVerdict{
		WinnerProvider: "",
		Scores:         make(map[string]OracleScore, len(candidates)),
		Reasoning:      payload.Reason,
	}

	if winnerProvider, ok := idToProvider[payload.Winner]; ok {
		verdict.WinnerProvider = winnerProvider
	}

	for i, c := range candidates {
		id := fmt.Sprintf("candidate-%d", i)
		score := OracleScore{}
		if s, ok := payload.Scores[id]; ok {
			score.Correctness = clampScore(s.Correctness, maxScore)
			score.Completeness = clampScore(s.Completeness, maxScore)
			score.Risk = clampScore(s.Risk, maxScore)
			score.Reasoning = s.Reasoning
			score.Total = score.Correctness + score.Completeness + (maxScore - score.Risk)
		}
		verdict.Scores[c.Provider] = score
	}

	if verdict.WinnerProvider == "" {
		// Fallback: pick highest total score deterministically.
		verdict.WinnerProvider = pickHighestScore(verdict.Scores, candidates)
	}

	return verdict, nil
}

func clampScore(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

func pickHighestScore(scores map[string]OracleScore, candidates []Candidate) string {
	type scored struct {
		provider string
		score    int
		cost     float64
		latency  int64
	}
	var all []scored
	for _, c := range candidates {
		all = append(all, scored{provider: c.Provider, score: scores[c.Provider].Total, cost: c.CostUSD, latency: c.LatencyMs})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if all[i].cost != all[j].cost {
			return all[i].cost < all[j].cost
		}
		if all[i].latency != all[j].latency {
			return all[i].latency < all[j].latency
		}
		return all[i].provider < all[j].provider
	})
	if len(all) == 0 {
		return ""
	}
	return all[0].provider
}

// runOracle executes the oracle-mode tournament. All candidates run to
// completion; a single judge evaluates all outputs together in randomized
// order; the highest-scoring candidate wins.
func (t *Tournament) runOracle(ctx context.Context) (*Result, error) {
	start := time.Now()

	if t.RunFunc == nil {
		return nil, fmt.Errorf("fusion: RunFunc not wired")
	}
	if t.ForkFunc == nil {
		return nil, fmt.Errorf("fusion: ForkFunc not wired")
	}
	if t.OracleJudge == nil {
		return nil, fmt.Errorf("fusion: OracleJudge not wired for oracle mode")
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	t.fireHook(ctx, "fusion.dispatch", map[string]any{
		"providers": len(t.Providers),
		"mode":      "oracle",
	})

	ch := make(chan Candidate, len(t.Providers))
	var wg sync.WaitGroup

	var costMu sync.Mutex
	var cumulativeCost float64
	costExceeded := false
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
				Provider:   prov.Name,
				SessionID:  fork.ID,
				Output:     res.Summary,
				TokensUsed: res.Tokens,
				LatencyMs:  latency,
				CostUSD:    cost,
			}
		}(prov)
	}

	go func() {
		wg.Wait()
		close(ch)
	}()

	var candidates []Candidate
	for c := range ch {
		candidates = append(candidates, c)
	}

	costMu.Lock()
	exceeded := costExceeded
	costMu.Unlock()

	if exceeded || (t.MaxCostUSD > 0 && safeCost() > t.MaxCostUSD) {
		result := &Result{
			Losers:       candidates,
			AllFailed:    true,
			TotalCostUSD: safeCost(),
			DurationMs:   time.Since(start).Milliseconds(),
		}
		t.recordOutcome(ctx, result)
		return result, ErrCostCeilingExceeded
	}

	if len(candidates) == 0 {
		result := &Result{
			AllFailed:    true,
			TotalCostUSD: safeCost(),
			DurationMs:   time.Since(start).Milliseconds(),
		}
		t.recordOutcome(ctx, result)
		return result, ErrAllProvidersFailed
	}

	verdict, err := t.OracleJudge(ctx, t.Prompt, candidates)
	if err != nil {
		result := &Result{
			Losers:       candidates,
			AllFailed:    true,
			TotalCostUSD: safeCost(),
			DurationMs:   time.Since(start).Milliseconds(),
		}
		t.recordOutcome(ctx, result)
		return result, fmt.Errorf("fusion: oracle judge failed: %w", err)
	}

	winnerIdx := -1
	for i, c := range candidates {
		if c.Provider == verdict.WinnerProvider {
			winnerIdx = i
			break
		}
	}

	if winnerIdx < 0 {
		// Fallback to deterministic score tie-break if judge returned unknown winner.
		verdict.WinnerProvider = pickHighestScore(verdict.Scores, candidates)
		for i, c := range candidates {
			if c.Provider == verdict.WinnerProvider {
				winnerIdx = i
				break
			}
		}
	}

	var winner *Candidate
	var losers []Candidate
	if winnerIdx >= 0 {
		w := candidates[winnerIdx]
		w.VerifyResult = verify.Result{Passed: true, Mode: verify.ModeOracle, Report: "oracle judge winner"}
		winner = &w
		for i, c := range candidates {
			if i != winnerIdx {
				c.VerifyResult = verify.Result{Passed: false, Mode: verify.ModeOracle, Report: "oracle judge loser"}
				losers = append(losers, c)
			}
		}
	} else {
		losers = candidates
	}

	result := &Result{
		Winner:       winner,
		Losers:       losers,
		TotalCostUSD: safeCost(),
		DurationMs:   time.Since(start).Milliseconds(),
	}
	t.recordOutcome(ctx, result)
	if winner == nil {
		return result, ErrAllProvidersFailed
	}
	return result, nil
}
