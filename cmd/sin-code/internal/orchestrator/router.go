// SPDX-License-Identifier: MIT
// Purpose: Pre-LLM router — cheap intent classification using keyword heuristics.
// In production, this would call a Haiku-class model. For now, deterministic
// keyword scoring is the fallback when no LLM is configured.
package orchestrator

import (
	"strings"
)

type Router struct {
	Keywords map[Intent][]string
}

func NewRouter() *Router {
	return &Router{
		Keywords: map[Intent][]string{
			IntentCodebase: {
				"implement", "add", "create", "build", "write", "code",
				"feature", "refactor", "fix", "bug", "implement",
				"function", "method", "class", "module", "api",
				"endpoint", "route", "handler", "service",
			},
			IntentTest: {
				"test", "spec", "coverage", "unit test", "integration test",
				"e2e", "end-to-end", "tdd", "mock", "stub", "assert",
			},
			IntentReview: {
				"review", "lint", "audit code", "check", "pr", "pull request",
				"diff", "code review", "feedback", "approve",
			},
			IntentDocs: {
				"document", "docs", "documentation", "readme", "comment",
				"javadoc", "docstring", "explain", "tutorial", "guide",
				"changelog", "wiki",
			},
			IntentSecurity: {
				"security", "vulnerability", "cve", "exploit", "audit",
				"penetration", "pentest", "xss", "csrf", "injection",
				"owasp", "secret", "credential", "leak",
			},
			IntentArchitecture: {
				"architect", "design", "diagram", "schema", "data model",
				"structure", "pattern", "refactor architecture", "decompose",
				"system design", "adl",
			},
		},
	}
}

func (r *Router) Classify(prompt string) Intent {
	p := strings.ToLower(prompt)
	scores := map[Intent]int{}
	for intent, words := range r.Keywords {
		for _, w := range words {
			if strings.Contains(p, w) {
				scores[intent]++
			}
		}
	}
	max := 0
	best := IntentGeneral
	for intent, score := range scores {
		if score > max {
			max = score
			best = intent
		}
		if score == max && intent == IntentTest {
			best = IntentTest
		}
	}
	return best
}

func (r *Router) SubIntents(prompt string) []Intent {
	seen := map[Intent]bool{}
	var out []Intent
	p := strings.ToLower(prompt)
	for intent, words := range r.Keywords {
		for _, w := range words {
			if strings.Contains(p, w) {
				if !seen[intent] {
					seen[intent] = true
					out = append(out, intent)
				}
				break
			}
		}
	}
	return out
}
