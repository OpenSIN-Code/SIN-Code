// SPDX-License-Identifier: MIT
// Purpose: question catalog — the canonical anti-patterns and
// grilling questions (issue #141). Ported from the external
// SIN-Code-Grill-Me-Skill catalog. v0 ships a small seed; v1 will
// import the full list from the upstream SKILL.md.
//
// The catalog is intentionally a Go slice (not a YAML file) so
// that:
//  1. The CLI has zero file-deps for `grill next`.
//  2. The questions are typed (no string-template magic).
//  3. Operators can grep the binary for the question set.
package grill

// AntiPattern is one entry in the catalog: the "what to look for"
// plus 2-3 example questions. The CLI picks one anti-pattern at
// random (or round-robin) per `grill next` call.
type AntiPattern struct {
	Name        string
	Description string
	Questions   []string
}

// Catalog is the seed of anti-patterns the v0 grill ships with.
// v1 will import the full upstream catalog and add domain-specific
// patterns (security, perf, ergonomics, etc.).
var Catalog = []AntiPattern{
	{
		Name:        "Hidden Assumptions",
		Description: "The plan depends on conditions the operator hasn't verified yet.",
		Questions: []string{
			"What does this plan assume about the operator's environment that you have not verified?",
			"If the assumption is wrong, what's the failure mode?",
			"Could you state the assumption in a single sentence and label it 'unverified'?",
		},
	},
	{
		Name:        "Rollback Plan",
		Description: "The plan has no path back if it goes wrong.",
		Questions: []string{
			"If the first step of this plan goes wrong, what do you do?",
			"Can the operator undo step 1 cleanly, or is it a one-way door?",
			"Would adding a checkpoint before step 2 change the operator's confidence?",
		},
	},
	{
		Name:        "Failure Modes",
		Description: "The plan only describes the happy path.",
		Questions: []string{
			"List the three most likely ways this plan fails.",
			"For each failure, is it silent (no signal) or loud (error/log)?",
			"Which failure would the operator discover last?",
		},
	},
	{
		Name:        "Operator Cost",
		Description: "The plan costs the operator more than it returns.",
		Questions: []string{
			"How many operator-hours does this plan consume?",
			"How many operator-hours does it save per occurrence?",
			"At what frequency of occurrence is the saving worth the cost?",
		},
	},
	{
		Name:        "Premature Optimization",
		Description: "The plan optimizes a non-bottleneck.",
		Questions: []string{
			"What is the slowest step of this plan?",
			"Is that the same as the most expensive step?",
			"Would removing the optimization make the plan simpler without losing the win?",
		},
	},
	{
		Name:        "Scope Creep",
		Description: "The plan grows beyond its stated goal.",
		Questions: []string{
			"What is the smallest version of this plan that delivers the stated value?",
			"Which of the 'nice to have' items can be deferred?",
			"If you could only ship one feature, which one is it?",
		},
	},
	{
		Name:        "Single Point of Failure",
		Description: "The plan has a single point whose failure cascades.",
		Questions: []string{
			"Which component, if it fails, takes down the whole plan?",
			"Is the SPOF documented in the runbook?",
			"Could you remove the SPOF or replace it with a redundant subsystem?",
		},
	},
	{
		Name:        "Verification Gap",
		Description: "The plan claims to verify, but the verification is itself unverified.",
		Questions: []string{
			"How do you know the verification works?",
			"Has the verification ever caught a failure?",
			"What would a failure of the verification look like?",
		},
	},
}
