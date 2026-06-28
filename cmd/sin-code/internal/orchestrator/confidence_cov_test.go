// SPDX-License-Identifier: MIT
// Purpose: coverage tests for confidence.go that run without the "coverage"
// build tag.
package orchestrator

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func covSqlOpen(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestBrierScoreWithDB(t *testing.T) {
	db := covSqlOpen(t)
	c, err := NewCalibrator(db)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.9, Passed: true})
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.1, Passed: false})
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.5, Passed: true})

	score, n, err := c.BrierScore(ctx, "a")
	if err != nil {
		t.Fatalf("BrierScore: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected n=3, got %d", n)
	}
	// Brier = mean((0.9-1)^2 + (0.1-0)^2 + (0.5-1)^2) = mean(0.01 + 0.01 + 0.25) = 0.09
	expected := (0.01 + 0.01 + 0.25) / 3.0
	if score < expected-0.001 || score > expected+0.001 {
		t.Errorf("BrierScore = %f, want ~%f", score, expected)
	}
}

func TestBrierScorePerfectPrediction(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 1.0, Passed: true})
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.0, Passed: false})

	score, n, err := c.BrierScore(ctx, "a")
	if err != nil {
		t.Fatalf("BrierScore: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected n=2, got %d", n)
	}
	if score != 0 {
		t.Errorf("perfect prediction should have BrierScore=0, got %f", score)
	}
}

func TestBrierScoreWorstPrediction(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 1.0, Passed: false})
	_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.0, Passed: true})

	score, n, err := c.BrierScore(ctx, "a")
	if err != nil {
		t.Fatalf("BrierScore: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected n=2, got %d", n)
	}
	// Brier = mean((1-0)^2 + (0-1)^2) = mean(1 + 1) = 1.0
	if score != 1.0 {
		t.Errorf("worst prediction should have BrierScore=1.0, got %f", score)
	}
}

func TestBrierScoreNilDB(t *testing.T) {
	c, _ := NewCalibrator(nil)
	score, n, err := c.BrierScore(context.Background(), "a")
	if err != nil || score != 0 || n != 0 {
		t.Fatalf("nil DB should return (0,0,nil), got (%f,%d,%v)", score, n, err)
	}
}

func TestBrierScoreEmpty(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	score, n, err := c.BrierScore(context.Background(), "nonexistent")
	if err != nil || score != 0 || n != 0 {
		t.Fatalf("empty should return (0,0,nil), got (%f,%d,%v)", score, n, err)
	}
}

func TestBrierScoreNilCalibrator(t *testing.T) {
	var c *Calibrator
	score, n, err := c.BrierScore(context.Background(), "a")
	if err != nil || score != 0 || n != 0 {
		t.Fatalf("nil calibrator should return (0,0,nil), got (%f,%d,%v)", score, n, err)
	}
}

func TestCalibratorRecord(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	err := c.Record(context.Background(), ConfidenceClaim{
		AgentName: "test",
		TaskClass: ClassBugfix,
		Declared:  0.8,
		Passed:    true,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
}

func TestCalibratorRecordNilDB(t *testing.T) {
	c, _ := NewCalibrator(nil)
	err := c.Record(context.Background(), ConfidenceClaim{
		AgentName: "test",
		Declared:  0.8,
		Passed:    true,
	})
	if err != nil {
		t.Fatalf("nil DB Record should be no-op, got %v", err)
	}
}

func TestCalibratorNilCalibratorRecord(t *testing.T) {
	var c *Calibrator
	err := c.Record(context.Background(), ConfidenceClaim{})
	if err != nil {
		t.Fatalf("nil calibrator Record should be no-op, got %v", err)
	}
}

func TestCalibratorCalibrateNilDB(t *testing.T) {
	c, _ := NewCalibrator(nil)
	cal, err := c.Calibrate(context.Background(), "a", 0.7)
	if err != nil || cal != 0.7 {
		t.Fatalf("nil DB Calibrate should return declared, got %f err=%v", cal, err)
	}
}

func TestCalibratorCalibrateNilCalibrator(t *testing.T) {
	var c *Calibrator
	cal, err := c.Calibrate(context.Background(), "a", 0.7)
	if err != nil || cal != 0.7 {
		t.Fatalf("nil calibrator should return declared, got %f err=%v", cal, err)
	}
}

func TestDefaultMergePolicy(t *testing.T) {
	p := DefaultMergePolicy()
	if p.AutoMergeThreshold != 0.85 {
		t.Errorf("AutoMergeThreshold = %f, want 0.85", p.AutoMergeThreshold)
	}
	if p.ReviewThreshold != 0.6 {
		t.Errorf("ReviewThreshold = %f, want 0.6", p.ReviewThreshold)
	}
}

func TestMergePolicyDecide(t *testing.T) {
	p := DefaultMergePolicy()
	if d := p.Decide(false, 0.99); d != DecisionBlock {
		t.Errorf("Decide(false, 0.99) = %s, want block", d)
	}
	if d := p.Decide(true, 0.90); d != DecisionAutoMerge {
		t.Errorf("Decide(true, 0.90) = %s, want auto-merge", d)
	}
	if d := p.Decide(true, 0.70); d != DecisionGreenReview {
		t.Errorf("Decide(true, 0.70) = %s, want green-needs-review", d)
	}
	if d := p.Decide(true, 0.30); d != DecisionGreenReview {
		t.Errorf("Decide(true, 0.30) = %s, want green-needs-review", d)
	}
}

func TestCalibrateWithFewSamples(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	ctx := context.Background()
	// Less than 10 samples -> small-sample formula: 0.5 + (declared-0.5)*0.5
	for i := 0; i < 5; i++ {
		_ = c.Record(ctx, ConfidenceClaim{AgentName: "a", Declared: 0.8, Passed: true})
	}
	cal, err := c.Calibrate(ctx, "a", 0.8)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	// Expected: 0.5 + (0.8-0.5)*0.5 = 0.65
	if cal < 0.64 || cal > 0.66 {
		t.Errorf("expected ~0.65 for few samples, got %f", cal)
	}
}

func TestCalibrateNoDataReturnsDefault(t *testing.T) {
	db := covSqlOpen(t)
	c, _ := NewCalibrator(db)
	cal, err := c.Calibrate(context.Background(), "unknown", 0.8)
	if err != nil {
		t.Fatalf("Calibrate: %v", err)
	}
	// No data -> 0.5 + (0.8-0.5)*0.5 = 0.65
	if cal != 0.65 {
		t.Errorf("expected 0.65 for no data, got %f", cal)
	}
}
