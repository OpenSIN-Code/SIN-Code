// SPDX-License-Identifier: MIT
// Purpose: integration test exercising the full path: loopbuilder.Build →
// WireFusion → fusionAdapter → fusion.Tournament end-to-end (issue #290).
// All tests must pass under `go test -race -count=1` (mandate M7).
package loopbuilder

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/fusion"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func TestFusionIntegration_BuildWiresTournamentRunner(t *testing.T) {
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace:        t.TempDir(),
		MaxTurns:         5,
		VerifyMode:       "poc",
		SkipMCP:          true,
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if loop.TournamentRunner == nil {
		t.Fatal("expected TournamentRunner to be non-nil when fusion is enabled")
	}
}

func TestFusionIntegration_RunReturnsErrorWithoutSessionStore(t *testing.T) {
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace:        t.TempDir(),
		MaxTurns:         5,
		VerifyMode:       "poc",
		SkipMCP:          true,
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if loop.TournamentRunner == nil {
		t.Fatal("expected TournamentRunner to be non-nil")
	}

	_, _, err = loop.TournamentRunner.Run(context.Background(), "test prompt")
	if err == nil {
		t.Fatal("expected error when ForkFunc/RunFunc are nil (no SessionStore)")
	}
	if !strings.Contains(err.Error(), "not fully wired") {
		t.Fatalf("expected 'not fully wired' error, got: %v", err)
	}
}

func TestFusionIntegration_BuildWithSessionStoreWiresForkRun(t *testing.T) {
	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	loop, cleanup, err := Build(context.Background(), Config{
		Workspace:        t.TempDir(),
		MaxTurns:         5,
		VerifyMode:       "poc",
		SkipMCP:          true,
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		SessionStore:     store,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if loop.TournamentRunner == nil {
		t.Fatal("expected TournamentRunner to be non-nil")
	}

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.t.ForkFunc == nil {
		t.Error("expected ForkFunc to be non-nil when SessionStore is provided")
	}
	if adapter.t.RunFunc == nil {
		t.Error("expected RunFunc to be non-nil when SessionStore is provided")
	}
}

func TestFusionIntegration_WireFusionDirectly(t *testing.T) {
	loop := &agentloop.Loop{}
	gate := verify.NewGate("poc", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
	}

	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	if loop.TournamentRunner == nil {
		t.Fatal("expected TournamentRunner to be non-nil after WireFusion")
	}
}

func TestFusionIntegration_WireFusionNoopWhenDisabled(t *testing.T) {
	loop := &agentloop.Loop{}
	gate := verify.NewGate("poc", nil, nil)
	cfg := Config{FusionEnabled: false}

	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	if loop.TournamentRunner != nil {
		t.Fatal("expected TournamentRunner to be nil when fusion is disabled")
	}
}

func TestFusionIntegration_WireFusionNoopWhenGateOff(t *testing.T) {
	loop := &agentloop.Loop{}
	gate := verify.NewGate("off", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
	}

	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	if loop.TournamentRunner != nil {
		t.Fatal("expected TournamentRunner to be nil when gate mode is off")
	}
}

func TestFusionIntegration_WireFusionOracleMode(t *testing.T) {
	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
	}

	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	if loop.TournamentRunner == nil {
		t.Fatal("expected TournamentRunner to be non-nil in oracle mode")
	}
	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.t.Mode != fusion.ModeOracle {
		t.Errorf("expected oracle mode, got %q", adapter.t.Mode)
	}
	if adapter.t.OracleJudge == nil {
		t.Error("expected OracleJudge to be wired in oracle mode")
	}
	if adapter.t.MaxCostUSD != 2.0 {
		t.Errorf("expected oracle max cost capped at 2.0, got %v", adapter.t.MaxCostUSD)
	}
}

func TestFusionIntegration_TournamentEndToEndAllFail(t *testing.T) {
	mockRunFunc := func(ctx context.Context, prov fusion.ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   "wrong answer",
			Turns:     1,
			Tokens:    100,
		}, nil
	}
	mockForkFunc := func(srcSessionID string, turn int) (*session.Session, error) {
		return &session.Session{ID: "fork-session"}, nil
	}
	mockVerifyFn := func(ctx context.Context, workspace string) verify.Result {
		return verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error: undefined variable"}
	}

	tournament := &fusion.Tournament{
		Providers:          []fusion.ProviderConfig{{Name: "mock-a"}, {Name: "mock-b"}},
		RunFunc:            mockRunFunc,
		ForkFunc:           mockForkFunc,
		VerifyFn:           mockVerifyFn,
		MinQuorum:          2,
		MaxCostUSD:         10.0,
		PerProviderTimeout: 5 * time.Second,
		Workspace:          t.TempDir(),
		Prompt:             "test prompt",
		SourceSessionID:    "src-1",
	}

	adapter := &fusionAdapter{t: tournament}
	_, _, err := adapter.Run(context.Background(), "test prompt")
	if !errors.Is(err, fusion.ErrAllProvidersFailed) {
		t.Fatalf("expected ErrAllProvidersFailed, got: %v", err)
	}
}

func TestFusionIntegration_TournamentEndToEndWinner(t *testing.T) {
	mockRunFunc := func(ctx context.Context, prov fusion.ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   "CORRECT",
			Turns:     1,
			Tokens:    100,
		}, nil
	}
	mockForkFunc := func(srcSessionID string, turn int) (*session.Session, error) {
		return &session.Session{ID: "fork-session"}, nil
	}
	mockVerifyFn := func(ctx context.Context, workspace string) verify.Result {
		return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed: workspace correct"}
	}

	tournament := &fusion.Tournament{
		Providers:          []fusion.ProviderConfig{{Name: "mock-a"}, {Name: "mock-b"}},
		RunFunc:            mockRunFunc,
		ForkFunc:           mockForkFunc,
		VerifyFn:           mockVerifyFn,
		MinQuorum:          2,
		MaxCostUSD:         10.0,
		PerProviderTimeout: 5 * time.Second,
		Workspace:          t.TempDir(),
		Prompt:             "test prompt",
		SourceSessionID:    "src-1",
	}

	adapter := &fusionAdapter{t: tournament}
	output, tokens, err := adapter.Run(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output != "CORRECT" {
		t.Errorf("expected output 'CORRECT', got %q", output)
	}
	if tokens != 100 {
		t.Errorf("expected 100 tokens, got %d", tokens)
	}
}

func TestFusionIntegration_ShouldRun(t *testing.T) {
	// Default: FusionDifficultyGate=true → text heuristic filters stylistic failures.
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}

	structuralFail := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error: undefined variable"}
	if !adapter.ShouldRun(structuralFail) {
		t.Error("expected ShouldRun=true for structural failure")
	}

	passed := verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"}
	if adapter.ShouldRun(passed) {
		t.Error("expected ShouldRun=false for passed result")
	}

	stylisticFail := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "naming convention violation"}
	if adapter.ShouldRun(stylisticFail) {
		t.Error("expected ShouldRun=false for stylistic failure when difficulty gate is on")
	}
}

func TestFusionIntegration_ShouldRunDifficultyGateOff(t *testing.T) {
	// FusionDifficultyGate=false → ALL failures trigger tournament (no filtering).
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: false}}

	structuralFail := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error: undefined variable"}
	if !adapter.ShouldRun(structuralFail) {
		t.Error("expected ShouldRun=true for structural failure (gate off)")
	}

	stylisticFail := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "naming convention violation"}
	if !adapter.ShouldRun(stylisticFail) {
		t.Error("expected ShouldRun=true for stylistic failure when difficulty gate is off")
	}

	passed := verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"}
	if adapter.ShouldRun(passed) {
		t.Error("expected ShouldRun=false for passed result even with gate off")
	}
}

func TestFusionIntegration_RunSetsPromptAndSourceSessionID(t *testing.T) {
	var capturedPrompt string
	var capturedSourceSessionID string

	mockRunFunc := func(ctx context.Context, prov fusion.ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		capturedPrompt = prompt
		return &agentloop.Result{
			SessionID: sess.ID,
			Summary:   "CORRECT",
			Turns:     1,
			Tokens:    100,
		}, nil
	}
	mockForkFunc := func(srcSessionID string, turn int) (*session.Session, error) {
		capturedSourceSessionID = srcSessionID
		return &session.Session{ID: "fork-session"}, nil
	}
	mockVerifyFn := func(ctx context.Context, workspace string) verify.Result {
		return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"}
	}

	tournament := &fusion.Tournament{
		Providers:          []fusion.ProviderConfig{{Name: "mock-a"}, {Name: "mock-b"}},
		RunFunc:            mockRunFunc,
		ForkFunc:           mockForkFunc,
		VerifyFn:           mockVerifyFn,
		MinQuorum:          2,
		MaxCostUSD:         10.0,
		PerProviderTimeout: 5 * time.Second,
		Workspace:          t.TempDir(),
	}

	adapter := &fusionAdapter{
		t:   tournament,
		cfg: Config{SessionID: "source-session-42"},
	}

	output, _, err := adapter.Run(context.Background(), "fix the bug in auth.go")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if output != "CORRECT" {
		t.Errorf("expected output 'CORRECT', got %q", output)
	}
	if capturedPrompt != "fix the bug in auth.go" {
		t.Errorf("expected prompt propagated to RunFunc, got %q", capturedPrompt)
	}
	if capturedSourceSessionID != "source-session-42" {
		t.Errorf("expected SourceSessionID propagated to ForkFunc, got %q", capturedSourceSessionID)
	}
	if tournament.Prompt != "fix the bug in auth.go" {
		t.Errorf("expected tournament.Prompt set, got %q", tournament.Prompt)
	}
	if tournament.SourceSessionID != "source-session-42" {
		t.Errorf("expected tournament.SourceSessionID set, got %q", tournament.SourceSessionID)
	}
}

func TestFusionIntegration_WireFusionSetsSourceSessionID(t *testing.T) {
	loop := &agentloop.Loop{}
	gate := verify.NewGate("poc", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		SessionID:        "test-session-99",
	}

	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.t.SourceSessionID != "test-session-99" {
		t.Errorf("expected SourceSessionID 'test-session-99', got %q", adapter.t.SourceSessionID)
	}
}

// TestWireFusion_OracleJudgePrefersEvaluatorModel verifies that when
// SIN_EVALUATOR_MODEL is set, the oracle judge uses it instead of the
// worker model — avoiding self-preference bias (mirrors stop-gate pattern).
func TestWireFusion_OracleJudgePrefersEvaluatorModel(t *testing.T) {
	t.Setenv("SIN_EVALUATOR_MODEL", "gpt-4o-judge")
	t.Setenv("SIN_EVALUATOR_BASE_URL", "") // ensure no separate client

	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		Model:            "worker-model-xyz",
	}
	client := llm.NewClient("http://worker-localhost", "worker-key")

	WireFusion(loop, cfg, gate, client, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.oracleJudge == nil {
		t.Fatal("expected oracleJudge to be non-nil in oracle mode")
	}
	if adapter.oracleJudge.ModelName != "gpt-4o-judge" {
		t.Errorf("expected judge model 'gpt-4o-judge' (from SIN_EVALUATOR_MODEL), got %q",
			adapter.oracleJudge.ModelName)
	}
	if adapter.oracleJudge.ModelName == cfg.Model {
		t.Error("judge model should NOT equal worker model when SIN_EVALUATOR_MODEL is set")
	}
}

// TestWireFusion_OracleJudgeFallsBackToWorkerModel verifies that when
// SIN_EVALUATOR_MODEL is NOT set, the oracle judge falls back to the
// worker model (cfg.Model).
func TestWireFusion_OracleJudgeFallsBackToWorkerModel(t *testing.T) {
	t.Setenv("SIN_EVALUATOR_MODEL", "")   // not set
	t.Setenv("SIN_EVALUATOR_BASE_URL", "") // no separate client

	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		Model:            "worker-model-fallback",
	}
	client := llm.NewClient("http://worker-localhost", "worker-key")

	WireFusion(loop, cfg, gate, client, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.oracleJudge == nil {
		t.Fatal("expected oracleJudge to be non-nil in oracle mode")
	}
	if adapter.oracleJudge.ModelName != "worker-model-fallback" {
		t.Errorf("expected judge model 'worker-model-fallback' (fallback to cfg.Model), got %q",
			adapter.oracleJudge.ModelName)
	}
	// Judge should reuse the worker client since no evaluator base URL was set.
	if adapter.oracleJudge.Client != client {
		t.Error("expected judge to reuse worker client when SIN_EVALUATOR_BASE_URL is not set")
	}
}

// TestWireFusion_OracleJudgeSeparateClient verifies that when
// SIN_EVALUATOR_BASE_URL is set, a separate LLM client is created for
// the judge (different BaseURL and API key from the worker client).
func TestWireFusion_OracleJudgeSeparateClient(t *testing.T) {
	t.Setenv("SIN_EVALUATOR_MODEL", "evaluator-model")
	t.Setenv("SIN_EVALUATOR_BASE_URL", "http://evaluator-endpoint:8080")
	t.Setenv("SIN_EVALUATOR_API_KEY", "evaluator-secret-key")

	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		Model:            "worker-model-xyz",
	}
	workerClient := llm.NewClient("http://worker-localhost", "worker-key")

	WireFusion(loop, cfg, gate, workerClient, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.oracleJudge == nil {
		t.Fatal("expected oracleJudge to be non-nil in oracle mode")
	}
	if adapter.oracleJudge.ModelName != "evaluator-model" {
		t.Errorf("expected judge model 'evaluator-model', got %q", adapter.oracleJudge.ModelName)
	}
	if adapter.oracleJudge.Client == workerClient {
		t.Fatal("expected a SEPARATE client for the judge, but got the same worker client")
	}
	if adapter.oracleJudge.Client.BaseURL != "http://evaluator-endpoint:8080" {
		t.Errorf("expected judge client BaseURL 'http://evaluator-endpoint:8080', got %q",
			adapter.oracleJudge.Client.BaseURL)
	}
	if adapter.oracleJudge.Client.APIKey != "evaluator-secret-key" {
		t.Errorf("expected judge client APIKey 'evaluator-secret-key', got %q",
			adapter.oracleJudge.Client.APIKey)
	}
}

// TestWireFusion_OracleJudgeSeparateClientFallsBackToWorkerKey verifies
// that when SIN_EVALUATOR_BASE_URL is set but SIN_EVALUATOR_API_KEY is
// NOT set, the separate client reuses the worker client's API key.
func TestWireFusion_OracleJudgeSeparateClientFallsBackToWorkerKey(t *testing.T) {
	t.Setenv("SIN_EVALUATOR_MODEL", "evaluator-model")
	t.Setenv("SIN_EVALUATOR_BASE_URL", "http://evaluator-endpoint:8080")
	t.Setenv("SIN_EVALUATOR_API_KEY", "") // not set — should fall back to worker key

	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		Model:            "worker-model-xyz",
	}
	workerClient := llm.NewClient("http://worker-localhost", "worker-original-key")

	WireFusion(loop, cfg, gate, workerClient, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.oracleJudge == nil {
		t.Fatal("expected oracleJudge to be non-nil in oracle mode")
	}
	if adapter.oracleJudge.Client.BaseURL != "http://evaluator-endpoint:8080" {
		t.Errorf("expected judge client BaseURL 'http://evaluator-endpoint:8080', got %q",
			adapter.oracleJudge.Client.BaseURL)
	}
	if adapter.oracleJudge.Client.APIKey != "worker-original-key" {
		t.Errorf("expected judge client to fall back to worker key 'worker-original-key', got %q",
			adapter.oracleJudge.Client.APIKey)
	}
}

// TestWireFusion_OracleJudgeNilClientNoPanic verifies that WireFusion does
// not panic when oracle mode is active and the client is nil (common in
// unit tests that don't wire a real LLM provider).
func TestWireFusion_OracleJudgeNilClientNoPanic(t *testing.T) {
	t.Setenv("SIN_EVALUATOR_MODEL", "")
	t.Setenv("SIN_EVALUATOR_BASE_URL", "http://eval-endpoint")
	t.Setenv("SIN_EVALUATOR_API_KEY", "eval-key")

	loop := &agentloop.Loop{}
	gate := verify.NewGate("oracle", nil, nil)
	cfg := Config{
		FusionEnabled:    true,
		FusionOracleMode: true,
		FusionProviders:  []string{"minimax-m3", "glm-5p2"},
		FusionMaxCostUSD: 10.0,
		FusionMinQuorum:  2,
		Model:            "worker-model",
	}

	// Should not panic even though client is nil.
	WireFusion(loop, cfg, gate, nil, nil, nil, nil)

	adapter, ok := loop.TournamentRunner.(*fusionAdapter)
	if !ok {
		t.Fatalf("expected *fusionAdapter, got %T", loop.TournamentRunner)
	}
	if adapter.oracleJudge == nil {
		t.Fatal("expected oracleJudge to be non-nil even with nil worker client")
	}
	// With SIN_EVALUATOR_BASE_URL set, a separate client is created.
	if adapter.oracleJudge.Client == nil {
		t.Fatal("expected judge client to be non-nil when SIN_EVALUATOR_BASE_URL is set")
	}
	if adapter.oracleJudge.Client.BaseURL != "http://eval-endpoint" {
		t.Errorf("expected judge client BaseURL 'http://eval-endpoint', got %q",
			adapter.oracleJudge.Client.BaseURL)
	}
	if adapter.oracleJudge.Client.APIKey != "eval-key" {
		t.Errorf("expected judge client APIKey 'eval-key', got %q",
			adapter.oracleJudge.Client.APIKey)
	}
}

// --- ShouldRunWithConfidence tests (issue #290 difficulty gate) ---

func TestFusionAdapter_ShouldRunWithConfidence_HighConfidenceStylistic_NoTournament(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "naming convention violation"}
	got := adapter.ShouldRunWithConfidence(vr, 0.8, 1)
	if got {
		t.Error("expected false for high-confidence stylistic miss (legacy retry)")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_LowConfidence_Tournament(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "ambiguous failure"}
	got := adapter.ShouldRunWithConfidence(vr, 0.2, 1)
	if !got {
		t.Error("expected true for low confidence (structural, tournament)")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_HighAttemptCount_Tournament(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "unclear failure"}
	got := adapter.ShouldRunWithConfidence(vr, 0.5, 5)
	if !got {
		t.Error("expected true for attemptCount=5 (repeated failures suggest structural)")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_BackwardCompat_FallsBackToText(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}

	// confidence=0, attemptCount=0 → text heuristic fallback.
	// Structural report → true.
	structuralVR := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "compile error: undefined x"}
	if !adapter.ShouldRunWithConfidence(structuralVR, 0, 0) {
		t.Error("expected true for structural report via text fallback")
	}

	// Stylistic report → false.
	stylisticVR := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "naming convention violation"}
	if adapter.ShouldRunWithConfidence(stylisticVR, 0, 0) {
		t.Error("expected false for stylistic report via text fallback")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_DifficultyGateOff(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: false}}
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "style: cosmetic issue"}
	got := adapter.ShouldRunWithConfidence(vr, 0.9, 1)
	if !got {
		t.Error("expected true for any failure when difficulty gate is off")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_PassedResult_NoTournament(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}
	vr := verify.Result{Passed: true, Mode: verify.ModePoC, Report: "all good"}
	got := adapter.ShouldRunWithConfidence(vr, 0.1, 10)
	if got {
		t.Error("expected false for passed result regardless of confidence/attempts")
	}
}

func TestFusionAdapter_ShouldRunWithConfidence_OracleMode_ClassifiedStylistic(t *testing.T) {
	adapter := &fusionAdapter{t: &fusion.Tournament{}, cfg: Config{FusionDifficultyGate: true}}
	// Oracle failures are classified as "stylistic" → high confidence means retry.
	vr := verify.Result{Passed: false, Mode: verify.ModeOracle, Report: "output doesn't match expected style"}
	got := adapter.ShouldRunWithConfidence(vr, 0.8, 1)
	if got {
		t.Error("expected false for high-confidence oracle failure (classified as stylistic)")
	}
}

// --- classifyError tests ---

func TestClassifyError_ModePoC_Compile(t *testing.T) {
	tests := []string{
		"compile error: undefined variable",
		"build failed: exit code 1",
		"syntax error: unexpected token",
		"parse error: invalid syntax",
		"type mismatch: cannot use int as string",
		"cannot find package foo",
	}
	for _, report := range tests {
		vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: report}
		if got := classifyError(vr); got != "compile" {
			t.Errorf("classifyError(%q) = %q, want \"compile\"", report, got)
		}
	}
}

func TestClassifyError_ModePoC_Runtime(t *testing.T) {
	tests := []string{
		"panic: runtime error: index out of range",
		"segfault at 0x7fff",
		"nil pointer dereference",
	}
	for _, report := range tests {
		vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: report}
		if got := classifyError(vr); got != "runtime" {
			t.Errorf("classifyError(%q) = %q, want \"runtime\"", report, got)
		}
	}
}

func TestClassifyError_ModePoC_Test(t *testing.T) {
	tests := []string{
		"test fail: TestFoo failed",
		"tests failed: 3 of 10",
		"test: fail in package bar",
	}
	for _, report := range tests {
		vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: report}
		if got := classifyError(vr); got != "test" {
			t.Errorf("classifyError(%q) = %q, want \"test\"", report, got)
		}
	}
}

func TestClassifyError_ModePoC_Stylistic(t *testing.T) {
	tests := []string{
		"style: naming convention violation",
		"format: incorrect indentation",
		"documentation missing for exported function",
		"cosmetic: whitespace inconsistency",
	}
	for _, report := range tests {
		vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: report}
		if got := classifyError(vr); got != "stylistic" {
			t.Errorf("classifyError(%q) = %q, want \"stylistic\"", report, got)
		}
	}
}

func TestClassifyError_ModePoC_Unknown(t *testing.T) {
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "something went wrong"}
	if got := classifyError(vr); got != "" {
		t.Errorf("classifyError(\"something went wrong\") = %q, want \"\"", got)
	}
}

func TestClassifyError_ModeOracle_AlwaysStylistic(t *testing.T) {
	vr := verify.Result{Passed: false, Mode: verify.ModeOracle, Report: "compile error: undefined"}
	if got := classifyError(vr); got != "stylistic" {
		t.Errorf("classifyError with ModeOracle = %q, want \"stylistic\"", got)
	}
}

func TestClassifyError_CaseInsensitive(t *testing.T) {
	vr := verify.Result{Passed: false, Mode: verify.ModePoC, Report: "COMPILE ERROR: undefined"}
	if got := classifyError(vr); got != "compile" {
		t.Errorf("classifyError(\"COMPILE ERROR...\") = %q, want \"compile\"", got)
	}
}
