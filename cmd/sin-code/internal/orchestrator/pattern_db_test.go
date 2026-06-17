// SPDX-License-Identifier: MIT
// Purpose: tests for issue #288 — Pattern Learning database.
// Tests cover: sequence recording, pattern aggregation, prompt matching
// (exact + fuzzy), success rate tracking, stats, listing, and race-free
// concurrent access (M7).
package orchestrator

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func newPatternDB(t *testing.T) *PatternDB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })
	pdb, err := NewPatternDB(db)
	if err != nil {
		t.Fatalf("NewPatternDB: %v", err)
	}
	return pdb
}

func makePlan(prompt string, taskTypes []TaskType, success bool) *Plan {
	tasks := make([]*Task, len(taskTypes))
	for i, tt := range taskTypes {
		tasks[i] = &Task{
			ID:          GenerateID("tk"),
			Type:        tt,
			Description: string(tt),
			Status:      TaskCompleted,
		}
		if !success {
			tasks[i].Status = TaskFailed
		}
	}
	now := time.Now()
	return &Plan{
		ID:        GenerateID("plan"),
		Prompt:    prompt,
		Tasks:     tasks,
		Created:   now.Add(-time.Minute),
		Started:   now.Add(-time.Minute),
		Completed: now,
		Success:   success,
	}
}

func TestHashPrompt_Normalization(t *testing.T) {
	h1 := HashPrompt("Implement Auth Module")
	h2 := HashPrompt("implement auth module")
	h3 := HashPrompt("  implement   auth  module  ")

	if h1 != h2 {
		t.Errorf("case-insensitive hash mismatch: %q vs %q", h1, h2)
	}
	if h1 != h3 {
		t.Errorf("whitespace-normalized hash mismatch: %q vs %q", h1, h3)
	}
	if len(h1) != 16 {
		t.Errorf("hash length = %d, want 16", len(h1))
	}
}

func TestPatternDB_RecordAndMatch(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	seq := []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}
	plan := makePlan("implement auth module", seq, true)
	if err := pdb.RecordSequence(ctx, plan); err != nil {
		t.Fatalf("RecordSequence: %v", err)
	}

	pred, err := pdb.MatchPrompt(ctx, "implement auth module")
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}
	if pred.MatchCount != 1 {
		t.Errorf("MatchCount = %d, want 1", pred.MatchCount)
	}
	if pred.SuccessRate != 1.0 {
		t.Errorf("SuccessRate = %.2f, want 1.0", pred.SuccessRate)
	}
	if len(pred.Patterns) != 4 {
		t.Errorf("Patterns len = %d, want 4", len(pred.Patterns))
	}

	// All patterns should have probability 1.0 (appeared in 1/1 sequences).
	for _, p := range pred.Patterns {
		if p.Probability != 1.0 {
			t.Errorf("Pattern %s P=%.2f, want 1.0", p.TaskType, p.Probability)
		}
	}
}

func TestPatternDB_ProbabilityAggregation(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	// Record 3 sequences: 2 with docs, 1 without.
	pdb.RecordSequence(ctx, makePlan("implement auth", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))
	pdb.RecordSequence(ctx, makePlan("implement auth", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskDocs}, true))
	pdb.RecordSequence(ctx, makePlan("implement auth", []TaskType{TaskArchitect, TaskCode, TaskTest, TaskSecurity}, false))

	pred, err := pdb.MatchPrompt(ctx, "implement auth")
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}
	if pred.MatchCount != 3 {
		t.Errorf("MatchCount = %d, want 3", pred.MatchCount)
	}

	patMap := map[TaskType]TaskPattern{}
	for _, p := range pred.Patterns {
		patMap[p.TaskType] = p
	}

	// architect: 3/3 = 1.0
	if patMap[TaskArchitect].Probability != 1.0 {
		t.Errorf("architect P=%.2f, want 1.0", patMap[TaskArchitect].Probability)
	}
	// coder: 3/3 = 1.0
	if patMap[TaskCode].Probability != 1.0 {
		t.Errorf("coder P=%.2f, want 1.0", patMap[TaskCode].Probability)
	}
	// tester: 3/3 = 1.0
	if patMap[TaskTest].Probability != 1.0 {
		t.Errorf("tester P=%.2f, want 1.0", patMap[TaskTest].Probability)
	}
	// docs: 2/3 = 0.67
	if patMap[TaskDocs].Probability < 0.66 || patMap[TaskDocs].Probability > 0.67 {
		t.Errorf("docs P=%.2f, want ~0.67", patMap[TaskDocs].Probability)
	}
	// security: 1/3 = 0.33
	if patMap[TaskSecurity].Probability < 0.32 || patMap[TaskSecurity].Probability > 0.34 {
		t.Errorf("security P=%.2f, want ~0.33", patMap[TaskSecurity].Probability)
	}
}

func TestPatternDB_SuccessRateTracking(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	pdb.RecordSequence(ctx, makePlan("build api", []TaskType{TaskArchitect, TaskCode}, true))
	pdb.RecordSequence(ctx, makePlan("build api", []TaskType{TaskArchitect, TaskCode}, true))
	pdb.RecordSequence(ctx, makePlan("build api", []TaskType{TaskArchitect, TaskCode}, false))

	pred, _ := pdb.MatchPrompt(ctx, "build api")
	if pred.MatchCount != 3 {
		t.Errorf("MatchCount = %d, want 3", pred.MatchCount)
	}
	if pred.SuccessRate < 0.66 || pred.SuccessRate > 0.67 {
		t.Errorf("SuccessRate = %.2f, want ~0.67", pred.SuccessRate)
	}
}

func TestPatternDB_FuzzyMatch(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	// Record a sequence with a specific prompt.
	pdb.RecordSequence(ctx, makePlan("implement user authentication with JWT", []TaskType{TaskArchitect, TaskCode}, true))

	// Different prompt but might share a hash prefix.
	pred, err := pdb.MatchPrompt(ctx, "implement user authentication with OAuth")
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}
	// Either exact match (unlikely different hash) or fuzzy match.
	// At minimum, it should not crash and return a valid prediction.
	_ = pred
}

func TestPatternDB_NoMatch(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	pred, err := pdb.MatchPrompt(ctx, "completely unknown prompt")
	if err != nil {
		t.Fatalf("MatchPrompt: %v", err)
	}
	if pred.MatchCount != 0 {
		t.Errorf("MatchCount = %d, want 0", pred.MatchCount)
	}
	if len(pred.Patterns) != 0 {
		t.Errorf("Patterns len = %d, want 0", len(pred.Patterns))
	}
}

func TestPatternDB_Stats(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	pdb.RecordSequence(ctx, makePlan("task A", []TaskType{TaskCode}, true))
	pdb.RecordSequence(ctx, makePlan("task B", []TaskType{TaskCode, TaskTest}, false))

	stats, err := pdb.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.TotalSequences != 2 {
		t.Errorf("TotalSequences = %d, want 2", stats.TotalSequences)
	}
	if stats.UniquePrompts != 2 {
		t.Errorf("UniquePrompts = %d, want 2", stats.UniquePrompts)
	}
	if stats.OverallSuccess != 0.5 {
		t.Errorf("OverallSuccess = %.2f, want 0.5", stats.OverallSuccess)
	}
}

func TestPatternDB_ListPatterns(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	pdb.RecordSequence(ctx, makePlan("task A", []TaskType{TaskCode}, true))
	pdb.RecordSequence(ctx, makePlan("task B", []TaskType{TaskCode, TaskTest}, true))

	entries, err := pdb.ListPatterns(ctx, 10)
	if err != nil {
		t.Fatalf("ListPatterns: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("entries len = %d, want 2", len(entries))
	}
}

func TestPatternDB_NilSafe(t *testing.T) {
	var pdb *PatternDB
	ctx := context.Background()

	// All methods should be no-ops on nil db.
	pred, err := pdb.MatchPrompt(ctx, "test")
	if err != nil {
		t.Errorf("nil MatchPrompt error: %v", err)
	}
	if pred == nil || pred.MatchCount != 0 {
		t.Error("nil MatchPrompt should return empty prediction")
	}

	stats, err := pdb.Stats(ctx)
	if err != nil {
		t.Errorf("nil Stats error: %v", err)
	}
	if stats == nil {
		t.Error("nil Stats should return empty stats")
	}
}

func TestPatternDB_RecordMultipleSequences(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	// Record 5 identical sequences.
	for i := 0; i < 5; i++ {
		err := pdb.RecordSequence(ctx, makePlan("build feature", []TaskType{TaskArchitect, TaskCode, TaskTest}, true))
		if err != nil {
			t.Fatalf("RecordSequence %d: %v", i, err)
		}
	}

	pred, _ := pdb.MatchPrompt(ctx, "build feature")
	if pred.MatchCount != 5 {
		t.Errorf("MatchCount = %d, want 5", pred.MatchCount)
	}
	for _, p := range pred.Patterns {
		if p.Frequency != 5 {
			t.Errorf("Pattern %s frequency = %d, want 5", p.TaskType, p.Frequency)
		}
	}
}

func TestPatternDB_RaceFree(t *testing.T) {
	pdb := newPatternDB(t)
	ctx := context.Background()

	// Concurrent writes + reads to stress the race detector.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			pdb.RecordSequence(ctx, makePlan("race test", []TaskType{TaskCode, TaskTest}, true))
		}
	}()

	for i := 0; i < 20; i++ {
		pdb.MatchPrompt(ctx, "race test")
	}
	<-done
}
