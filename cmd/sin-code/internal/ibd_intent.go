// SPDX-License-Identifier: MIT
// Purpose: ibd intent evaluation — scores how well code changes match the
// stated intent, plus diff counting helpers and text output.
// sin-debt: shrink, upgrade: when a second intent-related function is needed, merge into a shared file
package internal

import (
	"fmt"
	"strings"
)

func evaluateIntent(intent string, added, removed, modified []symbolInfo, diff []diffLine) (string, int) {
	if intent == "" {
		return "unknown", 50
	}

	intentLower := strings.ToLower(intent)
	score := 50

	// Check for keywords in intent
	keywords := []string{"add", "remove", "delete", "refactor", "fix", "implement", "create", "update", "modify", "change", "optimize", "improve", "rename"}
	intentKeywords := make(map[string]bool)
	for _, kw := range keywords {
		if strings.Contains(intentLower, kw) {
			intentKeywords[kw] = true
		}
	}

	// Evaluate based on changes
	if intentKeywords["add"] || intentKeywords["create"] || intentKeywords["implement"] {
		if len(added) > 0 {
			score += 30
		} else {
			score -= 40
		}
	}

	if intentKeywords["remove"] || intentKeywords["delete"] {
		if len(removed) > 0 {
			score += 30
		} else {
			score -= 40
		}
	}

	if intentKeywords["refactor"] || intentKeywords["modify"] || intentKeywords["change"] || intentKeywords["update"] {
		if len(modified) > 0 || len(added) > 0 || len(removed) > 0 {
			score += 20
		}
	}

	if intentKeywords["fix"] || intentKeywords["optimize"] || intentKeywords["improve"] {
		if len(modified) > 0 {
			score += 25
		}
	}

	if intentKeywords["rename"] {
		// Check for add+remove pairs with similar names
		for _, a := range added {
			for _, r := range removed {
				if strings.ToLower(a.Name) == strings.ToLower(r.Name) ||
					strings.Contains(strings.ToLower(a.Name), strings.ToLower(r.Name)) ||
					strings.Contains(strings.ToLower(r.Name), strings.ToLower(a.Name)) {
					score += 30
					break
				}
			}
		}
	}

	// Check for error handling keywords
	if strings.Contains(intentLower, "error") || strings.Contains(intentLower, "exception") || strings.Contains(intentLower, "handle") {
		for _, line := range diff {
			if line.Type == "added" && (strings.Contains(strings.ToLower(line.Text), "error") || strings.Contains(strings.ToLower(line.Text), "exception") || strings.Contains(strings.ToLower(line.Text), "catch") || strings.Contains(strings.ToLower(line.Text), "try")) {
				score += 15
				break
			}
		}
	}

	// Check for retry logic
	if strings.Contains(intentLower, "retry") {
		for _, line := range diff {
			if line.Type == "added" && strings.Contains(strings.ToLower(line.Text), "retry") {
				score += 20
				break
			}
		}
	}

	// Check for test-related changes
	if strings.Contains(intentLower, "test") {
		for _, sym := range added {
			if strings.Contains(strings.ToLower(sym.Name), "test") || strings.Contains(strings.ToLower(sym.Name), "spec") {
				score += 20
				break
			}
		}
	}

	if score > 100 {
		score = 100
	}
	if score < 0 {
		score = 0
	}

	// Determine match level
	match := "none"
	if score >= 80 {
		match = "strong"
	} else if score >= 60 {
		match = "partial"
	} else if score >= 40 {
		match = "weak"
	}

	return match, score
}

func countChanged(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "added" || d.Type == "removed" {
			count++
		}
	}
	return count
}

func countAdded(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "added" {
			count++
		}
	}
	return count
}

func countRemoved(diff []diffLine) int {
	count := 0
	for _, d := range diff {
		if d.Type == "removed" {
			count++
		}
	}
	return count
}

func outputTextIBD(result *ibdResult) error {
	fmt.Printf("Intent-Based Diffing\n")
	fmt.Printf("Before:     %s\n", result.Before)
	fmt.Printf("After:      %s\n", result.After)
	fmt.Printf("Intent:     %s\n", result.Intent)
	fmt.Printf("Match:      %s (score: %d/100)\n\n", result.IntentMatch, result.Score)

	// Show summary of changes
	fmt.Printf("Changes:\n")
	fmt.Printf("  Lines changed: %d (+%d, -%d)\n", countChanged(result.Diff), countAdded(result.Diff), countRemoved(result.Diff))
	fmt.Printf("  Symbols added:   %d\n", len(result.Added))
	fmt.Printf("  Symbols removed: %d\n", len(result.Removed))
	fmt.Printf("  Symbols modified:  %d\n", len(result.Modified))

	if len(result.Added) > 0 {
		fmt.Printf("\nAdded symbols:\n")
		for _, sym := range result.Added {
			fmt.Printf("  + %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	if len(result.Removed) > 0 {
		fmt.Printf("\nRemoved symbols:\n")
		for _, sym := range result.Removed {
			fmt.Printf("  - %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	if len(result.Modified) > 0 {
		fmt.Printf("\nModified symbols:\n")
		for _, sym := range result.Modified {
			fmt.Printf("  ~ %s (%s) line %d\n", sym.Name, sym.Type, sym.Line)
		}
	}

	fmt.Printf("\n%s\n", result.Summary)
	return nil
}
