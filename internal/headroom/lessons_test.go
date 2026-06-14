// internal/headroom/lessons_test.go
package headroom

import (
	"path/filepath"
	"testing"
)

func TestLessonStore_RecordAndReinforce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lessons.json")
	store, err := NewLessonStore(path)
	if err != nil {
		t.Fatalf("NewLessonStore failed: %v", err)
	}

	l1 := store.Record("compression", "keep stack traces", "stack traces are critical", 0.6)
	if l1.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", l1.Hits)
	}

	// Reinforce the same category+pattern.
	l2 := store.Record("compression", "keep stack traces", "still critical", 0.8)
	if l2.Hits != 2 {
		t.Errorf("expected 2 hits after reinforcement, got %d", l2.Hits)
	}
	if l1.ID != l2.ID {
		t.Errorf("reinforcement should reuse the same lesson ID: %s != %s", l1.ID, l2.ID)
	}
	if store.Count() != 1 {
		t.Errorf("expected 1 lesson, got %d", store.Count())
	}
}

func TestLessonStore_WeightClamp(t *testing.T) {
	store, _ := NewLessonStore(filepath.Join(t.TempDir(), "l.json"))
	l := store.Record("x", "y", "z", 5.0) // over 1.0
	if l.Weight > 1.0 {
		t.Errorf("weight should be clamped to <= 1.0, got %f", l.Weight)
	}
	l2 := store.Record("a", "b", "c", -2.0) // below 0
	if l2.Weight < 0 {
		t.Errorf("weight should be clamped to >= 0, got %f", l2.Weight)
	}
}

func TestLessonStore_TopOrdering(t *testing.T) {
	store, _ := NewLessonStore(filepath.Join(t.TempDir(), "l.json"))
	store.Record("c", "low", "", 0.2)
	store.Record("c", "high", "", 0.9)
	store.Record("c", "mid", "", 0.5)

	top := store.Top(2)
	if len(top) != 2 {
		t.Fatalf("expected 2 lessons, got %d", len(top))
	}
	if top[0].Weight < top[1].Weight {
		t.Errorf("Top should return highest weight first: %f < %f", top[0].Weight, top[1].Weight)
	}
	if top[0].Pattern != "high" {
		t.Errorf("expected highest-weighted pattern 'high', got %q", top[0].Pattern)
	}
}

func TestLessonStore_Persistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lessons.json")

	store, _ := NewLessonStore(path)
	store.Record("persist", "pattern-a", "insight-a", 0.7)
	if err := store.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Reload from disk into a fresh store.
	reloaded, err := NewLessonStore(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}
	if reloaded.Count() != 1 {
		t.Errorf("expected 1 persisted lesson, got %d", reloaded.Count())
	}
	top := reloaded.Top(1)
	if len(top) == 0 || top[0].Insight != "insight-a" {
		t.Errorf("persisted lesson not loaded correctly: %+v", top)
	}
}

func TestSimpleHash_Stable(t *testing.T) {
	a := simpleHash("hello world")
	b := simpleHash("hello world")
	if a != b {
		t.Errorf("simpleHash should be deterministic: %d != %d", a, b)
	}
	if simpleHash("a") == simpleHash("b") {
		t.Error("different inputs should generally produce different hashes")
	}
}
