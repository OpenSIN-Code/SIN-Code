// SPDX-License-Identifier: MIT
// Purpose: tests for the unified memory query (issue #346).
package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

type fakeMemoryStore struct {
	results []ScoredMemory
	err     error
}

func (f *fakeMemoryStore) Search(query string, project string, limit int) ([]ScoredMemory, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.results, nil
}

type fakeLessonsStore struct {
	entries []lessons.Entry
	err     error
}

func (f *fakeLessonsStore) Query(ctx context.Context, workspace string, limit int) ([]lessons.Entry, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.entries, nil
}

type fakeSessionStore struct {
	infos []session.Info
	err   error
}

func (f *fakeSessionStore) List() ([]session.Info, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.infos, nil
}

type fakeLedgerStore struct {
	sessions []string
	entries  map[string][]ledger.Entry
	err      error
}

func (f *fakeLedgerStore) Sessions(ctx context.Context, limit int) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.sessions, nil
}

func (f *fakeLedgerStore) List(ctx context.Context, sessionID string, limit int) ([]ledger.Entry, error) {
	if f.entries == nil {
		return nil, nil
	}
	return f.entries[sessionID], nil
}

type fakeEpisodeStore struct {
	hits []EpisodeHit
	err  error
}

func (f *fakeEpisodeStore) Similar(ctx context.Context, taskTitle string, k int) ([]EpisodeHit, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.hits, nil
}

func TestUnifiedStoreNilSafe(t *testing.T) {
	var u *UnifiedStore
	res, err := u.Query(context.Background(), "x", 5)
	if err != nil {
		t.Fatalf("nil store should not error: %v", err)
	}
	if res != nil {
		t.Errorf("nil store should return nil results, got %v", res)
	}
}

func TestUnifiedStoreAllNilStores(t *testing.T) {
	u := NewUnifiedStore(nil, nil, nil, nil, nil)
	res, err := u.Query(context.Background(), "x", 5)
	if err != nil {
		t.Fatalf("empty store should not error: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("empty store should return 0 results, got %d", len(res))
	}
	for _, label := range []StoreLabel{StoreMemory, StoreLessons, StoreSessions, StoreLedger, StoreEpisodes} {
		if u.HasStore(label) {
			t.Errorf("HasStore(%s) should be false when nil", label)
		}
	}
}

func TestUnifiedStoreHasStore(t *testing.T) {
	u := &UnifiedStore{
		mem:     &fakeMemoryStore{},
		lessons: &fakeLessonsStore{},
	}
	if !u.HasStore(StoreMemory) || !u.HasStore(StoreLessons) {
		t.Error("wired stores should report true")
	}
	if u.HasStore(StoreSessions) || u.HasStore(StoreLedger) || u.HasStore(StoreEpisodes) {
		t.Error("unwired stores should report false")
	}
}

func TestUnifiedStoreMemoryOnly(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{
			results: []ScoredMemory{
				{Memory: &Memory{ID: "m1", Insight: "auth module bug", Updated: now}, Score: 0.9},
				{Memory: &Memory{ID: "m2", Insight: "unrelated", Updated: now}, Score: 0.2},
			},
		},
	}
	res, err := u.Query(context.Background(), "auth", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Store != StoreMemory {
		t.Errorf("store label: got %s, want memory", res[0].Store)
	}
	if res[0].ID != "m1" {
		t.Errorf("top result should be m1, got %s", res[0].ID)
	}
}

func TestUnifiedStoreLessonsOnly(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		lessons: &fakeLessonsStore{
			entries: []lessons.Entry{
				{ID: "l1", Lesson: "always run tests", Occurrences: 5, LastSeen: now},
				{ID: "l2", Lesson: "check nil pointers", Occurrences: 1, LastSeen: now},
			},
		},
	}
	res, err := u.Query(context.Background(), "tests", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("expected 2 results, got %d", len(res))
	}
	if res[0].Store != StoreLessons {
		t.Errorf("store: got %s, want lessons", res[0].Store)
	}
}

func TestUnifiedStoreDedup(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{
			results: []ScoredMemory{
				{Memory: &Memory{ID: "m1", Insight: "duplicate content", Updated: now}, Score: 0.8},
			},
		},
		lessons: &fakeLessonsStore{
			entries: []lessons.Entry{
				{ID: "l1", Lesson: "duplicate content", Occurrences: 3, LastSeen: now},
			},
		},
	}
	res, err := u.Query(context.Background(), "dup", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 {
		t.Fatalf("dedup: expected 1 result, got %d", len(res))
	}
}

func TestUnifiedStoreRankingByScore(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{
			results: []ScoredMemory{
				{Memory: &Memory{ID: "low", Insight: "low score", Updated: now}, Score: 0.1},
				{Memory: &Memory{ID: "high", Insight: "high score", Updated: now}, Score: 0.95},
			},
		},
	}
	res, err := u.Query(context.Background(), "score", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) < 2 {
		t.Fatalf("expected >= 2 results, got %d", len(res))
	}
	if res[0].Score < res[1].Score {
		t.Errorf("results should be sorted descending")
	}
	if res[0].ID != "high" {
		t.Errorf("top result should be 'high', got %s", res[0].ID)
	}
}

func TestUnifiedStoreTopKLimit(t *testing.T) {
	now := time.Now()
	results := make([]ScoredMemory, 20)
	for i := range results {
		results[i] = ScoredMemory{
			Memory: &Memory{ID: fmt.Sprintf("m%d", i), Insight: fmt.Sprintf("item number %d", i), Updated: now},
			Score:  float64(i) / 20.0,
		}
	}
	u := &UnifiedStore{mem: &fakeMemoryStore{results: results}}
	res, err := u.Query(context.Background(), "item", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 {
		t.Errorf("topK limit: expected 5, got %d", len(res))
	}
}

func TestUnifiedStoreStoreErrorNonFatal(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{err: errors.New("memory boom")},
		lessons: &fakeLessonsStore{
			entries: []lessons.Entry{
				{ID: "l1", Lesson: "survives", LastSeen: now},
			},
		},
	}
	res, err := u.Query(context.Background(), "surv", 10)
	if err != nil {
		t.Fatalf("partial error should not fail query: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("expected 1 result from lessons, got %d", len(res))
	}
	if res[0].Store != StoreLessons {
		t.Errorf("expected lessons result, got %s", res[0].Store)
	}
}

func TestUnifiedStoreAllStoresError(t *testing.T) {
	u := &UnifiedStore{
		mem:      &fakeMemoryStore{err: errors.New("e1")},
		lessons:  &fakeLessonsStore{err: errors.New("e2")},
		sess:     &fakeSessionStore{err: errors.New("e3")},
		ledger:   &fakeLedgerStore{err: errors.New("e4")},
		episodes: &fakeEpisodeStore{err: errors.New("e5")},
	}
	res, err := u.Query(context.Background(), "x", 10)
	if err == nil {
		t.Fatal("all stores erroring should return an error")
	}
	if res != nil {
		t.Errorf("expected nil results on total failure")
	}
}

func TestUnifiedStoreMultiStoreMerge(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{
			results: []ScoredMemory{
				{Memory: &Memory{ID: "m1", Insight: "auth fix", Updated: now}, Score: 0.7},
			},
		},
		lessons: &fakeLessonsStore{
			entries: []lessons.Entry{
				{ID: "l1", Lesson: "auth lesson", Occurrences: 2, LastSeen: now},
			},
		},
		sess: &fakeSessionStore{
			infos: []session.Info{
				{ID: "s1", Title: "auth session", UpdatedAt: now.Format(time.RFC3339)},
			},
		},
		ledger: &fakeLedgerStore{
			sessions: []string{"sess1"},
			entries: map[string][]ledger.Entry{
				"sess1": {{ID: "le1", Summary: "auth verify", CreatedAt: now}},
			},
		},
		episodes: &fakeEpisodeStore{
			hits: []EpisodeHit{
				{ID: 1, TaskTitle: "auth episode", Score: 0.8, Passed: true, CreatedAt: now},
			},
		},
	}
	res, err := u.Query(context.Background(), "auth", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 5 {
		t.Fatalf("expected 5 merged results, got %d", len(res))
	}
	stores := map[StoreLabel]bool{}
	for _, r := range res {
		stores[r.Store] = true
	}
	for _, want := range []StoreLabel{StoreMemory, StoreLessons, StoreSessions, StoreLedger, StoreEpisodes} {
		if !stores[want] {
			t.Errorf("missing result from store %s", want)
		}
	}
}

func TestUnifiedStoreConcurrentQueries(t *testing.T) {
	now := time.Now()
	u := &UnifiedStore{
		mem: &fakeMemoryStore{
			results: []ScoredMemory{
				{Memory: &Memory{ID: "m1", Insight: "concurrent", Updated: now}, Score: 0.9},
			},
		},
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := u.Query(context.Background(), "concurrent", 5)
			if err != nil {
				t.Errorf("concurrent query error: %v", err)
			}
			if len(res) != 1 {
				t.Errorf("expected 1 result, got %d", len(res))
			}
		}()
	}
	wg.Wait()
}
