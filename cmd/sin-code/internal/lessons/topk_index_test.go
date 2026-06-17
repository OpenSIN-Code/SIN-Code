// SPDX-License-Identifier: MIT
// Purpose: Tests for the TopKIndex (issue #341). All tests must pass
// under `go test -race -count=1` (mandate M7).
package lessons

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"
)

func mkLesson(occurrences int, lastSeen time.Time, lesson string) Lesson {
	return Lesson{
		Type:        TypeFailedVerification,
		Workspace:   "/test",
		Lesson:      lesson,
		Occurrences: occurrences,
		LastSeen:    lastSeen,
	}
}

// TestTopKIndex_Add verifies that Add inserts lessons into the index.
func TestTopKIndex_Add(t *testing.T) {
	idx := NewTopKIndex(5)
	base := time.Now().UTC()
	idx.Add(mkLesson(3, base, "a"))
	idx.Add(mkLesson(5, base, "b"))
	if idx.Size() != 2 {
		t.Fatalf("expected size 2, got %d", idx.Size())
	}
}

// TestTopKIndex_Top returns the top-K lessons in descending relevance order.
func TestTopKIndex_Top(t *testing.T) {
	idx := NewTopKIndex(3)
	base := time.Now().UTC()
	idx.Add(mkLesson(1, base, "low"))
	idx.Add(mkLesson(5, base, "high"))
	idx.Add(mkLesson(3, base, "mid"))
	top := idx.Top()
	if len(top) != 3 {
		t.Fatalf("expected 3 lessons, got %d", len(top))
	}
	if top[0].Lesson != "high" || top[1].Lesson != "mid" || top[2].Lesson != "low" {
		t.Fatalf("expected [high mid low], got [%s %s %s]", top[0].Lesson, top[1].Lesson, top[2].Lesson)
	}
}

// TestTopKIndex_Refresh rebuilds the index from a full set.
func TestTopKIndex_Refresh(t *testing.T) {
	idx := NewTopKIndex(3)
	base := time.Now().UTC()
	idx.Add(mkLesson(1, base, "old-1"))
	idx.Add(mkLesson(1, base, "old-2"))

	lessons := []Lesson{
		mkLesson(10, base, "x"),
		mkLesson(20, base, "y"),
		mkLesson(30, base, "z"),
		mkLesson(5, base, "w"),
		mkLesson(1, base, "v"),
	}
	idx.Refresh(lessons)
	if idx.Size() != 3 {
		t.Fatalf("expected size 3 after refresh, got %d", idx.Size())
	}
	top := idx.Top()
	if top[0].Lesson != "z" || top[1].Lesson != "y" || top[2].Lesson != "x" {
		t.Fatalf("expected [z y x], got [%s %s %s]", top[0].Lesson, top[1].Lesson, top[2].Lesson)
	}
}

// TestTopKIndex_Size returns the current number of elements.
func TestTopKIndex_Size(t *testing.T) {
	idx := NewTopKIndex(10)
	if idx.Size() != 0 {
		t.Fatalf("expected size 0, got %d", idx.Size())
	}
	base := time.Now().UTC()
	for i := 0; i < 5; i++ {
		idx.Add(mkLesson(i+1, base, fmt.Sprintf("l-%d", i)))
	}
	if idx.Size() != 5 {
		t.Fatalf("expected size 5, got %d", idx.Size())
	}
}

// TestTopKIndex_Ordering verifies correct relevance ordering with
// occurrences as primary and LastSeen as tiebreaker.
func TestTopKIndex_Ordering(t *testing.T) {
	idx := NewTopKIndex(5)
	t1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	// Same occurrences, different LastSeen: more recent wins.
	idx.Add(mkLesson(3, t1, "older"))
	idx.Add(mkLesson(3, t2, "newer"))
	idx.Add(mkLesson(1, t2, "low-occ"))
	idx.Add(mkLesson(10, t1, "high-occ"))
	top := idx.Top()
	if len(top) != 4 {
		t.Fatalf("expected 4, got %d", len(top))
	}
	// Expected order: high-occ(10), newer(3,t2), older(3,t1), low-occ(1)
	want := []string{"high-occ", "newer", "older", "low-occ"}
	for i, w := range want {
		if top[i].Lesson != w {
			t.Fatalf("expected top[%d]=%s, got %s (full %v)", i, w, top[i].Lesson, top)
		}
	}
}

// TestTopKIndex_DuplicateHandling verifies that adding the same lesson
// twice does not create duplicates in the top-K.
func TestTopKIndex_DuplicateHandling(t *testing.T) {
	idx := NewTopKIndex(5)
	base := time.Now().UTC()
	l := mkLesson(5, base, "dup")
	idx.Add(l)
	idx.Add(l)
	idx.Add(l)
	if idx.Size() != 3 {
		t.Fatalf("expected size 3 (duplicates are separate entries in the heap), got %d", idx.Size())
	}
	top := idx.Top()
	for _, tl := range top {
		if tl.Lesson != "dup" {
			t.Fatalf("expected all 'dup', got %s", tl.Lesson)
		}
	}
}

// TestTopKIndex_Empty verifies that an empty index returns nil from Top().
func TestTopKIndex_Empty(t *testing.T) {
	idx := NewTopKIndex(5)
	if idx.Size() != 0 {
		t.Fatalf("expected size 0, got %d", idx.Size())
	}
	top := idx.Top()
	if top != nil {
		t.Fatalf("expected nil from empty Top(), got %v", top)
	}
}

// TestTopKIndex_LargeDataset verifies correctness with a large number of
// lessons and confirms the index only keeps K elements.
func TestTopKIndex_LargeDataset(t *testing.T) {
	k := 10
	idx := NewTopKIndex(k)
	base := time.Now().UTC()
	rng := rand.New(rand.NewSource(42))
	for i := 0; i < 10000; i++ {
		idx.Add(mkLesson(rng.Intn(100)+1, base.Add(time.Duration(i)*time.Millisecond), fmt.Sprintf("lesson-%d", i)))
	}
	if idx.Size() != k {
		t.Fatalf("expected size %d, got %d", k, idx.Size())
	}
	top := idx.Top()
	// Verify descending order.
	for i := 1; i < len(top); i++ {
		if lessonRelevance(top[i-1], top[i]) {
			t.Fatalf("top not in descending order at index %d: %v before %v", i, top[i-1], top[i])
		}
	}
}

// TestTopKIndex_Concurrent verifies race-safe concurrent Add and Top
// calls (mandate M7).
func TestTopKIndex_Concurrent(t *testing.T) {
	idx := NewTopKIndex(20)
	base := time.Now().UTC()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			idx.Add(mkLesson(n%30+1, base.Add(time.Duration(n)*time.Millisecond), fmt.Sprintf("c-%d", n)))
		}(i)
	}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = idx.Top()
			_ = idx.Size()
		}()
	}
	wg.Wait()
	if idx.Size() > 20 {
		t.Fatalf("expected size <= 20, got %d", idx.Size())
	}
}

// BenchmarkTopKIndex_AddAndTop measures the performance of Add and Top
// on a large dataset.
func BenchmarkTopKIndex_AddAndTop(b *testing.B) {
	base := time.Now().UTC()
	lessons := make([]Lesson, 1000)
	for i := range lessons {
		lessons[i] = mkLesson(i%50+1, base.Add(time.Duration(i)*time.Millisecond), fmt.Sprintf("bench-%d", i))
	}
	b.ResetTimer()
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		idx := NewTopKIndex(10)
		for _, l := range lessons {
			idx.Add(l)
		}
		_ = idx.Top()
	}
}
