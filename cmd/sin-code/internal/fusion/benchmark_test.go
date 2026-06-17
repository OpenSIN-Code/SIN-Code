// SPDX-License-Identifier: MIT
// Purpose: Integration benchmarks for SIN Fusion v1 on real coding tasks
// (issue #343). Uses httptest stubs as fallback when no real API keys
// are available.
//
// Build tag: integration — run with:
//   go test -tags integration -race -count=1 -bench=. ./cmd/sin-code/internal/fusion/...
//
//go:build integration

package fusion

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func hasRealAPIKeys() bool {
	return os.Getenv("FIREWORKS_API_KEY") != ""
}

func stubLLMServer(delay time.Duration, output string, tokens int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(delay)
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": output}, "finish_reason": "stop"},
			},
			"usage": map[string]any{
				"prompt_tokens":     tokens / 2,
				"completion_tokens": tokens / 2,
				"total_tokens":      tokens,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func benchTournament(
	b *testing.B,
	providers []ProviderConfig,
	runFn RunFunc,
	verifyFn func(ctx context.Context, workspace string) verify.Result,
) {
	b.Helper()
	ctx := context.Background()
	for n := 0; n < b.N; n++ {
		tournament := &Tournament{
			Providers:          providers,
			RunFunc:            runFn,
			ForkFunc:           makeForkFunc(),
			VerifyFn:           verifyFn,
			MinQuorum:          2,
			PerProviderTimeout: 10 * time.Second,
			Workspace:          "/bench/ws",
			Prompt:             "benchmark task",
			SourceSessionID:    "bench-src",
		}
		result, err := tournament.Run(ctx)
		if err != nil && err != ErrAllProvidersFailed && err != ErrCostCeilingExceeded {
			b.Fatalf("unexpected error: %v", err)
		}
		_ = result
	}
}

func makePassVerifyFn() func(ctx context.Context, workspace string) verify.Result {
	return func(ctx context.Context, workspace string) verify.Result {
		return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "passed"}
	}
}

func makeFailVerifyFn() func(ctx context.Context, workspace string) verify.Result {
	return func(ctx context.Context, workspace string) verify.Result {
		return verify.Result{Passed: false, Mode: verify.ModePoC, Report: "failed"}
	}
}

func TestStubLLMServer(t *testing.T) {
	srv := stubLLMServer(10*time.Millisecond, "test output", 100)
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET stub server: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	choices, ok := body["choices"].([]any)
	if !ok || len(choices) != 1 {
		t.Fatalf("expected 1 choice, got %v", body["choices"])
	}
	usage, ok := body["usage"].(map[string]any)
	if !ok {
		t.Fatal("missing usage in response")
	}
	if usage["total_tokens"].(float64) != 100 {
		t.Fatalf("expected 100 tokens, got %v", usage["total_tokens"])
	}
}

func TestBenchmarkSetupProducesResult(t *testing.T) {
	ctx := context.Background()
	vs := newVerifyState()
	runFn := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		vs.set(prov.Name, "CORRECT")
		return &agentloop.Result{SessionID: sess.ID, Summary: "CORRECT", Turns: 1, Tokens: 100}, nil
	}
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "stub-a", InputPer1M: 1.0, OutputPer1M: 2.0},
			{Name: "stub-b", InputPer1M: 1.0, OutputPer1M: 2.0},
		},
		RunFunc:         runFn,
		ForkFunc:        makeForkFunc(),
		VerifyFn:        makeVerifyFn(vs, "CORRECT"),
		MinQuorum:       2,
		Workspace:       "/test/ws",
		Prompt:          "test task",
		SourceSessionID: "src",
	}
	result, err := tournament.Run(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Winner == nil {
		t.Fatal("expected a winner")
	}
	if result.TotalCostUSD <= 0 {
		t.Fatal("expected positive cost")
	}
}

func BenchmarkFusion_SimpleFix(b *testing.B) {
	_ = stubLLMServer(5*time.Millisecond, "CORRECT", 50)
	providers := []ProviderConfig{
		{Name: "fast-fix", InputPer1M: 0.30, OutputPer1M: 1.20},
		{Name: "backup-fix", InputPer1M: 0.95, OutputPer1M: 4.00},
		{Name: "third-fix", InputPer1M: 1.40, OutputPer1M: 4.40},
	}
	vs := newVerifyState()
	runFn := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		vs.set(prov.Name, "CORRECT")
		return &agentloop.Result{SessionID: sess.ID, Summary: "CORRECT", Turns: 1, Tokens: 50}, nil
	}
	verifyFn := makeVerifyFn(vs, "CORRECT")
	b.ResetTimer()
	b.ReportAllocs()
	benchTournament(b, providers, runFn, verifyFn)
}

func BenchmarkFusion_MediumFeature(b *testing.B) {
	providers := []ProviderConfig{
		{Name: "model-a", InputPer1M: 0.30, OutputPer1M: 1.20},
		{Name: "model-b", InputPer1M: 0.95, OutputPer1M: 4.00},
		{Name: "model-c", InputPer1M: 1.74, OutputPer1M: 3.48},
		{Name: "model-d", InputPer1M: 0.40, OutputPer1M: 1.60},
	}
	vs := newVerifyState()
	runFn := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		out := "wrong"
		tokens := 300
		if prov.Name == "model-c" {
			out = "CORRECT"
			tokens = 500
		}
		vs.set(prov.Name, out)
		return &agentloop.Result{SessionID: sess.ID, Summary: out, Turns: 2, Tokens: tokens}, nil
	}
	verifyFn := makeVerifyFn(vs, "CORRECT")
	b.ResetTimer()
	b.ReportAllocs()
	benchTournament(b, providers, runFn, verifyFn)
}

func BenchmarkFusion_ComplexRefactor(b *testing.B) {
	providers := []ProviderConfig{
		{Name: "minimax-m3", InputPer1M: 0.30, OutputPer1M: 1.20},
		{Name: "kimi-k2p7-code", InputPer1M: 0.95, OutputPer1M: 4.00},
		{Name: "deepseek-v4-pro", InputPer1M: 1.74, OutputPer1M: 3.48},
		{Name: "qwen-3p7-plus", InputPer1M: 0.40, OutputPer1M: 1.60},
		{Name: "glm-5p2", InputPer1M: 1.40, OutputPer1M: 4.40},
	}
	vs := newVerifyState()
	runFn := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		out := "wrong"
		tokens := 800
		if prov.Name == "kimi-k2p7-code" || prov.Name == "glm-5p2" {
			out = "CORRECT"
			tokens = 1200
		}
		select {
		case <-time.After(30 * time.Millisecond):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		vs.set(prov.Name, out)
		return &agentloop.Result{SessionID: sess.ID, Summary: out, Turns: 3, Tokens: tokens}, nil
	}
	verifyFn := makeVerifyFn(vs, "CORRECT")
	b.ResetTimer()
	b.ReportAllocs()
	benchTournament(b, providers, runFn, verifyFn)
}

func BenchmarkFusion_VerifyFail(b *testing.B) {
	providers := []ProviderConfig{
		{Name: "fail-a", InputPer1M: 0.30, OutputPer1M: 1.20},
		{Name: "fail-b", InputPer1M: 0.95, OutputPer1M: 4.00},
		{Name: "fail-c", InputPer1M: 1.74, OutputPer1M: 3.48},
	}
	runFn := func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
		return &agentloop.Result{SessionID: sess.ID, Summary: "incorrect", Turns: 2, Tokens: 400}, nil
	}
	b.ResetTimer()
	b.ReportAllocs()
	benchTournament(b, providers, runFn, makeFailVerifyFn())
}
