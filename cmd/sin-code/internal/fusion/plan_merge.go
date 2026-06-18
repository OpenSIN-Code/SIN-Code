// SPDX-License-Identifier: MIT
package fusion

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

const ModePlanMerge Mode = "plan-merge"

type PlanMergeJudgeFn func(ctx context.Context, prompt string, plans []PlanCandidate) (string, error)

type LLMPlanMergeJudge struct {
	Client    *llm.Client
	ModelName string
}

func NewLLMPlanMergeJudge(client *llm.Client, modelName string) *LLMPlanMergeJudge {
	return &LLMPlanMergeJudge{Client: client, ModelName: modelName}
}

func (j *LLMPlanMergeJudge) Merge(ctx context.Context, prompt string, plans []PlanCandidate) (string, error) {
	if j.Client == nil || j.Client.BaseURL == "" || j.Client.APIKey == "" || j.ModelName == "" {
		return "", errors.New("fusion: plan-merge judge not configured")
	}
	if len(plans) == 0 {
		return "", errors.New("fusion: no plan candidates to merge")
	}
	systemPrompt := `You are a senior software architect. Merge multiple AI plans into ONE superior plan. Include unique insights from each candidate. Resolve conflicts by choosing the simpler approach. Output ONLY the merged plan as Markdown.`
	userPrompt := buildPlanMergePrompt(prompt, plans)
	resp, err := j.Client.Chat(ctx, llm.ChatRequest{
		Model:       j.ModelName,
		Messages:    []llm.Message{{Role: "system", Content: systemPrompt}, {Role: "user", Content: userPrompt}},
		MaxTokens:   4096,
		Temperature: 0.0,
	})
	if err != nil {
		return "", fmt.Errorf("fusion: plan-merge judge LLM call failed: %w", err)
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == "" {
		return "", errors.New("fusion: plan-merge judge returned empty response")
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content), nil
}

func buildPlanMergePrompt(prompt string, plans []PlanCandidate) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Task:\n%s\n\n%d candidate plans:\n\n", prompt, len(plans))
	for i, p := range plans {
		fmt.Fprintf(&b, "--- Plan %d (from %s) ---\n%s\n\n", i+1, p.Model, p.Plan)
	}
	b.WriteString("Merge these into a single superior plan. Output ONLY the merged plan.\n")
	return b.String()
}

func (t *Tournament) runPlanMerge(ctx context.Context) (*Result, error) {
	start := time.Now()
	if len(t.Providers) < 2 {
		return nil, ErrInsufficientQuorum
	}
	if t.RunFunc == nil {
		return nil, fmt.Errorf("fusion: RunFunc not wired")
	}
	if t.ForkFunc == nil {
		return nil, fmt.Errorf("fusion: ForkFunc not wired")
	}
	if t.PlanMergeJudge == nil {
		return nil, fmt.Errorf("fusion: PlanMergeJudge not wired")
	}
	t.fireHook(ctx, "fusion.dispatch", map[string]any{"providers": len(t.Providers), "mode": "plan-merge"})
	planTimeout := t.PerProviderTimeout
	if planTimeout <= 0 {
		planTimeout = 120 * time.Second
	}
	type planResult struct {
		provider ProviderConfig
		plan     string
		err      error
	}
	planChan := make(chan planResult, len(t.Providers))
	var planWg sync.WaitGroup
	for _, prov := range t.Providers {
		planWg.Add(1)
		go func(p ProviderConfig) {
			defer planWg.Done()
			pctx, cancel := context.WithTimeout(ctx, planTimeout)
			defer cancel()
			sess, err := t.ForkFunc(t.SourceSessionID, 0)
			if err != nil {
				planChan <- planResult{provider: p, err: fmt.Errorf("fork failed: %w", err)}
				return
			}
			planPrompt := t.Prompt + "\n\nProduce a detailed implementation plan. Do NOT write code."
			result, err := t.RunFunc(pctx, p, sess, planPrompt)
			if err != nil {
				planChan <- planResult{provider: p, err: err}
				return
			}
			planChan <- planResult{provider: p, plan: result.Summary}
		}(prov)
	}
	planWg.Wait()
	close(planChan)
	var candidates []PlanCandidate
	var planErrors []string
	for pr := range planChan {
		if pr.err != nil {
			planErrors = append(planErrors, fmt.Sprintf("%s: %v", pr.provider.Name, pr.err))
			continue
		}
		candidates = append(candidates, PlanCandidate{Model: pr.provider.Name, Plan: pr.plan})
	}
	if len(candidates) < 1 {
		return &Result{Mode: ModePlanMerge, Success: false, Error: fmt.Sprintf("all %d planners failed: %s", len(t.Providers), strings.Join(planErrors, "; "))}, nil
	}
	mergedPlan, err := t.PlanMergeJudge(ctx, t.Prompt, candidates)
	if err != nil {
		return &Result{Mode: ModePlanMerge, Success: false, Error: fmt.Sprintf("plan-merge judge failed: %v", err)}, nil
	}
	t.fireHook(ctx, "fusion.dispatch", map[string]any{"phase": "plan-merged", "candidates": len(candidates), "merged_plan_len": len(mergedPlan)})
	execProv := t.Providers[0]
	execSess, err := t.ForkFunc(t.SourceSessionID, 0)
	if err != nil {
		return &Result{Mode: ModePlanMerge, Success: false, Error: fmt.Sprintf("execution fork failed: %v", err)}, nil
	}
	execPrompt := t.Prompt + "\n\nImplementation plan (follow this plan):\n\n" + mergedPlan
	execResult, err := t.RunFunc(ctx, execProv, execSess, execPrompt)
	if err != nil {
		return &Result{Mode: ModePlanMerge, Success: false, Error: fmt.Sprintf("execution failed: %v", err)}, nil
	}
	vr := t.VerifyFn(ctx, t.Workspace)
	elapsed := time.Since(start)
	result := &Result{Mode: ModePlanMerge, Success: vr.Passed, Winner: &Candidate{Provider: execProv.Name, Output: execResult.Summary}, Verified: vr.Passed, Duration: elapsed, Plans: candidates, MergedPlan: mergedPlan}
	if vr.Passed {
		result.VerifyResult = vr
		t.fireHook(ctx, "fusion.dispatch", map[string]any{"phase": "complete", "mode": "plan-merge", "winner": execProv.Name, "success": true})
	} else {
		result.Error = "execution passed but verify-gate failed: " + vr.Report
		t.fireHook(ctx, "fusion.dispatch", map[string]any{"phase": "verify-fail", "mode": "plan-merge", "report": vr.Report})
	}
	if t.Lessons != nil && t.Workspace != "" {
		entryType := lessons.TypeSuccessPattern
		if !result.Success {
			entryType = lessons.TypeFailedVerification
		}
		_ = t.Lessons.Record(ctx, lessons.Entry{Type: entryType, Workspace: t.Workspace, Context: map[string]any{"mode": "plan-merge", "plans": len(candidates)}, Lesson: fmt.Sprintf("plan-merge tournament: %d plans merged, success=%v", len(candidates), result.Success)})
	}
	return result, nil
}

func sortPlanCandidates(plans []PlanCandidate) {
	sort.SliceStable(plans, func(i, j int) bool { return plans[i].Model < plans[j].Model })
}
