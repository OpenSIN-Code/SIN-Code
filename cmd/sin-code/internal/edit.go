// SPDX-License-Identifier: MIT
// Purpose: edit — hashline-anchored surgical file editing. Replaces fragile
// native string/line editors: anchors carry a content hash so stale edits
// fail loudly instead of corrupting files, drift up to ±25 lines is
// auto-resolved, occurrence counting prevents ambiguous string replaces, and
// every edit re-runs the atomic write path with syntax validation.
// Docs: cmd/sin-code/internal/edit.doc.md
package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var (
	editAnchor     string
	editEndAnchor  string
	editNewText    string
	editOldString  string
	editNewString  string
	editReplaceAll bool
	editInsert     string
	editDelete     bool
	editDryRun     bool
	editNoValidate bool
	editDrift      int
	editFormat     string
)

var EditCmd = &cobra.Command{
	Use:   "edit [path]",
	Short: "Hashline-anchored surgical edits with validation",
	Long: `Surgical file editing with two addressing modes:

Anchor mode (preferred — anchors come from 'sin-code read'):
  --anchor 12:ab34cd56 --new-text "replacement"        replace one line
  --anchor 12:ab34cd56 --end-anchor 20:ef99aa01 ...    replace a line range
  --anchor 12:ab34cd56 --insert after --new-text "..." insert after a line
  --anchor 12:ab34cd56 --delete                        delete line (or range)

String mode (exact match, fails on ambiguity):
  --old-string "foo(a, b)" --new-string "foo(a, b, c)"
  --old-string "x" --new-string "y" --replace-all

Every edit validates syntax (like 'sin-code write') and applies atomically.
--dry-run prints a unified diff without touching the file.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		absPath, err := filepath.Abs(args[0])
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		req := editRequest{
			Anchor: editAnchor, EndAnchor: editEndAnchor, NewText: editNewText,
			OldString: editOldString, NewString: editNewString,
			ReplaceAll: editReplaceAll, Insert: editInsert, Delete: editDelete,
			DryRun: editDryRun, Validate: !editNoValidate, Drift: editDrift,
		}
		result, err := applyEdit(absPath, req)
		if err != nil {
			return err
		}
		if editFormat == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(result)
		}
		if result.DryRun {
			fmt.Print(result.Diff)
			return nil
		}
		fmt.Printf("edited %s: %s (%+d lines)\n", result.Path, result.Operation, result.LineDelta)
		if result.Diff != "" {
			fmt.Print(result.Diff)
		}
		return nil
	},
}

func init() {
	EditCmd.Flags().StringVar(&editAnchor, "anchor", "", "Hashline anchor LINE:HASH (from 'sin-code read')")
	EditCmd.Flags().StringVar(&editEndAnchor, "end-anchor", "", "End anchor for range operations (inclusive)")
	EditCmd.Flags().StringVar(&editNewText, "new-text", "", "Replacement/insertion text (may be multi-line)")
	EditCmd.Flags().StringVar(&editOldString, "old-string", "", "Exact string to replace (string mode)")
	EditCmd.Flags().StringVar(&editNewString, "new-string", "", "Replacement string (string mode)")
	EditCmd.Flags().BoolVar(&editReplaceAll, "replace-all", false, "Replace every occurrence (string mode)")
	EditCmd.Flags().StringVar(&editInsert, "insert", "", "Insert relative to anchor: before, after")
	EditCmd.Flags().BoolVar(&editDelete, "delete", false, "Delete the anchored line or range")
	EditCmd.Flags().BoolVar(&editDryRun, "dry-run", false, "Print diff without writing")
	EditCmd.Flags().BoolVar(&editNoValidate, "no-validate", false, "Skip syntax validation of the result")
	EditCmd.Flags().IntVar(&editDrift, "drift", DefaultDriftWindow, "Anchor drift tolerance in lines")
	EditCmd.Flags().StringVarP(&editFormat, "format", "f", "text", "Output: text, json")
}

type editRequest struct {
	Anchor     string
	EndAnchor  string
	NewText    string
	OldString  string
	NewString  string
	ReplaceAll bool
	Insert     string
	Delete     bool
	DryRun     bool
	Validate   bool
	Drift      int
}

type editResult struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
	LineDelta int    `json:"line_delta"`
	Drift     int    `json:"anchor_drift,omitempty"`
	DryRun    bool   `json:"dry_run"`
	Diff      string `json:"diff"`
}

func applyEdit(path string, req editRequest) (*editResult, error) {
	anchorMode := req.Anchor != ""
	stringMode := req.OldString != ""
	if anchorMode == stringMode {
		return nil, fmt.Errorf("exactly one addressing mode required: --anchor LINE:HASH or --old-string")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	original := string(data)
	lines, trailingNL := SplitLines(original)

	res := &editResult{Path: path, DryRun: req.DryRun}
	var updated []string

	if anchorMode {
		updated, err = applyAnchorEdit(lines, req, res)
	} else {
		updated, err = applyStringEdit(lines, original, req, res, &trailingNL)
	}
	if err != nil {
		return nil, err
	}

	newContent := JoinLines(updated, trailingNL)
	res.LineDelta = len(updated) - len(lines)
	res.Diff = unifiedDiff(path, lines, updated)

	if req.Validate {
		if err := validateSyntax(path, newContent); err != nil {
			return nil, fmt.Errorf("edit would break syntax, nothing written: %w\ndiff that was rejected:\n%s", err, res.Diff)
		}
	}
	if req.DryRun {
		return res, nil
	}
	if _, err := writeFileAtomic(path, newContent, writeOpts{validate: false}); err != nil {
		return nil, err
	}
	return res, nil
}

func applyAnchorEdit(lines []string, req editRequest, res *editResult) ([]string, error) {
	start, err := ParseAnchor(req.Anchor)
	if err != nil {
		return nil, err
	}
	startIdx, drift, err := ResolveAnchor(lines, start, req.Drift)
	if err != nil {
		return nil, err
	}
	res.Drift = drift

	endIdx := startIdx
	if req.EndAnchor != "" {
		end, err := ParseAnchor(req.EndAnchor)
		if err != nil {
			return nil, fmt.Errorf("end anchor: %w", err)
		}
		endIdx, _, err = ResolveAnchor(lines, end, req.Drift)
		if err != nil {
			return nil, fmt.Errorf("end anchor: %w", err)
		}
		if endIdx < startIdx {
			return nil, fmt.Errorf("end anchor (line %d) precedes start anchor (line %d)", endIdx+1, startIdx+1)
		}
	}

	newLines, _ := SplitLines(req.NewText)
	if req.NewText == "" {
		newLines = []string{}
	} else if !strings.HasSuffix(req.NewText, "\n") {
		newLines, _ = SplitLines(req.NewText + "\n")
	}

	out := make([]string, 0, len(lines)+len(newLines))
	switch {
	case req.Delete:
		res.Operation = fmt.Sprintf("delete lines %d-%d", startIdx+1, endIdx+1)
		out = append(out, lines[:startIdx]...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert == "before":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) before line %d", len(newLines), startIdx+1)
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[startIdx:]...)
	case req.Insert == "after":
		if len(newLines) == 0 {
			return nil, fmt.Errorf("--insert requires --new-text")
		}
		res.Operation = fmt.Sprintf("insert %d line(s) after line %d", len(newLines), endIdx+1)
		out = append(out, lines[:endIdx+1]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	case req.Insert != "":
		return nil, fmt.Errorf("invalid --insert %q: want before or after", req.Insert)
	default:
		if req.NewText == "" {
			return nil, fmt.Errorf("replace requires --new-text (use --delete to remove lines)")
		}
		res.Operation = fmt.Sprintf("replace lines %d-%d with %d line(s)", startIdx+1, endIdx+1, len(newLines))
		out = append(out, lines[:startIdx]...)
		out = append(out, newLines...)
		out = append(out, lines[endIdx+1:]...)
	}
	return out, nil
}

func applyStringEdit(lines []string, original string, req editRequest, res *editResult, trailingNL *bool) ([]string, error) {
	count := strings.Count(original, req.OldString)
	if count == 0 {
		return nil, fmt.Errorf("old string not found — re-read the file, content may have changed")
	}
	if count > 1 && !req.ReplaceAll {
		return nil, fmt.Errorf("old string matches %d times — add surrounding context to disambiguate, or pass --replace-all", count)
	}

	var newContent string
	if req.ReplaceAll {
		newContent = strings.ReplaceAll(original, req.OldString, req.NewString)
		res.Operation = fmt.Sprintf("replace %d occurrence(s)", count)
	} else {
		newContent = strings.Replace(original, req.OldString, req.NewString, 1)
		res.Operation = "replace 1 occurrence"
	}
	updated, tnl := SplitLines(newContent)
	*trailingNL = tnl
	return updated, nil
}

func unifiedDiff(path string, before, after []string) string {
	p := 0
	for p < len(before) && p < len(after) && before[p] == after[p] {
		p++
	}
	s := 0
	for s < len(before)-p && s < len(after)-p && before[len(before)-1-s] == after[len(after)-1-s] {
		s++
	}
	bStart, bEnd := p, len(before)-s
	aStart, aEnd := p, len(after)-s
	if bStart == bEnd && aStart == aEnd {
		return ""
	}

	ctx := 2
	cStart := bStart - ctx
	if cStart < 0 {
		cStart = 0
	}
	cEndB := bEnd + ctx
	if cEndB > len(before) {
		cEndB = len(before)
	}

	var buf strings.Builder
	fmt.Fprintf(&buf, "--- %s\n+++ %s\n", path, path)
	fmt.Fprintf(&buf, "@@ -%d,%d +%d,%d @@\n", cStart+1, cEndB-cStart, cStart+1, (aEnd-aStart)+(cEndB-cStart)-(bEnd-bStart))
	for i := cStart; i < bStart; i++ {
		fmt.Fprintf(&buf, " %s\n", before[i])
	}
	for i := bStart; i < bEnd; i++ {
		fmt.Fprintf(&buf, "-%s\n", before[i])
	}
	for i := aStart; i < aEnd; i++ {
		fmt.Fprintf(&buf, "+%s\n", after[i])
	}
	for i := bEnd; i < cEndB; i++ {
		fmt.Fprintf(&buf, " %s\n", before[i])
	}
	return buf.String()
}
