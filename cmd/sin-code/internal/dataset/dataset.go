// SPDX-License-Identifier: MIT
// Purpose: Golden Dataset Parser for SIN-Code evaluation
package dataset

import (
	"encoding/json"
	"fmt"
	"os"
)

// TestCase repräsentiert einen einzelnen Testfall
type TestCase struct {
	ID          string            `json:"id"`
	Prompt      string            `json:"prompt"`
	Constraints Constraints       `json:"constraints,omitempty"`
	Expected    Expected          `json:"expected,omitempty"`
	VerifyCmd   string            `json:"verify_cmd,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// Constraints definiert harte Regeln für den Agenten
type Constraints struct {
	MustUseTools    []string `json:"must_use_tools,omitempty"`
	ForbiddenTools  []string `json:"forbidden_tools,omitempty"`
	MaxTurns        int      `json:"max_turns,omitempty"`
	MaxTokens       int      `json:"max_tokens,omitempty"`
	RequireVerify   bool     `json:"require_verify"`
	TimeoutSeconds  int      `json:"timeout_seconds,omitempty"`
}

// Expected definiert Erwartungswerte für LLM-as-a-Judge
type Expected struct {
	ContainsKeywords []string `json:"contains_keywords,omitempty"`
	AvoidsKeywords   []string `json:"avoids_keywords,omitempty"`
	MinQuality       float64  `json:"min_quality,omitempty"` // 0.0 - 1.0
	CustomCriteria   string   `json:"custom_criteria,omitempty"`
}

// Dataset ist eine Sammlung von TestCases
type Dataset struct {
	Name        string     `json:"name"`
	Version     string     `json:"version"`
	Description string     `json:"description"`
	TestCases   []TestCase `json:"test_cases"`
}

// LoadDataset lädt ein Golden Dataset aus einer JSON-Datei
func LoadDataset(path string) (*Dataset, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read dataset file: %w", err)
	}

	var ds Dataset
	if err := json.Unmarshal(data, &ds); err != nil {
		return nil, fmt.Errorf("failed to parse dataset: %w", err)
	}

	// Validierung
	if len(ds.TestCases) == 0 {
		return nil, fmt.Errorf("dataset contains no test cases")
	}

	for i, tc := range ds.TestCases {
		if tc.ID == "" {
			return nil, fmt.Errorf("test case %d has no ID", i)
		}
		if tc.Prompt == "" {
			return nil, fmt.Errorf("test case %s has no prompt", tc.ID)
		}
	}

	return &ds, nil
}

// SaveDataset speichert ein Dataset als JSON-Datei
func SaveDataset(path string, ds *Dataset) error {
	data, err := json.MarshalIndent(ds, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal dataset: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write dataset file: %w", err)
	}

	return nil
}
