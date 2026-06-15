// internal/headroom/lessons.go
package headroom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Lesson captures a single piece of knowledge learned from a failed or
// suboptimal session. Headroom uses lessons to avoid compressing away context
// that previously turned out to be important.
type Lesson struct {
	ID        string    `json:"id"`
	Category  string    `json:"category"` // e.g. "compression", "retrieval", "tooling"
	Pattern   string    `json:"pattern"`  // the content pattern that mattered
	Insight   string    `json:"insight"`  // what was learned
	Weight    float64   `json:"weight"`   // importance 0..1, higher = keep more
	Hits      int       `json:"hits"`     // how often this lesson was reinforced
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LessonStore is a thread-safe, file-backed store of lessons.
type LessonStore struct {
	mu      sync.RWMutex
	path    string
	lessons map[string]*Lesson
}

// DefaultLessonsPath returns the default on-disk location for lessons.
func DefaultLessonsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".sin-code", "headroom", "lessons.json")
	}
	return filepath.Join(home, ".sin-code", "headroom", "lessons.json")
}

// NewLessonStore creates a store backed by the given file path. If the file
// exists it is loaded; otherwise an empty store is returned.
func NewLessonStore(path string) (*LessonStore, error) {
	if path == "" {
		path = DefaultLessonsPath()
	}
	s := &LessonStore{
		path:    path,
		lessons: make(map[string]*Lesson),
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *LessonStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // empty store is fine
		}
		return fmt.Errorf("reading lessons file: %w", err)
	}
	if len(data) == 0 {
		return nil
	}
	var list []*Lesson
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("parsing lessons file: %w", err)
	}
	for _, l := range list {
		s.lessons[l.ID] = l
	}
	return nil
}

// Save persists all lessons to disk atomically.
func (s *LessonStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("creating lessons dir: %w", err)
	}

	list := make([]*Lesson, 0, len(s.lessons))
	for _, l := range s.lessons {
		list = append(list, l)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.Before(list[j].CreatedAt) })

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding lessons: %w", err)
	}

	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("writing lessons tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("committing lessons file: %w", err)
	}
	return nil
}

// Record adds a new lesson or reinforces an existing one with the same
// category+pattern. Reinforcement increases the hit count and weight.
func (s *LessonStore) Record(category, pattern, insight string, weight float64) *Lesson {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := lessonID(category, pattern)
	now := time.Now()

	if existing, ok := s.lessons[id]; ok {
		existing.Hits++
		existing.UpdatedAt = now
		// Move weight toward the new observation but never below the old.
		existing.Weight = clampWeight((existing.Weight + weight) / 2)
		if insight != "" {
			existing.Insight = insight
		}
		return existing
	}

	l := &Lesson{
		ID:        id,
		Category:  category,
		Pattern:   pattern,
		Insight:   insight,
		Weight:    clampWeight(weight),
		Hits:      1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.lessons[id] = l
	return l
}

// Top returns the n highest-weighted lessons, most important first.
func (s *LessonStore) Top(n int) []*Lesson {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*Lesson, 0, len(s.lessons))
	for _, l := range s.lessons {
		list = append(list, l)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Weight == list[j].Weight {
			return list[i].Hits > list[j].Hits
		}
		return list[i].Weight > list[j].Weight
	})
	if n > 0 && n < len(list) {
		list = list[:n]
	}
	return list
}

// Count returns the number of stored lessons.
func (s *LessonStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lessons)
}

// All returns a copy of every lesson, oldest first.
func (s *LessonStore) All() []*Lesson {
	return s.Top(0)
}

func lessonID(category, pattern string) string {
	return fmt.Sprintf("%s:%x", category, simpleHash(pattern))
}

// simpleHash is a small FNV-1a hash used to derive stable lesson IDs.
func simpleHash(s string) uint32 {
	const (
		offset = 2166136261
		prime  = 16777619
	)
	h := uint32(offset)
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= prime
	}
	return h
}

func clampWeight(w float64) float64 {
	if w < 0 {
		return 0
	}
	if w > 1 {
		return 1
	}
	return w
}
