// SPDX-License-Identifier: MIT
// Purpose: ibd — Intent-Based Diffing. Compares two versions of code and
// determines if the changes match the stated intent.
// sin-debt: shrink, upgrade: when a second ibd-related function is needed, merge into a shared file
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
	Version: Version,
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
	Before      string       `json:"before"`
	After       string       `json:"after"`
	Intent      string       `json:"intent"`
	Diff        []diffLine   `json:"diff"`
	Added       []symbolInfo `json:"added"`
	Removed     []symbolInfo `json:"removed"`
	Modified    []symbolInfo `json:"modified"`
	IntentMatch string       `json:"intent_match"` // strong, partial, weak, none
	Score       int          `json:"score"`        // 0-100
	Summary     string       `json:"summary"`
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

func init() {
	RegisterVersionCmd(IbdCmd)
	IbdCmd.Flags().StringVarP(&ibdBefore, "before", "b", "", "Before version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdAfter, "after", "a", "", "After version (file, ref, or commit)")
	IbdCmd.Flags().StringVarP(&ibdIntent, "intent", "i", "", "Stated intent of the change")
	IbdCmd.Flags().StringVarP(&ibdFrom, "from", "f", "", "Git commit (old) for path target")
	IbdCmd.Flags().StringVarP(&ibdTo, "to", "t", "", "Git commit (new) for path target")
	IbdCmd.Flags().StringVarP(&ibdOutput, "output", "o", "", "Output JSON file")
	IbdCmd.Flags().StringVarP(&ibdFormat, "format", "", "text", "Output format: text|json")
}
