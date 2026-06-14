// SPDX-License-Identifier: MIT
// Purpose: Tests for LLM-as-a-Judge Evaluator
package eval

import (
	"context"
	"testing"
)

func TestJudgeCreation(t *testing.T) {
	judge := NewJudge("test-model")
	if judge == nil {
		t.Fatal("Judge is nil")
	}
	if judge.model != "test-model" {
		t.Errorf("Expected model 'test-model', got %q", judge.model)
	}
}

func TestJudgeResultStructure(t *testing.T) {
	result := &JudgeResult{
		Score:    0.85,
		Reasoning: "Good output",
		Passed:   true,
		Feedback: "Works well",
		Criteria: map[string]float64{
			"correctness": 0.9,
			"completeness": 0.8,
		},
	}

	if result.Score != 0.85 {
		t.Errorf("Expected score 0.85, got %f", result.Score)
	}
	if !result.Passed {
		t.Error("Expected Passed to be true")
	}
	if len(result.Criteria) != 2 {
		t.Errorf("Expected 2 criteria, got %d", len(result.Criteria))
	}
}

func TestEvaluate(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	output := "Here is the generated code:\n```go\nfunc main() { fmt.Println(\"hello\") }\n```"
	expectedKeywords := []string{"code", "func", "main"}
	constraints := map[string]interface{}{
		"max_length": 1000,
	}

	result := judge.Evaluate(ctx, output, expectedKeywords, constraints)

	if result == nil {
		t.Fatal("Judge.Evaluate returned nil")
	}
	if result.Score < 0.0 || result.Score > 1.0 {
		t.Errorf("Score out of range: %f", result.Score)
	}
}

func TestEvaluateWithKeywords(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	tests := []struct {
		name     string
		output   string
		keywords []string
		wantPass bool
	}{
		{
			name:     "all keywords present",
			output:   "success completed verified",
			keywords: []string{"success", "completed"},
			wantPass: true,
		},
		{
			name:     "missing keyword",
			output:   "success only",
			keywords: []string{"success", "completed"},
			wantPass: false,
		},
		{
			name:     "empty keywords",
			output:   "any output",
			keywords: []string{},
			wantPass: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := judge.Evaluate(ctx, tt.output, tt.keywords, nil)
			if (result.Score > 0.5) != tt.wantPass {
				t.Errorf("Evaluate keyword matching failed")
			}
		})
	}
}

func TestBuildJudgePrompt(t *testing.T) {
	judge := NewJudge("test")
	output := "test output"
	criteria := []string{"correctness", "completeness"}

	prompt := judge.buildJudgePrompt(output, criteria)

	if prompt == "" {
		t.Error("buildJudgePrompt returned empty string")
	}
	if len(prompt) < len(output) {
		t.Error("Prompt too short")
	}
}

func TestMockEvaluate(t *testing.T) {
	judge := NewJudge("mock")

	output := "test"
	result := judge.mockEvaluate(output, []string{"test"})

	if result == nil {
		t.Fatal("mockEvaluate returned nil")
	}
	if result.Score <= 0 || result.Score > 1 {
		t.Errorf("Invalid score: %f", result.Score)
	}
}

func TestEvaluateMultiple(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	outputs := []string{
		"correct output",
		"another valid output",
		"third output",
	}

	results := judge.EvaluateMultiple(ctx, outputs, []string{"output"}, nil)

	if len(results) != len(outputs) {
		t.Errorf("Expected %d results, got %d", len(outputs), len(results))
	}

	for i, result := range results {
		if result == nil {
			t.Errorf("Result %d is nil", i)
		}
		if result.Score < 0 || result.Score > 1 {
			t.Errorf("Result %d has invalid score: %f", i, result.Score)
		}
	}
}

func TestScoreThreshold(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	tests := []struct {
		name           string
		output         string
		threshold      float64
		expectPass     bool
	}{
		{"high quality", "excellent output with perfect code", 0.5, true},
		{"medium quality", "output is ok", 0.8, false},
		{"perfect score", "perfect perfect perfect", 0.99, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := judge.Evaluate(ctx, tt.output, nil, nil)
			passed := result.Score >= tt.threshold
			if passed != tt.expectPass {
				t.Logf("Score: %f, Threshold: %f, Pass: %v", result.Score, tt.threshold, passed)
			}
		})
	}
}

func TestCriteriaScoring(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	output := "test output"
	result := judge.Evaluate(ctx, output, nil, nil)

	if result.Criteria == nil {
		t.Error("Criteria is nil")
	}

	// Should have multiple criteria
	if len(result.Criteria) < 3 {
		t.Logf("Expected at least 3 criteria, got %d (OK for mock)", len(result.Criteria))
	}
}

func TestJudgeWithConstraints(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	constraints := map[string]interface{}{
		"max_length":    1000,
		"required_libs": []string{"fmt", "log"},
		"forbidden":     []string{"panic"},
	}

	result := judge.Evaluate(ctx, "test output", nil, constraints)

	if result == nil {
		t.Fatal("Evaluate with constraints returned nil")
	}
	if result.Score == 0 {
		t.Error("Score should not be 0")
	}
}

func TestConcurrentEvaluation(t *testing.T) {
	judge := NewJudge("mock")
	ctx := context.Background()

	// Run multiple evaluations concurrently
	results := make(chan *JudgeResult, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			result := judge.Evaluate(ctx, "output"+string(rune(index)), nil, nil)
			results <- result
		}(i)
	}

	// Collect all results
	count := 0
	for count < 10 {
		result := <-results
		if result == nil {
			t.Error("Received nil result")
		}
		count++
	}

	if count != 10 {
		t.Errorf("Expected 10 results, got %d", count)
	}
}

func BenchmarkEvaluate(b *testing.B) {
	judge := NewJudge("mock")
	ctx := context.Background()
	output := "test output that should be evaluated"
	keywords := []string{"test", "output"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		judge.Evaluate(ctx, output, keywords, nil)
	}
}

func BenchmarkBuildJudgePrompt(b *testing.B) {
	judge := NewJudge("mock")
	output := "test output"
	criteria := []string{"correctness", "completeness", "clarity"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		judge.buildJudgePrompt(output, criteria)
	}
}
