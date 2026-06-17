// SPDX-License-Identifier: MIT
// Purpose: tests for todo learning patterns (issue #327).
package todo

import (
	"sync"
	"testing"
	"time"
)

func TestRecordCompletionCreatesPattern(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "refactor database layer"}, 30*time.Minute, true)
	ps := l.Patterns()
	if len(ps) == 0 {
		t.Fatal("expected at least one pattern")
	}
	found := false
	for _, p := range ps {
		if p.Keyword == "refactor" || p.Keyword == "database" || p.Keyword == "layer" {
			if p.Frequency == 1 && p.AvgDuration == 30*time.Minute && p.SuccessRate == 1.0 {
				found = true
			}
		}
	}
	if !found {
		t.Errorf("no matching pattern: %+v", ps)
	}
}

func TestRecordCompletionAccumulates(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "fix database bug"}, 10*time.Minute, true)
	l.RecordCompletion(&Todo{Title: "fix database issue"}, 20*time.Minute, false)
	ps := l.Patterns()
	var dbP *TodoPattern
	for i := range ps {
		if ps[i].Keyword == "database" {
			dbP = &ps[i]
		}
	}
	if dbP == nil {
		t.Fatal("expected 'database' pattern")
	}
	if dbP.Frequency != 2 {
		t.Errorf("Frequency = %d, want 2", dbP.Frequency)
	}
	if dbP.AvgDuration != 15*time.Minute {
		t.Errorf("AvgDuration = %v, want 15m", dbP.AvgDuration)
	}
	if dbP.SuccessRate != 0.5 {
		t.Errorf("SuccessRate = %f, want 0.5", dbP.SuccessRate)
	}
}

func TestPredictDurationMatched(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "fix database bug"}, 10*time.Minute, true)
	l.RecordCompletion(&Todo{Title: "fix database issue"}, 20*time.Minute, true)
	dur := l.PredictDuration("fix database problem")
	if dur <= 0 {
		t.Errorf("expected positive prediction, got %v", dur)
	}
}

func TestPredictDurationNoMatch(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "refactor auth"}, 30*time.Minute, true)
	dur := l.PredictDuration("deploy infrastructure")
	if dur != 0 {
		t.Errorf("expected 0 for no match, got %v", dur)
	}
}

func TestPredictDurationEmptyTitle(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "fix bug"}, 10*time.Minute, true)
	if dur := l.PredictDuration(""); dur != 0 {
		t.Errorf("expected 0 for empty title, got %v", dur)
	}
}

func TestRecordCompletionNilTodo(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(nil, 10*time.Minute, true)
	if len(l.Patterns()) != 0 {
		t.Error("expected no patterns after nil todo")
	}
}

func TestPatternsSortedByFrequency(t *testing.T) {
	l := NewTodoLearning()
	l.RecordCompletion(&Todo{Title: "rare keyword"}, 5*time.Minute, true)
	for i := 0; i < 5; i++ {
		l.RecordCompletion(&Todo{Title: "common task"}, 5*time.Minute, true)
	}
	ps := l.Patterns()
	if len(ps) < 2 {
		t.Fatalf("expected >= 2 patterns, got %d", len(ps))
	}
	if ps[0].Frequency < ps[1].Frequency {
		t.Errorf("patterns not sorted by frequency desc: %+v", ps)
	}
}

func TestLearningConcurrent(t *testing.T) {
	l := NewTodoLearning()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.RecordCompletion(&Todo{Title: "fix database bug"}, 10*time.Minute, true)
			_ = l.PredictDuration("fix database issue")
			_ = l.Patterns()
		}()
	}
	wg.Wait()
}
