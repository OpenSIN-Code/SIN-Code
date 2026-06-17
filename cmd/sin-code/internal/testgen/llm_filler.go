// SPDX-License-Identifier: MIT
// Purpose: LLM-driven test code generation for sin_test_generate. Unlike
// llm.go (which fills JSON cases into a scaffold), LLMFiller asks the model
// for a complete table-driven Go test file wrapped in a markdown code block.
// The RepairLoop (repair_loop.go) wraps LLMFiller in a generate→compile→
// execute→repair cycle.
//
// Issue #256: wires the LLM case filling and generate/execute/repair loop.
package testgen

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/llm"
)

const (
	defaultFillerMaxTokens   = 2048
	defaultFillerTemperature = 0.2
)

const defaultFillerSystemPrompt = "You are a Go test generation expert. " +
	"Generate table-driven Go tests with edge cases covering the happy path, " +
	"boundary conditions, and error branches. " +
	"Return ONLY a fenced ```go code block containing the complete test file. " +
	"Do not include prose, commentary, or explanations."

// FillRequest describes a single LLM test-generation call.
type FillRequest struct {
	SourceFile    string
	FunctionName  string
	ExistingTests string
	Language      string
	MaxCases      int
}

// FillResult holds the parsed LLM output for one Fill call.
type FillResult struct {
	TestCode       string
	CasesGenerated int
	Model          string
	TokensUsed     int
}

// LLMFillerOption configures an LLMFiller at construction time.
type LLMFillerOption func(*LLMFiller)

// LLMFiller generates complete Go test files via an LLM. It is the
// generate step in the RepairLoop cycle.
type LLMFiller struct {
	client       *llm.Client
	model        string
	maxTokens    int
	temperature  float64
	systemPrompt string
}

// NewLLMFiller constructs an LLMFiller with sensible defaults. Pass
// options to override max tokens, temperature, or the system prompt.
func NewLLMFiller(client *llm.Client, model string, opts ...LLMFillerOption) *LLMFiller {
	f := &LLMFiller{
		client:       client,
		model:        model,
		maxTokens:    defaultFillerMaxTokens,
		temperature:  defaultFillerTemperature,
		systemPrompt: defaultFillerSystemPrompt,
	}
	for _, o := range opts {
		o(f)
	}
	return f
}

// WithFillerMaxTokens sets the max_tokens parameter for chat completions.
func WithFillerMaxTokens(n int) LLMFillerOption {
	return func(f *LLMFiller) {
		if n > 0 {
			f.maxTokens = n
		}
	}
}

// WithFillerTemperature sets the sampling temperature (0–2).
func WithFillerTemperature(t float64) LLMFillerOption {
	return func(f *LLMFiller) {
		if t >= 0 && t <= 2 {
			f.temperature = t
		}
	}
}

// WithFillerSystemPrompt overrides the default system prompt.
func WithFillerSystemPrompt(s string) LLMFillerOption {
	return func(f *LLMFiller) {
		if s != "" {
			f.systemPrompt = s
		}
	}
}

// Fill sends a chat completion asking the LLM for a complete test file
// and extracts the Go code block from the response. When
// req.ExistingTests is non-empty the prompt asks the model to repair
// the previous code using the embedded failure output.
func (f *LLMFiller) Fill(ctx context.Context, req FillRequest) (*FillResult, error) {
	if f == nil || f.client == nil {
		return nil, fmt.Errorf("testgen: LLMFiller client is nil")
	}
	if req.SourceFile == "" {
		return nil, fmt.Errorf("testgen: FillRequest.SourceFile is required")
	}

	src, err := os.ReadFile(req.SourceFile)
	if err != nil {
		return nil, fmt.Errorf("testgen: read source file %s: %w", req.SourceFile, err)
	}

	maxCases := req.MaxCases
	if maxCases <= 0 {
		maxCases = 5
	}
	language := req.Language
	if language == "" {
		language = "go"
	}

	prompt := buildFillerPrompt(req, string(src), maxCases, language)

	resp, err := f.client.Chat(ctx, llm.ChatRequest{
		Model:       f.model,
		MaxTokens:   f.maxTokens,
		Temperature: f.temperature,
		Messages: []llm.Message{
			{Role: "system", Content: f.systemPrompt},
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("testgen: LLM chat: %w", err)
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("testgen: LLM returned no choices")
	}

	raw := resp.Choices[0].Message.Content
	code, count := extractGoCodeBlock(raw)
	if code == "" {
		return &FillResult{
			Model:      resp.Model,
			TokensUsed: resp.Usage.TotalTokens,
		}, fmt.Errorf("testgen: no Go code block found in LLM response")
	}

	return &FillResult{
		TestCode:       code,
		CasesGenerated: count,
		Model:          resp.Model,
		TokensUsed:     resp.Usage.TotalTokens,
	}, nil
}

// buildFillerPrompt constructs the user-message prompt for the LLM. The
// prompt is deterministic per (req, source, maxCases, language) so the
// four-arm comparator (issue #171) can pin its golden snapshot.
func buildFillerPrompt(req FillRequest, source string, maxCases int, language string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Generate %d table-driven %s test cases with edge cases for the following source file.\n", maxCases, language)
	if req.FunctionName != "" {
		fmt.Fprintf(&b, "Focus on the function: %s\n", req.FunctionName)
	}
	if req.ExistingTests != "" {
		fmt.Fprintf(&b, "\nPrevious test code and/or failure output to repair:\n%s\n\n", req.ExistingTests)
		fmt.Fprintf(&b, "Fix the failing tests and regenerate the complete test file.\n")
	}
	fmt.Fprintf(&b, "\nSource file %s:\n```%s\n%s\n```\n", req.SourceFile, language, source)
	fmt.Fprintf(&b, "\nReturn ONLY a ```go code block with the complete test file.\n")
	return b.String()
}

var goCodeBlockRe = regexp.MustCompile("(?s)```(?:go|golang)?\\s*\\n(.*?)\\n?```")

// extractGoCodeBlock pulls the first ```go (or bare ```) fenced block
// from raw LLM output. Returns the code and a count of Test functions
// found (minimum 1 when code is present).
func extractGoCodeBlock(raw string) (string, int) {
	m := goCodeBlockRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) < 2 {
		return "", 0
	}
	code := strings.TrimSpace(m[1])
	count := strings.Count(code, "func Test")
	if count == 0 {
		count = 1
	}
	return code, count
}
