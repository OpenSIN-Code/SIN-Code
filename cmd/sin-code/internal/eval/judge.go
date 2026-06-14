// SPDX-License-Identifier: MIT
// Purpose: LLM-as-a-Judge implementation for automated evaluation
package eval

import (
	"context"
	"fmt"
	"strings"
)

// JudgeResult contains the evaluation result from the judge LLM
type JudgeResult struct {
	Score      float64            `json:"score"`      // 0.0 - 1.0
	Reasoning  string             `json:"reasoning"`
	Criteria   map[string]float64 `json:"criteria"`   // Score per criterion
	Passed     bool               `json:"passed"`
	Feedback   string             `json:"feedback"`
}

// Judge evaluates agent outputs using LLM-as-a-Judge pattern
type Judge struct {
	model      string
	maxRetries int
}

// NewJudge creates a new LLM-as-a-Judge instance
func NewJudge(model string) *Judge {
	return &Judge{
		model:      model,
		maxRetries: 3,
	}
}

// Evaluate evaluates an agent output against expected criteria
func (j *Judge) Evaluate(ctx context.Context, agentOutput string, expectedCriteria []string, minQuality float64) (*JudgeResult, error) {
	prompt := j.buildJudgePrompt(agentOutput, expectedCriteria, minQuality)

	// In actual implementation, this would call the LLM provider
	// For now, we provide a mock implementation
	result := j.mockEvaluate(agentOutput, expectedCriteria, minQuality)

	return result, nil
}

// buildJudgePrompt constructs the prompt for the judge LLM
func (j *Judge) buildJudgePrompt(output string, criteria []string, minQuality float64) string {
	criteriaStr := strings.Join(criteria, "\n- ")

	prompt := fmt.Sprintf(`You are an expert evaluator for AI agent outputs.

Evaluate the following agent output against these criteria:
- %s

Agent Output:
---
%s
---

Provide your evaluation in JSON format with:
{
  "score": 0.0-1.0,
  "reasoning": "explanation of score",
  "criteria": {"criterion": score, ...},
  "passed": true/false (based on minimum quality of %.2f),
  "feedback": "constructive feedback"
}

Be strict and fair in your evaluation.`, criteriaStr, output, minQuality)

	return prompt
}

// mockEvaluate provides a mock implementation for testing
func (j *Judge) mockEvaluate(output string, criteria []string, minQuality float64) *JudgeResult {
	// Simple heuristic: count keyword presence
	score := 0.5 // baseline score

	for _, criterion := range criteria {
		if strings.Contains(strings.ToLower(output), strings.ToLower(criterion)) {
			score += 0.1
		}
	}

	// Cap at 1.0
	if score > 1.0 {
		score = 1.0
	}

	passed := score >= minQuality

	return &JudgeResult{
		Score:     score,
		Reasoning: "Mock evaluation based on keyword matching",
		Criteria: map[string]float64{
			"completeness": score * 0.8,
			"clarity":      score * 0.7,
			"correctness":  score * 0.6,
		},
		Passed: passed,
		Feedback: fmt.Sprintf(
			"Output quality score: %.2f (minimum required: %.2f). %s",
			score, minQuality,
			map[bool]string{true: "PASSED", false: "FAILED"}[passed],
		),
	}
}

// EvaluateMultiple evaluates multiple outputs in batch
func (j *Judge) EvaluateMultiple(ctx context.Context, outputs []string, expectedCriteria []string, minQuality float64) ([]*JudgeResult, error) {
	results := make([]*JudgeResult, len(outputs))

	for i, output := range outputs {
		result, err := j.Evaluate(ctx, output, expectedCriteria, minQuality)
		if err != nil {
			return nil, err
		}
		results[i] = result
	}

	return results, nil
}
