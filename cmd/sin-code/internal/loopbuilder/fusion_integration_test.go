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
