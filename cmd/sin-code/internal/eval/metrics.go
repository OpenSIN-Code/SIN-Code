// SPDX-License-Identifier: MIT
// Purpose: Evaluation metrics and reporting
package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// MetricsReport aggregates evaluation results and metrics
type MetricsReport struct {
	DatasetName      string           `json:"dataset_name"`
	TotalCases       int              `json:"total_cases"`
	PassedCases      int              `json:"passed_cases"`
	FailedCases      int              `json:"failed_cases"`
	PassRate         float64          `json:"pass_rate"`
	AverageScore     float64          `json:"average_score"`
	MinScore         float64          `json:"min_score"`
	MaxScore         float64          `json:"max_score"`
	TotalDuration    time.Duration    `json:"total_duration"`
	CriteriaScores   map[string]float64 `json:"criteria_scores"`
	Timestamp        string           `json:"timestamp"`
	FailedTestCases  []FailedTestInfo `json:"failed_test_cases,omitempty"`
}

// FailedTestInfo contains information about a failed test case
type FailedTestInfo struct {
	TestCaseID string `json:"test_case_id"`
	Reason     string `json:"reason"`
	Score      float64 `json:"score,omitempty"`
}

// CalculateMetrics computes metrics from evaluation results
func CalculateMetrics(datasetName string, results []interface{}) *MetricsReport {
	report := &MetricsReport{
		DatasetName:    datasetName,
		Timestamp:      time.Now().Format(time.RFC3339),
		CriteriaScores: make(map[string]float64),
		FailedTestCases: []FailedTestInfo{},
	}

	if len(results) == 0 {
		return report
	}

	totalScore := 0.0
	minScore := 1.0
	maxScore := 0.0
	criteriaCount := make(map[string]int)

	for _, result := range results {
		if judgeResult, ok := result.(*JudgeResult); ok {
			report.TotalCases++

			if judgeResult.Passed {
				report.PassedCases++
			} else {
				report.FailedCases++
			}

			totalScore += judgeResult.Score
			if judgeResult.Score < minScore {
				minScore = judgeResult.Score
			}
			if judgeResult.Score > maxScore {
				maxScore = judgeResult.Score
			}

			// Aggregate criteria scores
			for criterion, score := range judgeResult.Criteria {
				report.CriteriaScores[criterion] += score
				criteriaCount[criterion]++
			}
		}
	}

	// Calculate averages
	if report.TotalCases > 0 {
		report.PassRate = float64(report.PassedCases) / float64(report.TotalCases)
		report.AverageScore = totalScore / float64(report.TotalCases)
		report.MinScore = minScore
		report.MaxScore = maxScore

		// Average criteria scores
		for criterion := range report.CriteriaScores {
			report.CriteriaScores[criterion] /= float64(criteriaCount[criterion])
		}
	}

	return report
}

// SaveReport persists the metrics report to a JSON file
func (r *MetricsReport) SaveReport(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write report file: %w", err)
	}

	return nil
}

// PrintSummary prints a human-readable summary of the metrics
func (r *MetricsReport) PrintSummary() {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Printf("EVALUATION REPORT: %s\n", r.DatasetName)
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total Test Cases: %d\n", r.TotalCases)
	fmt.Printf("Passed: %d | Failed: %d\n", r.PassedCases, r.FailedCases)
	fmt.Printf("Pass Rate: %.2f%%\n", r.PassRate*100)
	fmt.Printf("Average Score: %.2f/1.0\n", r.AverageScore)
	fmt.Printf("Score Range: [%.2f, %.2f]\n", r.MinScore, r.MaxScore)
	fmt.Printf("Total Duration: %v\n", r.TotalDuration)

	if len(r.CriteriaScores) > 0 {
		fmt.Println("\nCriteria Scores:")
		for criterion, score := range r.CriteriaScores {
			fmt.Printf("  - %s: %.2f/1.0\n", criterion, score)
		}
	}

	if len(r.FailedTestCases) > 0 {
		fmt.Println("\nFailed Test Cases:")
		for _, failed := range r.FailedTestCases {
			fmt.Printf("  - %s: %s\n", failed.TestCaseID, failed.Reason)
		}
	}

	fmt.Println(strings.Repeat("=", 60))
}
