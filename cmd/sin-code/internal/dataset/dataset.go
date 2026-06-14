// SPDX-License-Identifier: MIT
// Purpose: pure-stdlib Golden Dataset language (issue #75, mandate M2).
// A Dataset is a JSON file under evals/*.json with a name, version,
// and an array of TestCase. Each TestCase is one evaluation task.
//
// The parser is intentionally stdlib-only (json + filepath + strings).
// Anything more would force a "vendoring" debate and undo the
// "NOT a place to vendor" mandate.
//
// TestCase fields map 1:1 to eval/runner.go's evaluation surface:
// constraints enforce hard rules (must_use_tools, forbidden_tools,
// max_turns, require_verify + verify_cmd). Expected rules enforce
// post-hoc output gates (contains / avoids keywords).
//
// Docs: dataset.doc.md
package dataset

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Dataset is a deployable, versioned collection of TestCase items.
// Version is opaque — JSON schema treats it as a free-form string so it
// can also hold semver-with-prefix strings ("v1.0.0", "1.0.0-rc1").
type Dataset struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description,omitempty"`
	TestCases   []TestCase `json:"test_cases"`
}

// TestCase is one evaluation task. Prompt is required; the rest are
// optional with sensible defaults enforced by Validate().
type TestCase struct {
	ID          string            `json:"id"`
	Description string            `json:"description,omitempty"`
	Prompt      string            `json:"prompt"`
	Tags        []string          `json:"tags,omitempty"`
	Constraints Constraints       `json:"constraints,omitempty"`
	Expected    Expected          `json:"expected,omitempty"`
	VerifyCmd   string            `json:"verify_cmd,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Constraints holds hard rules. All fields are optional; Validate
// makes sure they are consistent (e.g. require_verify + verify_cmd
// must both be set).
type Constraints struct {
	MustUseTools   []string `json:"must_use_tools,omitempty"`
	ForbiddenTools []string `json:"forbidden_tools,omitempty"`
	MaxTurns       int      `json:"max_turns,omitempty"`
	MaxTokens      int      `json:"max_tokens,omitempty"`
	RequireVerify  bool     `json:"require_verify,omitempty"`
	Timeout        string   `json:"timeout,omitempty"` // e.g. "5m", "10m"
}

// Expected holds post-hoc output gates. OutputContains / OutputAvoids
// are simple substring checks; MinQuality is reserved for the eventual
// LLM-as-a-Judge integration (judge config owns the threshold).
type Expected struct {
	ContainsKeywords []string `json:"contains_keywords,omitempty"`
	AvoidsKeywords   []string `json:"avoids_keywords,omitempty"`
	OutputContains   []string `json:"output_contains,omitempty"`
	OutputAvoids     []string `json:"output_avoids,omitempty"`
	MinQuality       float64  `json:"min_quality,omitempty"`
	CustomCriteria   string   `json:"custom_criteria,omitempty"`
}

// LoadDataset reads + validates a single JSON file. Returns the
// Dataset or a wrapped error including the absolute path so users
// see which file failed without running the command twice.
func LoadDataset(path string) (*Dataset, error) {
	if path == "" {
		return nil, errors.New("dataset: empty path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("dataset: absolute path for %q: %w", path, err)
	}
	raw, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("dataset: read %q: %w", abs, err)
	}
	var ds Dataset
	if err := json.Unmarshal(raw, &ds); err != nil {
		return nil, fmt.Errorf("dataset: parse %q: %w", abs, err)
	}
	if err := ds.Validate(); err != nil {
		return nil, fmt.Errorf("dataset: validate %q: %w", abs, err)
	}
	return &ds, nil
}

// Validate enforces the JSON schema contract. Cheap to call on every
// load so dataset drift is caught at parse time, not deep inside the
// eval runner.
func (ds *Dataset) Validate() error {
	if ds.Name == "" {
		return errors.New("name is required")
	}
	if ds.Version == "" {
		return errors.New("version is required")
	}
	if len(ds.TestCases) == 0 {
		return errors.New("test_cases is empty")
	}
	seen := make(map[string]struct{}, len(ds.TestCases))
	for i := range ds.TestCases {
		tc := &ds.TestCases[i]
		if tc.ID == "" {
			return fmt.Errorf("test_cases[%d]: id is required", i)
		}
		if tc.Prompt == "" {
			return fmt.Errorf("test_cases[%d] (%s): prompt is required", i, tc.ID)
		}
		if _, dup := seen[tc.ID]; dup {
			return fmt.Errorf("test_cases[%d]: duplicate id %q", i, tc.ID)
		}
		seen[tc.ID] = struct{}{}
		if tc.Constraints.MaxTurns < 0 {
			return fmt.Errorf("test_cases[%s]: max_turns must be >= 0 (got %d)", tc.ID, tc.Constraints.MaxTurns)
		}
		if tc.Constraints.RequireVerify && tc.VerifyCmd == "" {
			return fmt.Errorf("test_cases[%s]: require_verify=true but verify_cmd is empty", tc.ID)
		}
		if tc.Constraints.MaxTokens < 0 {
			return fmt.Errorf("test_cases[%s]: max_tokens must be >= 0 (got %d)", tc.ID, tc.Constraints.MaxTokens)
		}
		if tc.Expected.MinQuality < 0 || tc.Expected.MinQuality > 1 {
			return fmt.Errorf("test_cases[%s]: min_quality must be in [0,1] (got %v)", tc.ID, tc.Expected.MinQuality)
		}
		if tc.Constraints.Timeout != "" {
			if _, err := time.ParseDuration(tc.Constraints.Timeout); err != nil {
				return fmt.Errorf("test_cases[%s]: invalid timeout %q: %v", tc.ID, tc.Constraints.Timeout, err)
			}
		}
	}
	return nil
}

// FilterByTag returns a copy of ds that contains only TestCases whose
// Tags include tag (case-insensitive). Empty tag returns a shallow
// copy with all cases — useful for "no filter" default UX.
func (ds *Dataset) FilterByTag(tag string) *Dataset {
	out := &Dataset{
		Name:        ds.Name,
		Version:     ds.Version,
		Description: ds.Description,
		TestCases:   []TestCase{},
	}
	if tag == "" {
		out.TestCases = append(out.TestCases, ds.TestCases...)
		return out
	}
	needle := strings.ToLower(tag)
	for _, tc := range ds.TestCases {
		for _, t := range tc.Tags {
			if strings.ToLower(t) == needle {
				out.TestCases = append(out.TestCases, tc)
				break
			}
		}
	}
	return out
}

// ListDatasets walks dir for *.json files relative to dir. Empty dir
// returns (nil, nil) so the CLI prints "no datasets" without crashing.
// Non-existent dir returns an error (the CLI surfaces it).
func ListDatasets(dir string) ([]string, error) {
	if dir == "" {
		dir = "evals"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("dataset: absolute path for %q: %w", dir, err)
	}
	if info, err := os.Stat(abs); err != nil {
		return nil, fmt.Errorf("dataset: stat %q: %w", abs, err)
	} else if !info.IsDir() {
		return nil, fmt.Errorf("dataset: %q is not a directory", abs)
	}
	var out []string
	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(d.Name()), ".json") {
			return nil
		}
		rel, _ := filepath.Rel(abs, path)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("dataset: walk %q: %w", abs, err)
	}
	return out, nil
}
