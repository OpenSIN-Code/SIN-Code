// SPDX-License-Identifier: MIT
// Purpose: eval-driven development data model — EvalCase, EvalSet, Output,
// Subject, Result, Run + aggregate score. Port of ECC's eval-harness in a
// clean-room Go reimplementation.
// Docs: types.doc.md
package evalharness

import (
	"context"
	"time"
)

// EvalCase is a single test scenario.
type EvalCase struct {
	ID       string            `json:"id"`
	Prompt   string            `json:"prompt"`             // input given to the subject
	Expected string            `json:"expected,omitempty"` // reference answer (optional)
	Tags     []string          `json:"tags,omitempty"`     // e.g. ["go","refactor"]
	Weight   float64           `json:"weight,omitempty"`   // default 1.0
	Meta     map[string]string `json:"meta,omitempty"`
}

// EvalSet is a named collection of cases.
type EvalSet struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	Cases       []EvalCase `json:"cases"`
}

// Output is what the subject produced for one case.
type Output struct {
	Text     string            `json:"text"`
	Success  bool              `json:"success"`
	Duration time.Duration     `json:"duration_ns"`
	Meta     map[string]string `json:"meta,omitempty"`
	// USD is the price the caller wants to attach to this output.
	// Zero-cost path (stub subject, offline CI) leaves it at zero;
	// when set, the comparator uses it instead of re-computing the
	// cost from prompt/completion token counts.
	USD float64 `json:"usd,omitempty"`
}

// Subject is the thing under evaluation: an agent, a verify gate, a model
// call. Implement this to plug SIN-Code's runtime into the harness.
type Subject interface {
	Run(ctx context.Context, c EvalCase) (Output, error)
}

// Result is the scored outcome for one case.
type Result struct {
	CaseID   string        `json:"case_id"`
	Score    float64       `json:"score"` // 0.0 .. 1.0
	Weight   float64       `json:"weight"`
	Passed   bool          `json:"passed"`
	Output   string        `json:"output,omitempty"`
	Detail   string        `json:"detail,omitempty"`
	Duration time.Duration `json:"duration_ns"`
	Err      string        `json:"error,omitempty"`
	// ArmID identifies which arm of a multi-arm comparison produced
	// this result. Empty for legacy single-arm runs. Optional so
	// old consumers don't break.
	ArmID string `json:"arm_id,omitempty"`
	// PromptTokens / CompletionTokens / TotalTokens capture the LLM
	// token budget for the case under this arm. All three are
	// optional; absent in fully-stub CI runs where the LLM is mocked.
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
	// LOC is the line count of the generated output. Used to
	// satisfy ponytail's bench matrix (issue #171, caveman §4).
	LOC int `json:"loc,omitempty"`
	// USD is the price charged for this case under this arm. Zero
	// when the arm is stubbed or self-pricing was disabled.
	USD float64 `json:"usd,omitempty"`
}

// Run is a complete evaluation pass over an EvalSet.
type Run struct {
	ID        string    `json:"id"`
	SetName   string    `json:"set_name"`
	Subject   string    `json:"subject"`
	StartedAt time.Time `json:"started_at"`
	Results   []Result  `json:"results"`
}

// Aggregate computes the weighted score and pass-rate of a run.
func (r Run) Aggregate() (weightedScore, passRate float64) {
	if len(r.Results) == 0 {
		return 0, 0
	}
	var sumW, sumWS float64
	passed := 0
	for _, res := range r.Results {
		w := res.Weight
		if w == 0 {
			w = 1
		}
		sumW += w
		sumWS += w * res.Score
		if res.Passed {
			passed++
		}
	}
	if sumW > 0 {
		weightedScore = sumWS / sumW
	}
	passRate = float64(passed) / float64(len(r.Results))
	return weightedScore, passRate
}

// Arm is one variant of the system prompt under test in a multi-arm
// evaluation. The classical three-arm harness from caveman's
// evals/README.md has:
//
//	__baseline__  — no system prompt
//	__terse__     — "Answer concisely."
//	<skill>       — "Answer concisely.\n\n<SKILL.md body>"
//
// SIN-Code extends this with a fourth "verbosity" arm sourced from
// internal/style.RenderSystemBlock (issue #167) and a fifth "lazy"
// arm that asserts skill-code-lazy (issue #178). An arm carries
// enough metadata that the comparator can print a markdown matrix
// (LOC, USD, latency, correctness) without consulting the prompt
// strings again.
type Arm struct {
	// ID is the stable name reported in snapshots + diff output.
	// Reserved IDs: "__baseline__", "__terse__", "__lazy_skill__",
	// "__user_skill__". Custom skill arms use the skill name (e.g.
	// "skill-code-create").
	ID string
	// SystemPrompt is what gets appended to the user prompt before
	// the LLM is invoked. Empty for "no system prompt" baseline arm.
	SystemPrompt string
	// SkillName names the embedded skill (empty for non-skill arms).
	// The comparator uses this to look up the SKILL.md body when
	// SystemPrompt is left blank (SkillArm case).
	SkillName string
	// Verbosity is the verbosity level (`ultra`, `terse`, `normal`,
	// `verbose`, `default`). Empty for non-verbosity arms.
	Verbosity string
	// PricingName selects the price entry from prices.go (empty =
	// stub pricing, USD=0). Examples: "stub", "gpt-4o-mini".
	PricingName string
	// Setup is invoked once per case before the subject runs, so
	// tests can install a fixture for this arm. Production callers
	// usually leave this nil.
	Setup func(c EvalCase) error
}
