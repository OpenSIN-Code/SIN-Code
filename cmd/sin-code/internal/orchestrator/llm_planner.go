// SPDX-License-Identifier: MIT
// Purpose: LLMPlanner — an LLM-driven planner for the orchestrator
// (issue #383). Unlike the rule-based Planner (planner.go) and the
// heuristic DeepPlanner (deep_planner.go), the LLMPlanner asks a real
// language model to decompose a goal into ordered steps, each bound to
// a tool from the available surface. The model returns a JSON plan
// (steps, rationale, confidence) which ParsePlanResponse extracts from
// the raw completion — tolerating markdown fences and surrounding prose
// the way most chat models wrap code blocks.
//
// The LLMClient interface is intentionally minimal so any provider can
// satisfy it (the existing internal/llm.Client can be wrapped, or a
// test mock can be dropped in). LLMPlanner holds no mutable state and
// is safe for concurrent use (M7): Plan is stateless and the client is
// expected to be goroutine-safe.
package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// LLMClient is the minimal completion interface the LLMPlanner needs.
// Any provider that can turn a prompt into a text completion satisfies
// it — the orchestrator's own internal/llm.Client can be wrapped, and
// tests drop in a mock that returns a canned string.
type LLMClient interface {
	Complete(ctx context.Context, prompt string) (string, error)
}

// PlanRequest is the input to LLMPlanner.Plan. Prompt is the user goal;
// Context is optional background (e.g. repository shape, prior
// decisions); AvailableTools is the tool surface the model may select
// from; Constraints are hard rules the plan must honour (e.g. "no
// network", "must pass go test").
type PlanRequest struct {
	Prompt         string
	Context        string
	AvailableTools []string
	Constraints    []string
}

// PlanStep is one step in an LLM-generated plan. ID is a short stable
// identifier the model picks (e.g. "s1"); Action is the human-readable
// description; Tool is the tool name from AvailableTools; Args is the
// opaque argument bag the model fills; DependsOn lists step IDs that
// must complete before this one starts (forms a DAG).
type PlanStep struct {
	ID        string         `json:"id"`
	Action    string         `json:"action"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	DependsOn []string       `json:"depends_on"`
}

// LLMPlan is the parsed output of an LLMPlanner.Plan call. Steps is the
// ordered DAG of work; Rationale is the model's explanation; Confidence
// is the model's self-reported score in [0,1].
type LLMPlan struct {
	Steps      []PlanStep `json:"steps"`
	Rationale  string     `json:"rationale"`
	Confidence float64    `json:"confidence"`
}

// LLMPlanner turns a PlanRequest into an LLMPlan by asking an LLMClient
// to decompose the goal. It is stateless and safe for concurrent use.
type LLMPlanner struct {
	client LLMClient
	model  string
}

// NewLLMPlanner constructs an LLMPlanner backed by the given client.
// The model string is included in the rendered prompt so the model can
// self-identify and so downstream telemetry can attribute the plan; it
// is not used to dispatch the request (that is the client's job).
func NewLLMPlanner(client LLMClient, model string) *LLMPlanner {
	return &LLMPlanner{client: client, model: model}
}

// Plan renders a structured prompt from req, calls the LLM, and parses
// the JSON plan from the response. It returns an error if the client
// fails or the response cannot be parsed into a valid plan.
func (p *LLMPlanner) Plan(ctx context.Context, req PlanRequest) (*LLMPlan, error) {
	if p.client == nil {
		return nil, errors.New("llm_planner: client is nil")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, errors.New("llm_planner: prompt is required")
	}

	prompt := p.renderPrompt(req)
	raw, err := p.client.Complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm_planner: complete: %w", err)
	}
	plan, err := p.ParsePlanResponse(raw)
	if err != nil {
		return nil, fmt.Errorf("llm_planner: parse: %w", err)
	}
	return plan, nil
}

// ParsePlanResponse extracts the JSON plan object from a raw LLM
// completion. It tolerates markdown code fences (```json ... ```) and
// leading/trailing prose by locating the first balanced JSON object
// that decodes into an LLMPlan. A response with no steps is rejected.
func (p *LLMPlanner) ParsePlanResponse(raw string) (*LLMPlan, error) {
	jsonStr, err := extractJSONObject(raw)
	if err != nil {
		return nil, fmt.Errorf("extract json: %w", err)
	}

	var plan LLMPlan
	if err := json.Unmarshal([]byte(jsonStr), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal plan: %w", err)
	}
	if len(plan.Steps) == 0 {
		return nil, errors.New("plan has no steps")
	}
	// Validate step IDs are non-empty and references point at known IDs.
	known := make(map[string]struct{}, len(plan.Steps))
	for i := range plan.Steps {
		s := &plan.Steps[i]
		if strings.TrimSpace(s.ID) == "" {
			return nil, fmt.Errorf("step %d has empty id", i)
		}
		if _, dup := known[s.ID]; dup {
			return nil, fmt.Errorf("step %s has duplicate id", s.ID)
		}
		known[s.ID] = struct{}{}
	}
	for i := range plan.Steps {
		for _, dep := range plan.Steps[i].DependsOn {
			if _, ok := known[dep]; !ok {
				return nil, fmt.Errorf("step %s depends on unknown step %s", plan.Steps[i].ID, dep)
			}
		}
	}
	// Clamp confidence to [0,1].
	if plan.Confidence < 0 {
		plan.Confidence = 0
	}
	if plan.Confidence > 1 {
		plan.Confidence = 1
	}
	return &plan, nil
}

// renderPrompt builds the structured prompt sent to the LLM. It is
// deterministic so tests can assert on it and so the plan is
// reproducible for a given (request, model) pair.
func (p *LLMPlanner) renderPrompt(req PlanRequest) string {
	var b strings.Builder
	b.WriteString("You are a planning agent. Decompose the goal into an ordered plan.\n\n")
	if p.model != "" {
		b.WriteString("Model: ")
		b.WriteString(p.model)
		b.WriteString("\n\n")
	}
	b.WriteString("Goal:\n")
	b.WriteString(req.Prompt)
	b.WriteString("\n\n")
	if req.Context != "" {
		b.WriteString("Context:\n")
		b.WriteString(req.Context)
		b.WriteString("\n\n")
	}
	if len(req.AvailableTools) > 0 {
		b.WriteString("Available tools:\n")
		b.WriteString(strings.Join(req.AvailableTools, ", "))
		b.WriteString("\n\n")
	}
	if len(req.Constraints) > 0 {
		b.WriteString("Constraints:\n")
		for _, c := range req.Constraints {
			b.WriteString("- ")
			b.WriteString(c)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("Respond with a single JSON object of the form:\n")
	b.WriteString(`{"steps":[{"id":"s1","action":"...","tool":"...","args":{},"depends_on":[]}],` + "\n")
	b.WriteString(`"rationale":"...","confidence":0.0}` + "\n\n")
	b.WriteString("Do not wrap the JSON in markdown fences. Each step id must be unique. ")
	b.WriteString("Each depends_on entry must reference a known step id.\n")
	return b.String()
}

// extractJSONObject locates the first balanced JSON object in raw. It
// scans byte-by-byte tracking brace depth and string state so it works
// on responses that wrap the JSON in ```json fences or surround it with
// prose. Returns the substring of the outermost object.
func extractJSONObject(raw string) (string, error) {
	start := strings.IndexByte(raw, '{')
	if start < 0 {
		return "", errors.New("no '{' found in response")
	}
	depth := 0
	inStr := false
	escaped := false
	for i := start; i < len(raw); i++ {
		c := raw[i]
		if inStr {
			if escaped {
				escaped = false
				continue
			}
			if c == '\\' {
				escaped = true
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
			continue
		}
		if c == '{' {
			depth++
		} else if c == '}' {
			depth--
			if depth == 0 {
				return raw[start : i+1], nil
			}
		}
	}
	return "", errors.New("unbalanced braces in response")
}
