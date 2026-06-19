// SPDX-License-Identifier: MIT
package fusion

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"
)

func testMergeClient(baseURL, apiKey string) *llm.Client { return llm.NewClient(baseURL, apiKey) }

func TestPlanMergeJudge_MergeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"choices":[{"message":{"content":"## Unified Plan\n1. Create handler\n2. Add tests"}}]}`))
	}))
	defer server.Close()
	judge := NewLLMPlanMergeJudge(testMergeClient(server.URL, "test-key"), "test-model")
	merged, err := judge.Merge(context.Background(), "implement auth", []PlanCandidate{
		{Model: "a", Plan: "plan A"}, {Model: "b", Plan: "plan B"},
	})
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if merged == "" {
		t.Fatal("merged plan is empty")
	}
}

func TestPlanMergeJudge_NilClient(t *testing.T) {
	_, err := NewLLMPlanMergeJudge(nil, "").Merge(context.Background(), "test", []PlanCandidate{{Model: "a", Plan: "p"}})
	if err == nil {
		t.Fatal("expected error for nil client")
	}
}

func TestPlanMergeJudge_NoCandidates(t *testing.T) {
	_, err := NewLLMPlanMergeJudge(testMergeClient("http://localhost", "key"), "model").Merge(context.Background(), "test", nil)
	if err == nil {
		t.Fatal("expected error for no candidates")
	}
}

func TestRunPlanMerge_Success(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a", Model: "a", BaseURL: "http://localhost", APIKey: "k"},
			{Name: "b", Model: "b", BaseURL: "http://localhost", APIKey: "k"},
		},
		MinQuorum: 2, PerProviderTimeout: 5 * time.Second, Mode: ModePlanMerge, Prompt: "implement a function",
		ForkFunc: func(src string, turn int) (*session.Session, error) { return &session.Session{ID: "f"}, nil },
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			return &agentloop.Result{Summary: "plan: " + prov.Name}, nil
		},
		PlanMergeJudge: func(ctx context.Context, prompt string, plans []PlanCandidate) (string, error) { return "merged", nil },
		VerifyFn: func(ctx context.Context, ws string) verify.Result {
			return verify.Result{Passed: true, Mode: verify.ModePoC, Report: "ok"}
		},
	}
	result, err := tournament.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.MergedPlan == "" {
		t.Fatal("expected non-empty merged plan")
	}
	if len(result.Plans) != 2 {
		t.Errorf("expected 2 plans, got %d", len(result.Plans))
	}
}

func TestRunPlanMerge_VerifyFail(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a", Model: "a", BaseURL: "http://localhost", APIKey: "k"},
			{Name: "b", Model: "b", BaseURL: "http://localhost", APIKey: "k"},
		},
		MinQuorum: 2, Mode: ModePlanMerge, Prompt: "test",
		ForkFunc: func(src string, turn int) (*session.Session, error) { return &session.Session{ID: "f"}, nil },
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			return &agentloop.Result{Summary: "done"}, nil
		},
		PlanMergeJudge: func(ctx context.Context, prompt string, plans []PlanCandidate) (string, error) { return "merged", nil },
		VerifyFn: func(ctx context.Context, ws string) verify.Result {
			return verify.Result{Passed: false, Mode: verify.ModePoC, Report: "fail"}
		},
	}
	result, _ := tournament.Run(context.Background())
	if result.Success {
		t.Fatal("expected failure")
	}
}

func TestRunPlanMerge_JudgeFail(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a", Model: "a", BaseURL: "http://localhost", APIKey: "k"},
			{Name: "b", Model: "b", BaseURL: "http://localhost", APIKey: "k"},
		},
		MinQuorum: 2, Mode: ModePlanMerge,
		ForkFunc: func(src string, turn int) (*session.Session, error) { return &session.Session{ID: "f"}, nil },
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			return &agentloop.Result{Summary: "plan"}, nil
		},
		PlanMergeJudge: func(ctx context.Context, prompt string, plans []PlanCandidate) (string, error) {
			return "", errors.New("judge unavailable")
		},
		VerifyFn: func(ctx context.Context, ws string) verify.Result { return verify.Result{Passed: true} },
	}
	result, _ := tournament.Run(context.Background())
	if result.Success {
		t.Fatal("expected failure when judge fails")
	}
}

func TestRunPlanMerge_AllPlannersFail(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a", Model: "a", BaseURL: "http://localhost", APIKey: "k"},
			{Name: "b", Model: "b", BaseURL: "http://localhost", APIKey: "k"},
		},
		MinQuorum: 2, Mode: ModePlanMerge,
		ForkFunc: func(src string, turn int) (*session.Session, error) { return &session.Session{ID: "f"}, nil },
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			return nil, errors.New("model error")
		},
		PlanMergeJudge: func(ctx context.Context, prompt string, plans []PlanCandidate) (string, error) { return "merged", nil },
		VerifyFn:       func(ctx context.Context, ws string) verify.Result { return verify.Result{Passed: true} },
	}
	result, _ := tournament.Run(context.Background())
	if result.Success {
		t.Fatal("expected failure")
	}
}

func TestRunPlanMerge_NoJudgeWired(t *testing.T) {
	tournament := &Tournament{
		Providers: []ProviderConfig{
			{Name: "a", Model: "a", BaseURL: "http://localhost", APIKey: "k"},
			{Name: "b", Model: "b", BaseURL: "http://localhost", APIKey: "k"},
		},
		MinQuorum: 2, Mode: ModePlanMerge,
		ForkFunc: func(src string, turn int) (*session.Session, error) { return &session.Session{ID: "f"}, nil },
		RunFunc: func(ctx context.Context, prov ProviderConfig, sess *session.Session, prompt string) (*agentloop.Result, error) {
			return &agentloop.Result{Summary: "plan"}, nil
		},
		VerifyFn: func(ctx context.Context, ws string) verify.Result { return verify.Result{Passed: true} },
	}
	_, err := tournament.Run(context.Background())
	if err == nil {
		t.Fatal("expected error when PlanMergeJudge not wired")
	}
}

func TestSortPlanCandidates(t *testing.T) {
	plans := []PlanCandidate{{Model: "zeta"}, {Model: "alpha"}, {Model: "mid"}}
	sortPlanCandidates(plans)
	if plans[0].Model != "alpha" || plans[2].Model != "zeta" {
		t.Errorf("expected alphabetical sort")
	}
}

func TestBuildPlanMergePrompt(t *testing.T) {
	prompt := buildPlanMergePrompt("implement auth", []PlanCandidate{{Model: "a", Plan: "plan A"}, {Model: "b", Plan: "plan B"}})
	if !strings.Contains(prompt, "implement auth") {
		t.Error("prompt missing task")
	}
	if !strings.Contains(prompt, "Plan 1") || !strings.Contains(prompt, "Plan 2") {
		t.Error("prompt missing plans")
	}
}
