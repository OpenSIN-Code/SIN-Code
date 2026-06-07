// SPDX-License-Identifier: MIT
// Purpose: Unit tests for the ibd (Intent-Based Diffing) subcommand.
package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeDiff_Identical(t *testing.T) {
	diff := computeDiff("hello\nworld\n", "hello\nworld\n")
	for _, d := range diff {
		if d.Type != "context" {
			t.Errorf("expected all context lines for identical content, got %q at line %d", d.Type, d.Line)
		}
	}
}

func TestComputeDiff_AddedLines(t *testing.T) {
	diff := computeDiff("line1\n", "line1\nline2\n")
	added := 0
	for _, d := range diff {
		if d.Type == "added" {
			added++
		}
	}
	if added != 1 {
		t.Errorf("expected 1 added line, got %d", added)
	}
}

func TestComputeDiff_RemovedLines(t *testing.T) {
	diff := computeDiff("line1\nline2\nline3\n", "line1\nline3\n")
	removed := 0
	for _, d := range diff {
		if d.Type == "removed" {
			removed++
		}
	}
	if removed != 2 {
		t.Errorf("expected 2 removed lines, got %d", removed)
	}
}

func TestComputeDiff_ModifiedLine(t *testing.T) {
	diff := computeDiff("old line\n", "new line\n")
	removed := 0
	added := 0
	for _, d := range diff {
		if d.Type == "removed" {
			removed++
		}
		if d.Type == "added" {
			added++
		}
	}
	if removed != 1 || added != 1 {
		t.Errorf("expected 1 removed + 1 added for modified line, got %d removed, %d added", removed, added)
	}
}

func TestComputeDiff_EmptyBefore(t *testing.T) {
	diff := computeDiff("", "line1\nline2\n")
	added := 0
	for _, d := range diff {
		if d.Type == "added" {
			added++
		}
	}
	if added < 2 {
		t.Errorf("expected at least 2 added lines for empty before, got %d", added)
	}
}

func TestComputeDiff_EmptyAfter(t *testing.T) {
	diff := computeDiff("line1\nline2\n", "")
	removed := 0
	for _, d := range diff {
		if d.Type == "removed" {
			removed++
		}
	}
	if removed < 2 {
		t.Errorf("expected at least 2 removed lines for empty after, got %d", removed)
	}
}

func TestComputeDiff_BothEmpty(t *testing.T) {
	diff := computeDiff("", "")
	changed := 0
	for _, d := range diff {
		if d.Type != "context" {
			changed++
		}
	}
	if changed != 0 {
		t.Errorf("expected 0 changed lines for both empty, got %d", changed)
	}
}

func TestCountChanged(t *testing.T) {
	diff := []diffLine{
		{Type: "context", Line: 1, Text: "same", Number: 1},
		{Type: "added", Line: 2, Text: "new", Number: 2},
		{Type: "removed", Line: 3, Text: "old", Number: 3},
	}
	if c := countChanged(diff); c != 2 {
		t.Errorf("expected 2 changed lines, got %d", c)
	}
}

func TestCountAdded(t *testing.T) {
	diff := []diffLine{
		{Type: "context", Line: 1, Text: "same", Number: 1},
		{Type: "added", Line: 2, Text: "new", Number: 2},
		{Type: "removed", Line: 3, Text: "old", Number: 3},
		{Type: "added", Line: 4, Text: "new2", Number: 4},
	}
	if c := countAdded(diff); c != 2 {
		t.Errorf("expected 2 added lines, got %d", c)
	}
}

func TestCountRemoved(t *testing.T) {
	diff := []diffLine{
		{Type: "context", Line: 1, Text: "same", Number: 1},
		{Type: "removed", Line: 2, Text: "old", Number: 2},
		{Type: "removed", Line: 3, Text: "old2", Number: 3},
		{Type: "added", Line: 4, Text: "new", Number: 4},
	}
	if c := countRemoved(diff); c != 2 {
		t.Errorf("expected 2 removed lines, got %d", c)
	}
}

func TestReadFileOrString_Empty(t *testing.T) {
	content, err := readFileOrString("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "" {
		t.Errorf("expected empty string for empty path, got %q", content)
	}
}

func TestReadFileOrString_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "test.txt")
	os.WriteFile(f, []byte("hello"), 0644)
	content, err := readFileOrString(f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "hello" {
		t.Errorf("expected 'hello', got %q", content)
	}
}

func TestReadFileOrString_NonExistentPath(t *testing.T) {
	content, err := readFileOrString("not-a-file-ref")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content != "not-a-file-ref" {
		t.Errorf("expected input returned as-is, got %q", content)
	}
}

func TestCompareSymbols(t *testing.T) {
	before := []symbolInfo{
		{Name: "Add", Type: "function", Line: 1},
		{Name: "Remove", Type: "function", Line: 5},
		{Name: "Update", Type: "function", Line: 10},
	}
	after := []symbolInfo{
		{Name: "Add", Type: "function", Line: 1},
		{Name: "Update", Type: "function", Line: 15},
		{Name: "Create", Type: "function", Line: 20},
	}
	added, removed, modified := compareSymbols(before, after)

	if len(added) != 1 || added[0].Name != "Create" {
		t.Errorf("expected 1 added symbol 'Create', got %v", added)
	}
	if len(removed) != 1 || removed[0].Name != "Remove" {
		t.Errorf("expected 1 removed symbol 'Remove', got %v", removed)
	}
	if len(modified) != 1 || modified[0].Name != "Update" {
		t.Errorf("expected 1 modified symbol 'Update', got %v", modified)
	}
}

func TestCompareSymbols_Identical(t *testing.T) {
	syms := []symbolInfo{
		{Name: "Add", Type: "function", Line: 1},
		{Name: "Remove", Type: "function", Line: 5},
	}
	added, removed, modified := compareSymbols(syms, syms)
	if len(added) != 0 {
		t.Errorf("expected 0 added, got %d", len(added))
	}
	if len(removed) != 0 {
		t.Errorf("expected 0 removed, got %d", len(removed))
	}
	if len(modified) != 0 {
		t.Errorf("expected 0 modified, got %d", len(modified))
	}
}

func TestCompareSymbols_Empty(t *testing.T) {
	added, removed, modified := compareSymbols(nil, nil)
	if len(added) != 0 || len(removed) != 0 || len(modified) != 0 {
		t.Errorf("expected all zero for empty inputs, got added=%d removed=%d modified=%d", len(added), len(removed), len(modified))
	}
}

func TestEvaluateIntent_Empty(t *testing.T) {
	match, score := evaluateIntent("", nil, nil, nil, nil)
	if match != "unknown" {
		t.Errorf("expected 'unknown' match for empty intent, got %q", match)
	}
	if score != 50 {
		t.Errorf("expected score 50 for empty intent, got %d", score)
	}
}

func TestEvaluateIntent_AddKeywordWithAdditions(t *testing.T) {
	added := []symbolInfo{{Name: "NewFunc", Type: "function", Line: 5}}
	match, score := evaluateIntent("add new feature", added, nil, nil, nil)
	if match == "none" {
		t.Errorf("expected non-none match when adding symbols, got %q", match)
	}
	if score <= 50 {
		t.Errorf("expected score > 50 when intent matches changes, got %d", score)
	}
}

func TestEvaluateIntent_AddKeywordNoAdditions(t *testing.T) {
	_, score := evaluateIntent("add new feature", nil, nil, nil, nil)
	if score >= 50 {
		t.Errorf("expected score < 50 when intent says add but no additions, got %d", score)
	}
}

func TestEvaluateIntent_RemoveKeywordWithRemovals(t *testing.T) {
	removed := []symbolInfo{{Name: "OldFunc", Type: "function", Line: 3}}
	match, score := evaluateIntent("remove deprecated code", nil, removed, nil, nil)
	if match == "none" {
		t.Errorf("expected non-none match, got %q", match)
	}
	if score <= 50 {
		t.Errorf("expected score > 50, got %d", score)
	}
}

func TestEvaluateIntent_FixKeywordWithModifications(t *testing.T) {
	modified := []symbolInfo{{Name: "BugFunc", Type: "function", Line: 10}}
	match, score := evaluateIntent("fix bug in BugFunc", nil, nil, modified, nil)
	if match == "none" {
		t.Errorf("expected non-none match, got %q", match)
	}
	if score <= 50 {
		t.Errorf("expected score > 50, got %d", score)
	}
}

func TestEvaluateIntent_RenameKeywordWithAddRemove(t *testing.T) {
	added := []symbolInfo{{Name: "processData", Type: "function", Line: 5}}
	removed := []symbolInfo{{Name: "process", Type: "function", Line: 3}}
	match, score := evaluateIntent("rename process to processData", added, removed, nil, nil)
	if match == "none" {
		t.Errorf("expected non-none match for rename, got %q", match)
	}
	if score <= 50 {
		t.Errorf("expected score > 50 for matching rename, got %d", score)
	}
}

func TestEvaluateIntent_ErrorHandlingInDiff(t *testing.T) {
	diff := []diffLine{
		{Type: "added", Line: 5, Text: "try {", Number: 5},
		{Type: "added", Line: 6, Text: "catch (e) {", Number: 6},
	}
	_, score := evaluateIntent("fix error handling", nil, nil, []symbolInfo{{Name: "Func", Type: "function", Line: 1}}, diff)
	if score <= 50 {
		t.Errorf("expected score > 50, got %d", score)
	}
}

func TestEvaluateIntent_RetryInDiff(t *testing.T) {
	diff := []diffLine{
		{Type: "added", Line: 3, Text: "retry(options)", Number: 3},
	}
	_, score := evaluateIntent("implement retry logic", []symbolInfo{{Name: "RetryFunc", Type: "function", Line: 3}}, nil, nil, diff)
	if score <= 50 {
		t.Errorf("expected score > 50 for retry in diff, got %d", score)
	}
}

func TestEvaluateIntent_TestKeywordWithTestSymbols(t *testing.T) {
	added := []symbolInfo{{Name: "TestAuth", Type: "function", Line: 1}}
	_, score := evaluateIntent("add tests for auth", added, nil, nil, nil)
	if score <= 50 {
		t.Errorf("expected score > 50 for test symbols, got %d", score)
	}
}

func TestEvaluateIntent_ScoreCapped(t *testing.T) {
	added := make([]symbolInfo, 50)
	for i := range added {
		added[i] = symbolInfo{Name: fmt.Sprintf("Func%d", i), Type: "function", Line: i}
	}
	modified := make([]symbolInfo, 50)
	for i := range modified {
		modified[i] = symbolInfo{Name: fmt.Sprintf("Mod%d", i), Type: "function", Line: i}
	}
	_, score := evaluateIntent("add create implement fix optimize improve refactor modify change update", added, nil, modified, nil)
	if score > 100 {
		t.Errorf("expected score capped at 100, got %d", score)
	}
}

func TestEvaluateIntent_ScoreFloored(t *testing.T) {
	_, score := evaluateIntent("add create implement", nil, nil, nil, nil)
	if score < 0 {
		t.Errorf("expected score floored at 0, got %d", score)
	}
}

func TestDiffWithIntent_Files(t *testing.T) {
	dir := t.TempDir()
	beforeFile := filepath.Join(dir, "before.py")
	afterFile := filepath.Join(dir, "after.py")
	os.WriteFile(beforeFile, []byte("def old(): pass\n"), 0644)
	os.WriteFile(afterFile, []byte("def new(): pass\n"), 0644)

	result, err := diffWithIntent(beforeFile, afterFile, "remove old and add new")
	if err != nil {
		t.Fatalf("diffWithIntent failed: %v", err)
	}
	if result.Before != beforeFile {
		t.Errorf("expected before=%s, got %s", beforeFile, result.Before)
	}
	if result.After != afterFile {
		t.Errorf("expected after=%s, got %s", afterFile, result.After)
	}
	if result.Intent != "remove old and add new" {
		t.Errorf("expected intent='remove old and add new', got %q", result.Intent)
	}
	if len(result.Diff) == 0 {
		t.Error("expected non-empty diff")
	}
	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestDiffWithIntent_MissingBefore(t *testing.T) {
	result, err := diffWithIntent("/nonexistent/before.py", "/nonexistent/after.py", "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Before != "/nonexistent/before.py" {
		t.Errorf("expected before path as-is, got %q", result.Before)
	}
}

func TestDiffWithIntent_EmptyAfter(t *testing.T) {
	dir := t.TempDir()
	beforeFile := filepath.Join(dir, "before.py")
	os.WriteFile(beforeFile, []byte("def old(): pass\n"), 0644)

	result, err := diffWithIntent(beforeFile, "", "remove old")
	if err != nil {
		t.Fatalf("diffWithIntent failed: %v", err)
	}
	if result.After != "" {
		t.Errorf("expected empty after path, got %q", result.After)
	}
}

func TestDiffWithIntent_SameFileContent(t *testing.T) {
	dir := t.TempDir()
	beforeFile := filepath.Join(dir, "same.py")
	afterFile := filepath.Join(dir, "same_after.py")
	content := "def hello(): pass\n"
	os.WriteFile(beforeFile, []byte(content), 0644)
	os.WriteFile(afterFile, []byte(content), 0644)

	result, err := diffWithIntent(beforeFile, afterFile, "no changes")
	if err != nil {
		t.Fatalf("diffWithIntent failed: %v", err)
	}
	if countChanged(result.Diff) != 0 {
		t.Errorf("expected 0 changed lines for identical content, got %d", countChanged(result.Diff))
	}
}

func TestOutputTextIBD(t *testing.T) {
	result := &ibdResult{
		Before:      "old.py",
		After:       "new.py",
		Intent:      "add retry logic",
		IntentMatch: "strong",
		Score:       85,
		Diff: []diffLine{
			{Type: "added", Line: 3, Text: "retry()", Number: 3},
		},
		Added:   []symbolInfo{{Name: "RetryFunc", Type: "function", Line: 3}},
		Removed: []symbolInfo{{Name: "OldFunc", Type: "function", Line: 1}},
		Modified: []symbolInfo{{Name: "MainFunc", Type: "function", Line: 5}},
		Summary:  "Test summary",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextIBD(result); err != nil {
		t.Fatalf("outputTextIBD failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Intent-Based Diffing") {
		t.Errorf("expected header in output, got %q", out)
	}
	if !strings.Contains(out, "old.py") {
		t.Errorf("expected 'old.py' in output, got %q", out)
	}
	if !strings.Contains(out, "strong") {
		t.Errorf("expected 'strong' in output, got %q", out)
	}
	if !strings.Contains(out, "RetryFunc") {
		t.Errorf("expected 'RetryFunc' in added symbols, got %q", out)
	}
	if !strings.Contains(out, "OldFunc") {
		t.Errorf("expected 'OldFunc' in removed symbols, got %q", out)
	}
	if !strings.Contains(out, "MainFunc") {
		t.Errorf("expected 'MainFunc' in modified symbols, got %q", out)
	}
}

func TestOutputTextIBD_NoChanges(t *testing.T) {
	result := &ibdResult{
		Before:      "a.py",
		After:       "b.py",
		Intent:      "",
		IntentMatch: "unknown",
		Score:       50,
		Diff:        []diffLine{},
		Summary:     "No changes",
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	if err := outputTextIBD(result); err != nil {
		t.Fatalf("outputTextIBD failed: %v", err)
	}
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "Intent-Based Diffing") {
		t.Errorf("expected header in output, got %q", out)
	}
}

func TestIbdCmd_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	beforeFile := filepath.Join(dir, "before.py")
	afterFile := filepath.Join(dir, "after.py")
	os.WriteFile(beforeFile, []byte("def old(): pass\n"), 0644)
	os.WriteFile(afterFile, []byte("def new(): pass\n"), 0644)

	ibdBefore = beforeFile
	ibdAfter = afterFile
	ibdIntent = "refactor code"
	ibdFormat = "json"

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := IbdCmd.RunE(IbdCmd, []string{})
	w.Close()
	os.Stdout = oldStdout

	if err != nil {
		t.Fatalf("IbdCmd.RunE failed: %v", err)
	}

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	var result ibdResult
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("expected valid JSON output, got parse error: %v", err)
	}
	if result.Intent != "refactor code" {
		t.Errorf("expected intent='refactor code', got %q", result.Intent)
	}
}

func TestIbdCmd_NoArgsReturnsError(t *testing.T) {
	ibdBefore = ""
	ibdAfter = ""
	ibdIntent = ""
	ibdFormat = "text"
	err := IbdCmd.RunE(IbdCmd, []string{})
	if err == nil {
		t.Error("expected error when --before and --after are missing")
	}
}

func TestIbdCmd_ArgsPathWithFromTo(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "target.go")
	os.WriteFile(f, []byte("package main\nfunc main() {}\n"), 0644)

	ibdBefore = ""
	ibdAfter = ""
	ibdFrom = "main"
	ibdTo = "feature"
	ibdIntent = "add feature"
	ibdFormat = "text"

	oldStdout := os.Stdout
	oldStderr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	err := IbdCmd.RunE(IbdCmd, []string{f})
	wOut.Close()
	rOut.Close()
	wErr.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	if err != nil {
		t.Fatalf("IbdCmd.RunE failed: %v", err)
	}

	var errBuf bytes.Buffer
	errBuf.ReadFrom(rErr)
	stderr := errBuf.String()
	if !strings.Contains(stderr, "Note: Git diff") {
		t.Errorf("expected note about git diff, got %q", stderr)
	}
}
