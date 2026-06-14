// SPDX-License-Identifier: MIT
// Purpose: Tests for Metrics & Reporting
package eval

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/dataset"
)

func TestMetricsReportCreation(t *testing.T) {
	report := &MetricsReport{
		Name:          "test-suite",
		TotalTests:    10,
		PassedTests:   8,
		FailedTests:   2,
		AverageScore:  0.82,
		MinScore:      0.65,
		MaxScore:      0.99,
		TotalDuration: 15 * time.Second,
	}

	if report.PassRate() != 0.8 {
		t.Errorf("Expected pass rate 0.8, got %f", report.PassRate())
	}
}

func TestCalculateMetrics(t *testing.T) {
	results := []dataset.RunResult{
		{
			TestCaseID:   "tc-1",
			Passed:       true,
			JudgeScore:   0.95,
			Turns:        2,
			ToolsUsed:    []string{"code_gen"},
		},
		{
			TestCaseID:   "tc-2",
			Passed:       true,
			JudgeScore:   0.88,
			Turns:        3,
			ToolsUsed:    []string{"verify"},
		},
		{
			TestCaseID:   "tc-3",
			Passed:       false,
			JudgeScore:   0.45,
			Turns:        1,
			ToolsUsed:    []string{},
		},
	}

	report := CalculateMetrics("test", results)

	if report.TotalTests != 3 {
		t.Errorf("Expected 3 total tests, got %d", report.TotalTests)
	}
	if report.PassedTests != 2 {
		t.Errorf("Expected 2 passed tests, got %d", report.PassedTests)
	}
	if report.FailedTests != 1 {
		t.Errorf("Expected 1 failed test, got %d", report.FailedTests)
	}
	if report.PassRate() != 2.0/3.0 {
		t.Errorf("Expected pass rate 0.667, got %f", report.PassRate())
	}
}

func TestCalculateAverageScore(t *testing.T) {
	results := []dataset.RunResult{
		{TestCaseID: "tc-1", JudgeScore: 1.0},
		{TestCaseID: "tc-2", JudgeScore: 0.5},
		{TestCaseID: "tc-3", JudgeScore: 0.75},
	}

	report := CalculateMetrics("test", results)

	expected := 0.75
	if report.AverageScore != expected {
		t.Errorf("Expected average score %f, got %f", expected, report.AverageScore)
	}
}

func TestMinMaxScores(t *testing.T) {
	results := []dataset.RunResult{
		{TestCaseID: "tc-1", JudgeScore: 0.2},
		{TestCaseID: "tc-2", JudgeScore: 0.99},
		{TestCaseID: "tc-3", JudgeScore: 0.5},
	}

	report := CalculateMetrics("test", results)

	if report.MinScore != 0.2 {
		t.Errorf("Expected min score 0.2, got %f", report.MinScore)
	}
	if report.MaxScore != 0.99 {
		t.Errorf("Expected max score 0.99, got %f", report.MaxScore)
	}
}

func TestFailedTestCases(t *testing.T) {
	results := []dataset.RunResult{
		{TestCaseID: "tc-1", Passed: true, JudgeScore: 0.9},
		{TestCaseID: "tc-2", Passed: false, JudgeScore: 0.3},
		{TestCaseID: "tc-3", Passed: true, JudgeScore: 0.85},
	}

	report := CalculateMetrics("test", results)

	if len(report.FailedTestCases) != 1 {
		t.Errorf("Expected 1 failed test case, got %d", len(report.FailedTestCases))
	}
	if report.FailedTestCases[0].TestCaseID != "tc-2" {
		t.Error("Wrong failed test case")
	}
}

func TestSaveReport(t *testing.T) {
	tmpDir := t.TempDir()
	reportFile := filepath.Join(tmpDir, "test-report.json")

	report := &MetricsReport{
		Name:          "test",
		TotalTests:    5,
		PassedTests:   4,
		FailedTests:   1,
		AverageScore:  0.85,
		MinScore:      0.7,
		MaxScore:      0.95,
		TotalDuration: 10 * time.Second,
	}

	err := report.SaveReport(reportFile)
	if err != nil {
		t.Fatalf("Failed to save report: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(reportFile); err != nil {
		t.Errorf("Report file not created: %v", err)
	}

	// Verify file has content
	fileInfo, err := os.Stat(reportFile)
	if err != nil {
		t.Errorf("Failed to stat report file: %v", err)
	}
	if fileInfo.Size() == 0 {
		t.Error("Report file is empty")
	}
}

func TestPrintSummary(t *testing.T) {
	report := &MetricsReport{
		Name:          "test",
		TotalTests:    10,
		PassedTests:   8,
		FailedTests:   2,
		AverageScore:  0.82,
		MinScore:      0.65,
		MaxScore:      0.99,
		TotalDuration: 15 * time.Second,
	}

	// Should not panic
	report.PrintSummary()
}

func TestEmptyResults(t *testing.T) {
	results := []dataset.RunResult{}
	report := CalculateMetrics("empty", results)

	if report.TotalTests != 0 {
		t.Errorf("Expected 0 total tests, got %d", report.TotalTests)
	}
	if report.PassRate() != 0 {
		t.Errorf("Expected pass rate 0 for empty results, got %f", report.PassRate())
	}
}

func TestSingleTestResult(t *testing.T) {
	results := []dataset.RunResult{
		{TestCaseID: "tc-1", Passed: true, JudgeScore: 0.95},
	}

	report := CalculateMetrics("single", results)

	if report.TotalTests != 1 {
		t.Error("Expected 1 test")
	}
	if report.PassRate() != 1.0 {
		t.Error("Expected 100% pass rate")
	}
	if report.AverageScore != 0.95 {
		t.Error("Expected average score 0.95")
	}
}

func TestCriteriaAggregation(t *testing.T) {
	results := []dataset.RunResult{
		{
			TestCaseID: "tc-1",
			JudgeScore: 0.9,
			JudgeFeedback: "Good",
		},
		{
			TestCaseID: "tc-2",
			JudgeScore: 0.8,
			JudgeFeedback: "OK",
		},
	}

	report := CalculateMetrics("test", results)

	if report.AverageScore < 0.8 || report.AverageScore > 0.91 {
		t.Errorf("Average score out of expected range: %f", report.AverageScore)
	}
}

func TestPassRateCalculation(t *testing.T) {
	tests := []struct {
		name    string
		total   int
		passed  int
		expected float64
	}{
		{"all pass", 10, 10, 1.0},
		{"half pass", 10, 5, 0.5},
		{"none pass", 10, 0, 0.0},
		{"single pass", 1, 1, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &MetricsReport{
				TotalTests:  tt.total,
				PassedTests: tt.passed,
				FailedTests: tt.total - tt.passed,
			}

			if report.PassRate() != tt.expected {
				t.Errorf("Expected pass rate %f, got %f", tt.expected, report.PassRate())
			}
		})
	}
}

func TestDurationTracking(t *testing.T) {
	report := &MetricsReport{
		Name:          "duration-test",
		TotalTests:    3,
		PassedTests:   3,
		FailedTests:   0,
		AverageScore:  0.9,
		MinScore:      0.85,
		MaxScore:      0.95,
		TotalDuration: 25 * time.Second,
	}

	if report.TotalDuration != 25*time.Second {
		t.Errorf("Expected duration 25s, got %v", report.TotalDuration)
	}
}

func BenchmarkCalculateMetrics(b *testing.B) {
	results := make([]dataset.RunResult, 100)
	for i := 0; i < 100; i++ {
		results[i] = dataset.RunResult{
			TestCaseID:   "tc-" + string(rune(i)),
			Passed:       i%2 == 0,
			JudgeScore:   float64(i) / 100.0,
			Turns:        i % 5,
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateMetrics("bench", results)
	}
}

func BenchmarkSaveReport(b *testing.B) {
	tmpDir := b.TempDir()
	report := &MetricsReport{
		Name:          "bench",
		TotalTests:    50,
		PassedTests:   40,
		FailedTests:   10,
		AverageScore:  0.85,
		MinScore:      0.5,
		MaxScore:      0.99,
		TotalDuration: 60 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reportFile := filepath.Join(tmpDir, "report-"+string(rune(i))+".json")
		_ = report.SaveReport(reportFile)
	}
}
