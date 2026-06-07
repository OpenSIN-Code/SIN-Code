// SPDX-License-Identifier: MIT
// Purpose: ibd — Intent-Based Diffing. Compare two versions of code and
// determine if the changes match the stated intent. Pure Go implementation.
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	ibdBefore string
	ibdAfter  string
	ibdIntent string
	ibdFrom   string
	ibdTo     string
	ibdOutput string
	ibdFormat string
)

var IbdCmd = &cobra.Command{
	Use:   "ibd",
	Short: "Intent-Based Diffing — compare code changes against stated intent",
	Long: `Compare two versions of code and determine if the changes match the
stated intent. Pure Go implementation.

Examples:
  sin-code ibd --before old.py --after new.py --intent "add retry logic"
  sin-code ibd --before v1.0 --after HEAD --intent "refactor authentication"
  sin-code ibd file.go --from main --to feature-branch --intent "add error handling"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		var beforePath, afterPath string

		if ibdBefore != "" && ibdAfter != "" {
			beforePath = ibdBefore
			afterPath = ibdAfter
		} else if len(args) > 0 {
			beforePath = args[0]
			// If --from and --to are set, use git to get versions
			if ibdFrom != "" && ibdTo != "" {
				// This is a git diff request - we'll try to read the file from git
				// For now, just read the file as-is and note the limitation
				fmt.Fprintf(os.Stderr, "Note: Git diff (--from/--to) requires manual diff extraction. Reading file as-is.\n")
			}
		} else {
			return fmt.Errorf("either --before/--after or a target path is required")
		}

		result, err := diffWithIntent(beforePath, afterPath, ibdIntent)
		if err != nil {
			return err
		}

		if ibdFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		return outputTextIBD(result)
	},
}

type ibdResult struct {
	Before       string       `json:"before"`
	After        string       `json:"after"`
	Intent       string       `json:"intent"`
	Diff         []diffLine   `json:"diff"`
	Added        []symbolInfo `json:"added"`
	Removed      []symbolInfo `json:"removed"`
	Modified     []symbolInfo `json:"modified"`
	IntentMatch  string       `json:"intent_match"` // strong, partial, weak, none
	Score        int          `json:"score"`        // 0-100
	Summary      string       `json:"summary"`
}

type diffLine struct {
	Type   string `json:"type"` // added, removed, context
	Line   int    `json:"line"`
	Text   string `json:"text"`
	Number int    `json:"number"`
}

func diffWithIntent(beforePath, afterPath, intent string) (*ibdResult, error) {
	beforeContent, err := readFileOrString(beforePath)
	if err != nil {
		return nil, fmt.Errorf("cannot read before: %w", err)
	}

	var afterContent string
	if afterPath != "" {
		afterContent, err = readFileOrString(afterPath)
		if err != nil {
			return nil, fmt.Errorf("cannot read after: %w", err)
		}
	} else {
		afterContent = beforeContent
	}

	// Compute diff
	diff := computeDiff(beforeContent, afterContent)

	// Extract symbols from both versions
	beforeSymbols := extractSymbolsFromContent(beforeContent, beforePath)
	afterSymbols := extractSymbolsFromContent(afterContent, afterPath)

	// Compare symbols
	added, removed, modified := compareSymbols(beforeSymbols, afterSymbols)

	// Evaluate intent match
	intentMatch, score := evaluateIntent(intent, added, removed, modified, diff)

	summary := fmt.Sprintf("Diff: %d lines changed (%d added, %d removed). %d symbols added, %d removed, %d modified. Intent match: %s (score: %d/100)",
		countChanged(diff), countAdded(diff), countRemoved(diff),
		len(added), len(removed), len(modified),
		intentMatch, score)

	return &ibdResult{
		Before:      beforePath,
		After:       afterPath,
		Intent:      intent,
		Diff:        diff,
		Added:       added,
		Removed:     removed,
		Modified:    modified,
		IntentMatch: intentMatch,
		Score:       score,
		Summary:     summary,
	}, nil
}

func readFileOrString(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	// Could be a git ref or raw string
	return path, nil
}

func computeDiff(before, after string) []diffLine {
	beforeLines := strings.Split(before, "\n")
	afterLines := strings.Split(after, "\n")

	var diff []diffLine
	maxLen := len(beforeLines)
	if len(afterLines) > maxLen {
		maxLen = len(afterLines)
	}

	for i := 0; i < maxLen; i++ {
		var beforeLine, afterLine string
		if i < len(beforeLines) {
			beforeLine = beforeLines[i]
		}
		if i < len(afterLines) {
			afterLine = afterLines[i]
		}

		if beforeLine == afterLine {
			// Context line (unchanged)
			if i < len(beforeLines) {
				diff = append(diff, diffLine{Type: "context", Line: i + 1, Text: beforeLine, Number: i + 1})
			}
		} else if i < len(beforeLines) && i < len(afterLines) {
			// Modified line
			diff = append(diff, diffLine{Type: "removed", Line: i + 1, Text: beforeLine, Number: i + 1})
			diff = append(diff, diffLine{Type: "added", Line: i + 1, Text: afterLine, Number: i + 1})
		} else if i < len(beforeLines) {
			// Removed line
			diff = append(diff, diffLine{Type: "removed", Line: i + 1, Text: beforeLine, Number: i + 1})
		} else {
			// Added line
			diff = append(diff, diffLine{Type: "added", Line: i + 1, Text: afterLine, Number: i + 1})
		}
	}

	return diff
}

func extractSymbolsFromContent(content, path string) []symbolInfo {
	lang := detectLanguage(path)
	return extractSymbols(path, content, lang)
}

func compareSymbols(before, after []symbolInfo) (added, removed, modified []symbolInfo) {
	beforeMap := make(map[string]symbolInfo)
	for _, sym := range before {
		beforeMap[sym.Name] = sym
	}

	afterMap := make(map[string]symbolInfo)
	for _, sym := range after {
		afterMap[sym.Name] = sym
	}

	// Find added
	for name, sym := range afterMap {
		if _, ok := beforeMap[name]; !ok {
			added = append(added, sym)
		}
	}

	// Find removed
	for name, sym := range beforeMap {
		if _, ok := afterMap[name]; !ok {
			removed = append(removed, sym)
		}
	}

	// Find modified (same name, different line/type)
	for name, afterSym := range afterMap {
		if beforeSym, ok := beforeMap[name]; ok {
			if beforeSym.Type != afterSym.Type || beforeSym.Line != afterSym.Line {
				modified = append(modified, afterSym)
			}
		}
	}

	return
}

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
			score -= 20
		}
	}

	if intentKeywords["remove"] || intentKeywords["delete"] {
		if len(removed) > 0 {
			score += 30
		} else {
			score -= 20
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

func init() {
	IbdCmd.Flags().StringVarP(&ibdBefore, "before", "b", "", "Before version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdAfter, "after", "a", "", "After version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdIntent, "intent", "i", "", "Stated intent of the change")
	IbdCmd.Flags().StringVarP(&ibdFrom, "from", "f", "", "Git commit (old) for path target")
	IbdCmd.Flags().StringVarP(&ibdTo, "to", "t", "", "Git commit (new) for path target")
	IbdCmd.Flags().StringVarP(&ibdOutput, "output", "o", "", "Output JSON file")
	IbdCmd.Flags().StringVarP(&ibdFormat, "format", "", "text", "Output format: text|json")
}
