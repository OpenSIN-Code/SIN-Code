// SPDX-License-Identifier: MIT
// Purpose: LLM-as-a-Judge for automated evaluation of agent outputs
package eval

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// JudgeResult enthält das Bewertungsergebnis eines LLM-Judges
type JudgeResult struct {
	Score       float64            `json:"score"`           // 0.0 - 1.0
	Passed      bool               `json:"passed"`          // Score >= Threshold
	Reasoning   string             `json:"reasoning"`
	Criteria    map[string]float64 `json:"criteria_scores"` // Score pro Kriterium
	Feedback    string             `json:"feedback"`
	RawResponse string             `json:"raw_response,omitempty"`
}

// Judge wertet Agent-Outputs automatisiert
type Judge struct {
	model      string // z.B. "openai/gpt-4-mini"
	threshold  float64
	maxRetries int
}

// NewJudge erstellt einen neuen LLM-Judge
func NewJudge(model string) *Judge {
	return &Judge{
		model:      model,
		threshold:  0.7,
		maxRetries: 3,
	}
}

// Evaluate bewertet einen Agent-Output gegen Kriterien
func (j *Judge) Evaluate(ctx context.Context, criteria []string, output string, toolsUsed []string) JudgeResult {
	result := JudgeResult{
		Criteria: make(map[string]float64),
	}

	if output == "" {
		return JudgeResult{
			Score:    0.0,
			Passed:   false,
			Feedback: "Agent produced no output",
		}
	}

	// Für lokale Entwicklung: keyword-basierte Fallback-Bewertung
	if j.model == "" || strings.Contains(j.model, "mock") {
		return j.mockEvaluate(criteria, output, toolsUsed)
	}

	// Echter LLM-Call (mit Fallback auf Mock)
	judgePrompt := j.buildJudgePrompt(criteria, output, toolsUsed)
	response, err := j.callLLM(ctx, judgePrompt)
	if err != nil {
		return j.mockEvaluate(criteria, output, toolsUsed)
	}

	// Parse LLM-Antwort
	result.RawResponse = response
	if err := j.parseJudgeResponse(response, &result); err != nil {
		return j.mockEvaluate(criteria, output, toolsUsed)
	}

	result.Passed = result.Score >= j.threshold
	return result
}

// EvaluateMultiple wertet mehrere Outputs parallel
func (j *Judge) EvaluateMultiple(ctx context.Context, criteria []string, outputs []string) []JudgeResult {
	results := make([]JudgeResult, len(outputs))
	for i, output := range outputs {
		results[i] = j.Evaluate(ctx, criteria, output, nil)
	}
	return results
}

// buildJudgePrompt konstruiert einen Prompt für den Judge-LLM
func (j *Judge) buildJudgePrompt(criteria []string, output string, toolsUsed []string) string {
	criteriaText := strings.Join(criteria, "\n- ")
	toolsText := "none"
	if len(toolsUsed) > 0 {
		toolsText = strings.Join(toolsUsed, ", ")
	}

	prompt := fmt.Sprintf(`You are an expert evaluator for a code generation agent.

Evaluate the following agent output against these criteria:
- %s

Agent Output:
---
%s
---

Tools Used: %s

Respond ONLY with valid JSON (no markdown, no extra text) in this exact format:
{
  "score": 0.85,
  "passed": true,
  "reasoning": "The output meets X and Y criteria but lacks Z",
  "criteria_scores": {
    "criterion_1": 0.9,
    "criterion_2": 0.8
  },
  "feedback": "Improve by adding more error handling"
}

Criteria scoring rules:
- 1.0 = Excellent, fully meets criterion
- 0.8 = Good, mostly meets criterion
- 0.5 = Partial, partially meets criterion
- 0.0 = Missing, does not meet criterion

Overall score is the average of all criterion scores.
Passed = true if score >= 0.7.
`, criteriaText, output, toolsText)

	return prompt
}

// callLLM ruft den Judge-LLM auf (mit Retry-Logik)
func (j *Judge) callLLM(ctx context.Context, prompt string) (string, error) {
	// TODO: Integration mit AI SDK / Vercel AI Gateway
	// Beispiel mit AI SDK 6 (wenn implementiert):
	//
	// import "github.com/vercel/ai-go"
	// client := ai.NewClient()
	// response, err := client.GenerateText(ctx, &ai.GenerateTextRequest{
	//   Model: j.model,
	//   Messages: []ai.Message{{
	//     Role:    "user",
	//     Content: prompt,
	//   }},
	//   Temperature: 0.2,
	//   MaxTokens:  500,
	// })
	// if err != nil {
	//   return "", err
	// }
	// return response.Text, nil

	// Fallback
	return "", fmt.Errorf("LLM call not implemented")
}

// parseJudgeResponse parsed JSON-Response des Judges
func (j *Judge) parseJudgeResponse(response string, result *JudgeResult) error {
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	var parsed struct {
		Score          float64            `json:"score"`
		Passed         bool               `json:"passed"`
		Reasoning      string             `json:"reasoning"`
		CriteriaScores map[string]float64 `json:"criteria_scores"`
		Feedback       string             `json:"feedback"`
	}

	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		return fmt.Errorf("failed to parse judge JSON: %w", err)
	}

	result.Score = parsed.Score
	result.Passed = parsed.Passed
	result.Reasoning = parsed.Reasoning
	result.Criteria = parsed.CriteriaScores
	result.Feedback = parsed.Feedback

	return nil
}

// mockEvaluate liefert Fallback-Bewertung basierend auf Keywords
func (j *Judge) mockEvaluate(criteria []string, output string, toolsUsed []string) JudgeResult {
	result := JudgeResult{
		Criteria: make(map[string]float64),
	}

	output = strings.ToLower(output)

	// Keyword-basierte Heuristik
	keywordScores := map[string]float64{
		"error":      0.0,
		"invalid":    0.1,
		"success":    0.9,
		"completed": 0.85,
		"verified":   0.9,
		"tested":     0.8,
	}

	score := 0.5
	for keyword, s := range keywordScores {
		if strings.Contains(output, keyword) {
			score = s
			break
		}
	}

	// Tools bonus
	if len(toolsUsed) > 0 {
		score += 0.1
		if score > 1.0 {
			score = 1.0
		}
	}

	// Criteria scoring
	for _, criterion := range criteria {
		if strings.Contains(output, strings.ToLower(criterion)) {
			result.Criteria[criterion] = score
		} else {
			result.Criteria[criterion] = score * 0.8
		}
	}

	result.Score = score
	result.Passed = score >= j.threshold
	result.Reasoning = "Mock evaluation (LLM integration pending). Score based on keyword matching and tool usage."
	result.Feedback = "For accurate evaluation, configure LLM integration with AI SDK."
	result.RawResponse = fmt.Sprintf(`{"score": %.2f, "passed": %v}`, score, result.Passed)

	return result
}
