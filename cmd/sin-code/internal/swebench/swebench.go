// SPDX-License-Identifier: MIT
package swebench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Instance struct {
	InstanceID       string   `json:"instance_id"`
	Repo             string   `json:"repo"`
	BaseCommit       string   `json:"base_commit"`
	ProblemStatement string   `json:"problem_statement"`
	Patch            string   `json:"patch"`
	TestPatch        string   `json:"test_patch"`
	FailToPass       []string `json:"FAIL_TO_PASS"`
	PassToPass       []string `json:"PASS_TO_PASS"`
	Version          string   `json:"version"`
}

type Dataset struct { Instances []Instance `json:"instances"` }

func LoadDataset(path string) (*Dataset, error) {
	raw, err := os.ReadFile(path)
	if err != nil { return nil, fmt.Errorf("read: %w", err) }
	var instances []Instance
	if err := json.Unmarshal(raw, &instances); err == nil { return &Dataset{Instances: instances}, nil }
	var d Dataset
	if err := json.Unmarshal(raw, &d); err != nil { return nil, fmt.Errorf("parse: %w", err) }
	return &d, nil
}

type TestCase struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	Tags        []string          `json:"tags"`
	Constraints map[string]any    `json:"constraints"`
	Expected    map[string]any    `json:"expected"`
	VerifyCmd   string            `json:"verify_cmd"`
	Metadata    map[string]string `json:"metadata"`
}

func (inst *Instance) ToTestCase() TestCase {
	return TestCase{
		ID: inst.InstanceID, Description: "SWE-bench: " + inst.InstanceID,
		Prompt: fmt.Sprintf("Resolve issue %s in repo %s.\n\n%s\n\nTests: %s\nExisting: %s", inst.InstanceID, inst.Repo, inst.ProblemStatement, strings.Join(inst.FailToPass, ","), strings.Join(inst.PassToPass, ",")),
		Tags: []string{"swe-bench"}, Constraints: map[string]any{"require_verify": true, "max_turns": 60},
		Expected: map[string]any{"custom_criteria": "All tests pass"}, VerifyCmd: buildVerifyCmd(inst),
		Metadata: map[string]string{"repo": inst.Repo, "instance_id": inst.InstanceID},
	}
}

func buildVerifyCmd(inst *Instance) string {
	var parts []string
	for _, t := range inst.FailToPass { parts = append(parts, "python -m pytest "+t+" -x") }
	for _, t := range inst.PassToPass { parts = append(parts, "python -m pytest "+t+" -x") }
	return strings.Join(parts, " && ")
}

func ConvertDataset(ds *Dataset) []TestCase {
	cases := make([]TestCase, len(ds.Instances))
	for i, inst := range ds.Instances { cases[i] = inst.ToTestCase() }
	return cases
}

func WriteEvalDataset(cases []TestCase, outPath string) error {
	raw, err := json.MarshalIndent(map[string]any{"name": "swe-bench", "version": "1.0", "test_cases": cases}, "", "  ")
	if err != nil { return err }
	os.MkdirAll(filepath.Dir(outPath), 0o755)
	return os.WriteFile(outPath, raw, 0o644)
}

type ScorerResult struct {
	InstanceID      string  `json:"instance_id"`
	Resolved        bool    `json:"resolved"`
	FailToPass      int     `json:"fail_to_pass_passed"`
	FailToPassTotal int     `json:"fail_to_pass_total"`
	PassToPass      int     `json:"pass_to_pass_passed"`
	PassToPassTotal int     `json:"pass_to_pass_total"`
	Score           float64 `json:"score"`
}

func isWordChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func containsStandaloneOk(line string) bool {
	search := line
	for {
		idx := strings.Index(search, "ok")
		if idx == -1 {
			return false
		}
		absIdx := len(line) - len(search) + idx
		beforeOk := absIdx == 0 || !isWordChar(line[absIdx-1])
		afterIdx := absIdx + 2
		afterOk := afterIdx >= len(line) || !isWordChar(line[afterIdx])
		if beforeOk && afterOk {
			return true
		}
		search = search[idx+2:]
	}
}

func lineHasPassIndicator(line string) bool {
	if strings.Contains(line, "PASSED") {
		return true
	}
	if strings.Contains(line, "passed") {
		return true
	}
	return containsStandaloneOk(line)
}

func lineHasFailIndicator(line string) bool {
	for _, ind := range []string{"FAILED", "ERROR", "failed", "error"} {
		if strings.Contains(line, ind) {
			return true
		}
	}
	return false
}

func testPassed(testName, verifyOutput string) bool {
	for _, line := range strings.Split(verifyOutput, "\n") {
		if !strings.Contains(line, testName) {
			continue
		}
		if lineHasFailIndicator(line) {
			continue
		}
		if lineHasPassIndicator(line) {
			return true
		}
	}
	return false
}

func ScoreInstance(inst *Instance, verifyOutput string) ScorerResult {
	r := ScorerResult{InstanceID: inst.InstanceID, FailToPassTotal: len(inst.FailToPass), PassToPassTotal: len(inst.PassToPass)}
	for _, t := range inst.FailToPass {
		if testPassed(t, verifyOutput) {
			r.FailToPass++
		}
	}
	for _, t := range inst.PassToPass {
		if testPassed(t, verifyOutput) {
			r.PassToPass++
		}
	}
	if r.FailToPassTotal > 0 {
		ftpr := float64(r.FailToPass) / float64(r.FailToPassTotal)
		ptpr := 1.0; if r.PassToPassTotal > 0 { ptpr = float64(r.PassToPass) / float64(r.PassToPassTotal) }
		r.Score = 0.7*ftpr + 0.3*ptpr
		r.Resolved = r.FailToPass == r.FailToPassTotal && r.PassToPass == r.PassToPassTotal
	}
	return r
}

type Summary struct {
	Total       int            `json:"total"`
	Resolved    int            `json:"resolved"`
	ResolveRate float64        `json:"resolve_rate"`
	MeanScore   float64        `json:"mean_score"`
	Results     []ScorerResult `json:"results"`
}

func SummarizeResults(results []ScorerResult) Summary {
	s := Summary{Results: results, Total: len(results)}
	ts := 0.0
	for _, r := range results { if r.Resolved { s.Resolved++ }; ts += r.Score }
	if s.Total > 0 { s.ResolveRate = float64(s.Resolved) / float64(s.Total); s.MeanScore = ts / float64(s.Total) }
	return s
}
