// SPDX-License-Identifier: MIT
// Purpose: Coverage gap tests for the fusion package (issue #290, #321, #344).
// Targets uncovered error paths, edge cases, and helper functions.
// All tests pass under `go test -race -count=1` (mandate M7).
package fusion

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/hooks"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

// ---------------------------------------------------------------------------
// oracle.go coverage
// ---------------------------------------------------------------------------

func TestBuildOracleJudgePrompt(t *testing.T) {
	candidates := []Candidate{
		{Provider: "a", Output: "solution A"},
		{Provider: "b", Output: "solution B"},
	}
	prompt := buildOracleJudgePrompt("do the task", candidates, 10)
	if !strings.Contains(prompt, "do the task") {
		t.Error("prompt should contain the task prompt")
	}
	if !strings.Contains(prompt, "solution A") {
		t.Error("prompt should contain candidate A output")
	}
	if !strings.Contains(prompt, "solution B") {
		t.Error("prompt should contain candidate B output")
	}
	if !strings.Contains(prompt, "0-10") {
		t.Error("prompt should mention score range")
	}
}

func TestPickHighestScore_Empty(t *testing.T) {
	got := pickHighestScore(nil, nil)
	if got != "" {
		t.Errorf("expected empty string for no candidates, got %q", got)
	}
}

func TestPickHighestScore_ByScore(t *testing.T) {
	scores := map[string]OracleScore{
		"low":  {Total: 5},
		"high": {Total: 20},
		"mid":  {Total: 10},
	}
	candidates := []Candidate{
		{Provider: "low"},
		{Provider: "high"},
		{Provider: "mid"},
	}
	got := pickHighestScore(scores, candidates)
	if got != "high" {
		t.Errorf("expected 'high', got %q", got)
	}
}

func TestPickHighestScore_TieBreakByCost(t *testing.T) {
	scores := map[string]OracleScore{
		"cheap": {Total: 10},
		"pricey": {Total: 10},
	}
	candidates := []Candidate{
		{Provider: "cheap", CostUSD: 0.01},
		{Provider: "pricey", CostUSD: 0.10},
	}
	got := pickHighestScore(scores, candidates)
	if got != "cheap" {
		t.Errorf("expected 'cheap' (lower cost), got %q", got)
	}
}

func TestPickHighestScore_TieBreakByLatency(t *testing.T) {
	scores := map[string]OracleScore{
		"fast": {Total: 10},
		"slow": {Total: 10},
	}
	candidates := []Candidate{
		{Provider: "fast", CostUSD: 0.01, LatencyMs: 100},
		{Provider: "slow", CostUSD: 0.01, LatencyMs: 500},
	}
	got := pickHighestScore(scores, candidates)
	if got != "fast" {
		t.Errorf("expected 'fast' (lower latency), got %q", got)
	}
}

func TestPickHighestScore_TieBreakByName(t *testing.T) {
	scores := map[string]OracleScore{
		"zeta":  {Total: 10},
		"alpha": {Total: 10},
	}
	candidates := []Candidate{
		{Provider: "zeta", CostUSD: 0.01, LatencyMs: 100},
		{Provider: "alpha", CostUSD: 0.01, LatencyMs: 100},
	}
	got := pickHighestScore(scores, candidates)
	if got != "alpha" {
		t.Errorf("expected 'alpha' (alphabetical), got %q", got)
	}
}

func TestLLMOracleJudge_ResolveModel(t *testing.T) {
	j := &LLMOracleJudge{
		Client:    &llm.Client{BaseURL: "http://x", APIKey: "k"},
		ModelName: "test-model",
	}
	if got := j.resolveModel(context.Background()); got != "test-model" {
		t.Errorf("expected 'test-model', got %q", got)
	}
}

func TestLLMOracleJudge_EmptyCandidates(t *testing.T) {
	j := NewLLMOracleJudge(&llm.Client{BaseURL: "http://x", APIKey: "k"}, "model")
	_, err := j.Judge(context.Background(), "prompt", nil)
	if err == nil {
		t.Fatal("expected error for empty candidates")
	}
	if !strings.Contains(err.Error(), "no candidates") {
		t.Errorf("expected 'no candidates' error, got: %v", err)
	}
}

func TestLLMOracleJudge_MaxScoreDefault(t *testing.T) {
	srv := stubOracleServer(t, `{"scores":{"candidate-0":{"correctness_score":5,"completeness_score":5,"risk_score":2,"reasoning":"r"}},"winner_candidate_id":"candidate-0","reasoning":"ok"}`)
	defer srv.Close()

	j := &LLMOracleJudge{
		Client:    llm.NewClient(srv.URL, "test-key"),
		ModelName: "test-model",
		MaxScore:  0, // should default to DefaultOracleMaxScore
	}
	candidates := []Candidate{{Provider: "a", Output: "x"}}
	verdict, err := j.Judge(context.Background(), "prompt", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if verdict.WinnerProvider != "a" {
		t.Errorf("expected winner 'a', got %q", verdict.WinnerProvider)
	}
}

func TestLLMOracleJudge_SuccessViaHTTP(t *testing.T) {
	srv := stubOracleServer(t, `{"scores":{"candidate-0":{"correctness_score":9,"completeness_score":8,"risk_score":1,"reasoning":"good"},"candidate-1":{"correctness_score":3,"completeness_score":3,"risk_score":7,"reasoning":"bad"}},"winner_candidate_id":"candidate-0","reasoning":"a wins"}`)
	defer srv.Close()

	j := NewLLMOracleJudge(llm.NewClient(srv.URL, "test-key"), "test-model")
	candidates := []Candidate{
		{Provider: "a", Output: "good solution"},
		{Provider: "b", Output: "bad solution"},
	}
	verdict, err := j.Judge(context.Background(), "do the task", candidates)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The Judge method randomizes candidate order to mitigate position bias,
	// so candidate-0 may map to either "a" or "b". The mock always scores
	// candidate-0 high (total 26) and candidate-1 low (total 9). Verify the
	// winner has the high score and the loser has the low score, regardless
	// of which provider the shuffle assigned to which index.
	if verdict.WinnerProvider == "" {
		t.Fatal("expected non-empty winner")
	}
	winnerScore := verdict.Scores[verdict.WinnerProvider]
	if winnerScore.Total != 9+8+(10-1) {
		t.Errorf("expected winner total 26, got %d (winner=%s)", winnerScore.Total, verdict.WinnerProvider)
	}
	// Find the loser and verify it has the low score.
	for _, c := range candidates {
		if c.Provider != verdict.WinnerProvider {
			loserScore := verdict.Scores[c.Provider]
			if loserScore.Total != 3+3+(10-7) {
				t.Errorf("expected loser total 9, got %d (loser=%s)", loserScore.Total, c.Provider)
			}
		}
	}
}

func TestLLMOracleJudge_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": ""}, "finish_reason": "stop"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	j := NewLLMOracleJudge(llm.NewClient(srv.URL, "test-key"), "test-model")
	_, err := j.Judge(context.Background(), "prompt", []Candidate{{Provider: "a", Output: "x"}})
	if err == nil {
		t.Fatal("expected error for empty response")
	}
	if !strings.Contains(err.Error(), "empty response") {
		t.Errorf("expected 'empty response' error, got: %v", err)
	}
}

func TestLLMOracleJudge_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"choices": []map[string]any{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	j := NewLLMOracleJudge(llm.NewClient(srv.URL, "test-key"), "test-model")
	_, err := j.Judge(context.Background(), "prompt", []Candidate{{Provider: "a", Output: "x"}})
	if err == nil {
		t.Fatal("expected error for no choices")
	}
}

func TestLLMOracleJudge_LLMCallFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	j := NewLLMOracleJudge(llm.NewClient(srv.URL, "test-key"), "test-model")
	_, err := j.Judge(context.Background(), "prompt", []Candidate{{Provider: "a", Output: "x"}})
	if err == nil {
		t.Fatal("expected error for server failure")
	}
}

func TestParseOracleVerdict_InvalidJSON(t *testing.T) {
	_, err := parseOracleVerdict("not json at all", []Candidate{{Provider: "a"}}, 10)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseOracleVerdict_UnknownWinnerFallback(t *testing.T) {
	candidates := []Candidate{
		{Provider: "a", Output: "x", CostUSD: 0.01},
		{Provider: "b", Output: "y", CostUSD: 0.02},
	}
	raw := `{"scores":{"candidate-0":{"correctness_score":9,"completeness_score":9,"risk_score":1,"reasoning":"r0"},"candidate-1":{"correctness_score":3,"completeness_score":3,"risk_score":7,"reasoning":"r1"}},"winner_candidate_id":"candidate-99","reasoning":"unknown"}`
	verdict, err := parseOracleVerdict(raw, candidates, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Winner is unknown, so pickHighestScore should be used: a has total 27, b has 9
	if verdict.WinnerProvider != "a" {
		t.Errorf("expected fallback winner 'a', got %q", verdict.WinnerProvider)
	}
}

func TestParseOracleVerdict_MissingScoreForCandidate(t *testing.T) {
	candidates := []Candidate{
		{Provider: "a", Output: "x"},
		{Provider: "b", Output: "y"},
	}
	raw := `{"scores":{"candidate-0":{"correctness_score":9,"completeness_score":9,"risk_score":1,"reasoning":"r0"}},"winner_candidate_id":"candidate-0","reasoning":"ok"}`
	verdict, err := parseOracleVerdict(raw, candidates, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// candidate-1 has no score entry — should default to zero
	if verdict.Scores["b"].Total != 0 {
		t.Errorf("expected total 0 for missing score, got %d", verdict.Scores["b"].Total)
	}
}

// stubOracleServer creates an httptest server that returns the given JSON
// as the LLM response content.
func stubOracleServer(t *testing.T, responseJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": responseJSON}, "finish_reason": "stop"},
			},
			"usage": map[string]any{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

// ---------------------------------------------------------------------------
// tournament.go coverage — recordOutcome, fireHook, tieBreak, drainChannel
// ---------------------------------------------------------------------------

func TestRecordOutcome_WithLedgerAndLessons(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	lessonsStore, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatalf("open lessons: %v", err)
	}
	defer lessonsStore.Close()

	tournament := &Tournament{
		Ledger:        ledgerStore,
		Lessons:       lessonsStore,
		HookSessionID: "test-session",
		Workspace:     "/test/ws",
	}

	result := &Result{
		Winner: &Candidate{
			Provider:     "winner",
			SessionID:    "sess-1",
			TokensUsed:   100,
			LatencyMs:    50,
			CostUSD:      0.01,
			VerifyResult: verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"},
		},
		Losers: []Candidate{
			{Provider: "loser1", TokensUsed: 50, VerifyResult: verify.Result{Passed: false, Report: "failed"}},
		},
		TotalCostUSD: 0.02,
		DurationMs:   100,
	}

	tournament.recordOutcome(context.Background(), result)
	// If we get here without panic, the ledger+lessons paths are covered.
}

func TestRecordOutcome_AllFailed(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	lessonsStore, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatalf("open lessons: %v", err)
	}
	defer lessonsStore.Close()

	tournament := &Tournament{
		Ledger:        ledgerStore,
		Lessons:       lessonsStore,
		HookSessionID: "fail-session",
		Workspace:     "/test/ws",
	}

	result := &Result{
		AllFailed:    true,
		Losers:       []Candidate{{Provider: "a", TokensUsed: 10, VerifyResult: verify.Result{Passed: false, Report: "failed"}}},
		TotalCostUSD: 0.001,
		DurationMs:   50,
	}

	tournament.recordOutcome(context.Background(), result)
}

func TestRecordOutcome_NilStores(t *testing.T) {
	tournament := &Tournament{}
	result := &Result{
		Winner: &Candidate{Provider: "w", TokensUsed: 10, LatencyMs: 5, CostUSD: 0.01},
	}
	// Should not panic with nil Ledger/Lessons
	tournament.recordOutcome(context.Background(), result)
}

func TestRecordOutcome_LedgerWithoutSessionID(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	tournament := &Tournament{
		Ledger:        ledgerStore,
		HookSessionID: "", // empty session ID — should skip ledger recording
	}
	result := &Result{AllFailed: true}
	tournament.recordOutcome(context.Background(), result)
}

func TestFireHook_WithEngine(t *testing.T) {
	engine := hooks.New([]hooks.Hook{
		{Event: "fusion.dispatch", Type: "prompt", Text: "injected context"},
	})
	tournament := &Tournament{
		Hooks:         engine,
		HookSessionID: "hook-test",
		Workspace:     "/ws",
	}
	// Should not panic
	tournament.fireHook(context.Background(), "fusion.dispatch", map[string]any{"providers": 2})
}

func TestFireHook_NilEngine(t *testing.T) {
	tournament := &Tournament{}
	tournament.fireHook(context.Background(), "fusion.dispatch", nil)
}

func TestTieBreak_Empty(t *testing.T) {
	tournament := &Tournament{}
	got := tournament.tieBreak(nil)
	if got != nil {
		t.Errorf("expected nil for empty candidates, got %v", got)
	}
}

func TestTieBreak_SingleCandidate(t *testing.T) {
	tournament := &Tournament{}
	c := []Candidate{{Provider: "only"}}
	got := tournament.tieBreak(c)
	if got == nil || got.Provider != "only" {
		t.Errorf("expected 'only', got %v", got)
	}
}

func TestDrainChannel_Timeout(t *testing.T) {
	ch := make(chan Candidate, 2)
	// Don't send anything — drainChannel should timeout and return empty
	got := drainChannel(ch)
	if len(got) != 0 {
		t.Errorf("expected 0 candidates on timeout, got %d", len(got))
	}
}

func TestDrainChannel_ClosedChannel(t *testing.T) {
	ch := make(chan Candidate, 2)
	ch <- Candidate{Provider: "a"}
	close(ch)
	got := drainChannel(ch)
	if len(got) != 1 {
		t.Errorf("expected 1 candidate, got %d", len(got))
	}
	if got[0].Provider != "a" {
		t.Errorf("expected 'a', got %q", got[0].Provider)
	}
}

// ---------------------------------------------------------------------------
// provider_pool.go coverage
// ---------------------------------------------------------------------------

func TestResolveAPIKey_EmptyProvider(t *testing.T) {
	got := resolveAPIKey("")
	if got != "" {
		t.Errorf("expected empty string for empty provider, got %q", got)
	}
}

func TestResolveAPIKey_EnvSet(t *testing.T) {
	t.Setenv("TESTPROVIDER_API_KEY", "secret123")
	got := resolveAPIKey("testprovider")
	if got != "secret123" {
		t.Errorf("expected 'secret123', got %q", got)
	}
}

func TestResolveAPIKey_EnvNotSet(t *testing.T) {
	// Make sure the env var is not set
	os.Unsetenv("NONEXISTENTPROV_API_KEY")
	got := resolveAPIKey("nonexistentprov")
	if got != "" {
		t.Errorf("expected empty string for unset env var, got %q", got)
	}
}

func TestEstimateInputPrice(t *testing.T) {
	tests := []struct {
		provider string
		want     float64
	}{
		{"fireworks", 1.0},
		{"qwen-relay", 0.0},
		{"unknown", 2.0},
		{"", 2.0},
		{"FIREWORKS", 1.0}, // case-insensitive
	}
	for _, tt := range tests {
		got := estimateInputPrice(tt.provider, "any-model")
		if got != tt.want {
			t.Errorf("estimateInputPrice(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}

func TestEstimateOutputPrice(t *testing.T) {
	tests := []struct {
		provider string
		want     float64
	}{
		{"fireworks", 3.0},
		{"qwen-relay", 0.0},
		{"unknown", 5.0},
		{"", 5.0},
		{"QWEN-RELAY", 0.0}, // case-insensitive
	}
	for _, tt := range tests {
		got := estimateOutputPrice(tt.provider, "any-model")
		if got != tt.want {
			t.Errorf("estimateOutputPrice(%q) = %v, want %v", tt.provider, got, tt.want)
		}
	}
}

func TestLoadProviderPool_DecodeError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.toml")
	if err := os.WriteFile(path, []byte("this is not valid toml = = =\n[broken"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := LoadProviderPool(dir, nil)
	if err == nil {
		t.Fatal("expected error for invalid TOML")
	}
}

func TestLoadProviderPool_ReadFileError(t *testing.T) {
	dir := t.TempDir()
	// Create a symlink to a nonexistent file — os.ReadDir lists it, but
	// os.ReadFile fails when following the broken symlink.
	linkPath := filepath.Join(dir, "broken.toml")
	target := filepath.Join(dir, "nonexistent_target")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, err := LoadProviderPool(dir, nil)
	if err == nil {
		t.Fatal("expected error when reading broken symlink")
	}
}

func TestLoadProviderPool_EmptyNameSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "noname.toml")
	if err := os.WriteFile(path, []byte(`name = ""
model = "test-model"
base_url = "http://test"
provider = "test"
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pool, err := LoadProviderPool(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pool) != 0 {
		t.Errorf("expected 0 profiles (empty name skipped), got %d", len(pool))
	}
}

func TestLoadProviderPool_NonTomlSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("# not a profile"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pool, err := LoadProviderPool(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pool) != 0 {
		t.Errorf("expected 0 profiles, got %d", len(pool))
	}
}

func TestLoadProviderPool_ValidProfile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "testprof.toml")
	if err := os.WriteFile(path, []byte(`name = "testprof"
model = "accounts/fireworks/models/test"
base_url = "https://api.test.com/v1"
provider = "fireworks"
max_tokens = 4096
`), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	pool, err := LoadProviderPool(dir, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pool) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(pool))
	}
	if pool[0].Name != "testprof" {
		t.Errorf("expected 'testprof', got %q", pool[0].Name)
	}
	if pool[0].InputPer1M != 1.0 {
		t.Errorf("expected input price 1.0 for fireworks, got %v", pool[0].InputPer1M)
	}
	if pool[0].OutputPer1M != 3.0 {
		t.Errorf("expected output price 3.0 for fireworks, got %v", pool[0].OutputPer1M)
	}
}

// ---------------------------------------------------------------------------
// plan_execute.go coverage
// ---------------------------------------------------------------------------

func TestSimpleArbiter_PickResult_Empty(t *testing.T) {
	a := &SimpleArbiter{}
	_, err := a.PickResult(nil)
	if err == nil {
		t.Fatal("expected error for no result candidates")
	}
}

func TestSimpleArbiter_PickPlan_TieBreakByName(t *testing.T) {
	a := &SimpleArbiter{}
	plans := []PlanCandidate{
		{Model: "zeta", Plan: "same"},
		{Model: "alpha", Plan: "same"},
	}
	best, err := a.PickPlan(plans)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best.Model != "alpha" {
		t.Errorf("expected 'alpha' (alphabetical tie-break), got %q", best.Model)
	}
}

func TestProviderPool_Get_NilPool(t *testing.T) {
	var pool *ProviderPool
	got := pool.Get(nil)
	if got != nil {
		t.Errorf("expected nil for nil pool, got %v", got)
	}
}

func TestProviderPool_Get_NoMatch(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}, {Name: "b"}})
	got := pool.Get([]string{"nonexistent"})
	if len(got) != 0 {
		t.Errorf("expected 0 for no match, got %d", len(got))
	}
}

func TestPlanExecuteTournament_Plan_NilTournament(t *testing.T) {
	var tournament *PlanExecuteTournament
	_, err := tournament.Plan(context.Background(), "task", nil)
	if err == nil {
		t.Fatal("expected error for nil tournament")
	}
}

func TestPlanExecuteTournament_Plan_NilPool(t *testing.T) {
	tournament := &PlanExecuteTournament{}
	_, err := tournament.Plan(context.Background(), "task", nil)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestPlanExecuteTournament_Plan_NilPlanFunc(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	_, err := tournament.Plan(context.Background(), "task", nil)
	if err == nil {
		t.Fatal("expected error for nil PlanFunc")
	}
}

func TestPlanExecuteTournament_Execute_NilTournament(t *testing.T) {
	var tournament *PlanExecuteTournament
	_, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, nil)
	if err == nil {
		t.Fatal("expected error for nil tournament")
	}
}

func TestPlanExecuteTournament_Execute_NilPool(t *testing.T) {
	tournament := &PlanExecuteTournament{}
	_, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, nil)
	if err == nil {
		t.Fatal("expected error for nil pool")
	}
}

func TestPlanExecuteTournament_Execute_NilExecuteFunc(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	_, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, nil)
	if err == nil {
		t.Fatal("expected error for nil ExecuteFunc")
	}
}

func TestPlanExecuteTournament_Execute_NoProvidersMatch(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	tournament.ExecuteFunc = func(ctx context.Context, prov ProviderConfig, p *BestPlan) (string, error) {
		return "x", nil
	}
	_, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, []string{"nonexistent"})
	if err == nil {
		t.Fatal("expected error for no matching providers")
	}
}

func TestPlanExecuteTournament_Plan_NilArbiter(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	tournament.Arbiter = nil
	tournament.PlanFunc = func(ctx context.Context, prov ProviderConfig, prompt string) (string, error) {
		return "plan", nil
	}
	best, err := tournament.Plan(context.Background(), "task", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best == nil || best.Plan != "plan" {
		t.Errorf("expected plan from default arbiter, got %v", best)
	}
}

func TestPlanExecuteTournament_Execute_NilArbiter(t *testing.T) {
	pool := NewProviderPool([]ProviderConfig{{Name: "a"}})
	tournament := NewPlanExecuteTournament(pool)
	tournament.Arbiter = nil
	tournament.ExecuteFunc = func(ctx context.Context, prov ProviderConfig, p *BestPlan) (string, error) {
		return "output", nil
	}
	best, err := tournament.Execute(context.Background(), &BestPlan{Plan: "x"}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if best == nil || best.Output != "output" {
		t.Errorf("expected output from default arbiter, got %v", best)
	}
}

// ---------------------------------------------------------------------------
// runOracle additional coverage — ForkFunc/RunFunc failure paths
// ---------------------------------------------------------------------------

func TestOracleTournament_ForkFuncFailure(t *testing.T) {
	forkFunc := func(srcSessionID string, turn int) (*session.Session, error) {
		return nil, errors.New("fork failed")
	}
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:         makeOracleRunFunc(map[string]*fakeProvider{"a": {output: "x", tokens: 1}, "b": {output: "y", tokens: 1}}),
		ForkFunc:        forkFunc,
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	result, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("expected ErrAllProvidersFailed, got: %v", err)
	}
	if result == nil || !result.AllFailed {
		t.Error("expected AllFailed=true")
	}
}

func TestOracleTournament_RunFuncFailure(t *testing.T) {
	runFunc := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		return nil, errors.New("run failed")
	}
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:         runFunc,
		ForkFunc:        makeForkFunc(),
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	result, err := tournament.Run(context.Background())
	if !errors.Is(err, ErrAllProvidersFailed) {
		t.Fatalf("expected ErrAllProvidersFailed, got: %v", err)
	}
	if result == nil || !result.AllFailed {
		t.Error("expected AllFailed=true")
	}
}

func TestOracleTournament_NilRunFunc(t *testing.T) {
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}},
		ForkFunc:        makeForkFunc(),
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       1,
	}
	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil RunFunc in oracle mode")
	}
}

func TestOracleTournament_NilForkFunc(t *testing.T) {
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}},
		RunFunc:         makeOracleRunFunc(map[string]*fakeProvider{"a": {output: "x", tokens: 1}}),
		ForkFunc:        nil,
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       1,
	}
	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error for nil ForkFunc in oracle mode")
	}
}

func TestOracleTournament_JudgeReturnsUnknownWinner(t *testing.T) {
	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
		"b": {name: "b", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:   makeOracleRunFunc(providers),
		ForkFunc:  makeForkFunc(),
		Mode:      ModeOracle,
		OracleJudge: func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
			scores := make(map[string]OracleScore)
			for _, c := range candidates {
				scores[c.Provider] = OracleScore{Correctness: 8, Completeness: 8, Risk: 2, Total: 16}
			}
			// Return a winner that doesn't match any candidate
			return OracleVerdict{WinnerProvider: "nonexistent", Scores: scores, Reasoning: "fallback"}, nil
		},
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected success (fallback winner), got: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected a winner via fallback")
	}
}

func TestOracleTournament_JudgeReturnsEmptyWinner(t *testing.T) {
	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100, },
	}
	tournament := &Tournament{
		Providers: []ProviderConfig{{Name: "a"}},
		RunFunc:   makeOracleRunFunc(providers),
		ForkFunc:  makeForkFunc(),
		Mode:      ModeOracle,
		OracleJudge: func(ctx context.Context, prompt string, candidates []Candidate) (OracleVerdict, error) {
			scores := make(map[string]OracleScore)
			for _, c := range candidates {
				scores[c.Provider] = OracleScore{Correctness: 5, Completeness: 5, Risk: 5, Total: 10}
			}
			// Empty winner — should trigger pickHighestScore fallback
			return OracleVerdict{WinnerProvider: "", Scores: scores, Reasoning: "empty"}, nil
		},
		MinQuorum:       1,
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner via fallback")
	}
	if result.Winner.Provider != "a" {
		t.Errorf("expected 'a', got %q", result.Winner.Provider)
	}
}

func TestOracleTournament_WithHooks(t *testing.T) {
	engine := hooks.New(nil)
	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
	}
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}},
		RunFunc:         makeOracleRunFunc(providers),
		ForkFunc:        makeForkFunc(),
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       1,
		Hooks:           engine,
		HookSessionID:   "hook-test",
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	_, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestOracleTournament_WithLedgerAndLessons(t *testing.T) {
	ledgerStore, err := ledger.Open(filepath.Join(t.TempDir(), "ledger.db"))
	if err != nil {
		t.Fatalf("open ledger: %v", err)
	}
	defer ledgerStore.Close()

	lessonsStore, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatalf("open lessons: %v", err)
	}
	defer lessonsStore.Close()

	providers := map[string]*fakeProvider{
		"a": {name: "a", delay: 10 * time.Millisecond, output: "CORRECT", tokens: 100},
		"b": {name: "b", delay: 10 * time.Millisecond, output: "wrong", tokens: 80},
	}
	tournament := &Tournament{
		Providers:       []ProviderConfig{{Name: "a"}, {Name: "b"}},
		RunFunc:         makeOracleRunFunc(providers),
		ForkFunc:        makeForkFunc(),
		Mode:            ModeOracle,
		OracleJudge:     fakeOracleJudge("CORRECT"),
		MinQuorum:       2,
		Ledger:          ledgerStore,
		Lessons:         lessonsStore,
		HookSessionID:   "oracle-ledger-test",
		Workspace:       "/test/ws",
		Prompt:          "do the thing",
		SourceSessionID: "src-1",
	}
	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected winner")
	}
}
