// SPDX-License-Identifier: MIT
// Purpose: Top-K precomputed index for lesson relevance (issue #341).
//
// Replaces the linear scan in Briefing() with an in-memory min-heap that
// maintains the top-K lessons by relevance. The heap stores the K most
// relevant lessons; the least-relevant of those K sits at the root so it
// can be efficiently evicted when a more relevant lesson arrives.
//
// Thread-safe (mandate M7): all public methods are guarded by a mutex.
package lessons

import (
	"container/heap"
	"sync"
	"time"
)

// Lesson is an alias for Entry, providing the name expected by the
// TopKIndex API surface (issue #341).
type Lesson = Entry

// lessonRelevance compares two lessons by relevance. Returns true if a
// is LESS relevant than b (i.e. a should be evicted before b in a min-heap).
// Relevance ordering:
//  1. Higher Occurrences = more relevant.
//  2. More recent LastSeen = more relevant (tiebreaker).
func lessonRelevance(a, b Lesson) bool {
	if a.Occurrences != b.Occurrences {
		return a.Occurrences < b.Occurrences
	}
	return a.LastSeen.Before(b.LastSeen)
}

// lessonHeap is a min-heap of Lessons ordered by relevance (least relevant
// at index 0). Implements container/heap.Interface.
type lessonHeap []Lesson

func (h lessonHeap) Len() int { return len(h) }

func (h lessonHeap) Less(i, j int) bool {
	return lessonRelevance(h[i], h[j])
}

func (h lessonHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
}

func (h *lessonHeap) Push(x any) {
	*h = append(*h, x.(Lesson))
}

func (h *lessonHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// TopKIndex maintains the top-K lessons by relevance using a min-heap.
// The heap has at most K elements; the least relevant sits at the root.
// Add() is O(log K), Top() is O(K log K) (sorting for output), and
// Refresh() is O(n log K) for n lessons.
type TopKIndex struct {
	mu   sync.Mutex
	k    int
	heap lessonHeap
}

// NewTopKIndex creates a TopKIndex that tracks the top-k most relevant
// lessons. k must be > 0; if k <= 0 it defaults to 10.
func NewTopKIndex(k int) *TopKIndex {
	if k <= 0 {
		k = 10
	}
	return &TopKIndex{
		k:    k,
		heap: make(lessonHeap, 0, k),
	}
}

// Add inserts a lesson into the index, maintaining the top-K invariant.
// If the heap has fewer than K elements, the lesson is always added.
// If the heap is full and the new lesson is more relevant than the
// least-relevant element (the root), the root is evicted and the new
// lesson is inserted. Otherwise the lesson is discarded.
func (i *TopKIndex) Add(lesson Lesson) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.heap.Len() < i.k {
		heap.Push(&i.heap, lesson)
		return
	}
	if lessonRelevance(i.heap[0], lesson) {
		heap.Pop(&i.heap)
		heap.Push(&i.heap, lesson)
	}
}

// Top returns the top-K lessons in descending relevance order (most
// relevant first). The returned slice is a copy; modifying it does not
// affect the index.
func (i *TopKIndex) Top() []Lesson {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.heap.Len() == 0 {
		return nil
	}
	sorted := make([]Lesson, i.heap.Len())
	copy(sorted, i.heap)
	sortLessonsDesc(sorted)
	return sorted
}

// Refresh rebuilds the index from a full set of lessons. The previous
// contents are discarded. This is O(n log K) for n lessons.
func (i *TopKIndex) Refresh(lessons []Lesson) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.heap = make(lessonHeap, 0, i.k)
	heap.Init(&i.heap)
	for _, l := range lessons {
		if i.heap.Len() < i.k {
			heap.Push(&i.heap, l)
			continue
		}
		if lessonRelevance(i.heap[0], l) {
			heap.Pop(&i.heap)
			heap.Push(&i.heap, l)
		}
	}
}

// Size returns the current number of lessons in the index (0 to K).
func (i *TopKIndex) Size() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.heap.Len()
}

// sortLessonsDesc sorts lessons in descending relevance order (most
// relevant first). This is the reverse of lessonRelevance.
func sortLessonsDesc(lessons []Lesson) {
	for i := 1; i < len(lessons); i++ {
		for j := i; j > 0 && lessonRelevance(lessons[j-1], lessons[j]); j-- {
			lessons[j-1], lessons[j] = lessons[j], lessons[j-1]
		}
	}
}

// nowForTest is overridable in tests for deterministic timestamps.
var nowForTest = time.Now
