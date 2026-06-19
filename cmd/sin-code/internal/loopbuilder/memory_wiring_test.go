// SPDX-License-Identifier: MIT
// Purpose: integration tests for memory wiring: autoDream lifecycle,
// memory prime injection, episodic memory record/replay, lessons on
// verify.fail. All tests must pass under `go test -race -count=1`
// (mandate M7).
package loopbuilder

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/agentloop"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/lessons"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/memory"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/orchestrator"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/verify"

	_ "modernc.org/sqlite"
)

func TestAutoDreamStartsAndStopsWithDaemonLifecycle(t *testing.T) {
	memDir := filepath.Join(t.TempDir(), "memory.db")
	memStore, err := memory.Open(memDir)
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	_ = memStore.Add(&memory.Memory{
		Insight:    "test memory for autodream lifecycle",
		Tags:       []string{"test"},
		Importance: 0.5,
	})

	ad := memory.NewAutoDream(memStore, memory.WithInterval(10*time.Millisecond))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ad.Start(ctx)
	time.Sleep(80 * time.Millisecond)
	ad.Stop()

	stats := ad.Stats()
	if stats.TotalRuns == 0 {
		t.Fatal("expected at least 1 run after start+sleep+stop")
	}

	ad.Stop()
	ad.Stop()
}

func TestAutoDreamRaceFreeLifecycle(t *testing.T) {
	memStore, err := memory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	ad := memory.NewAutoDream(memStore, memory.WithInterval(5*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ad.Start(ctx)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			_, _ = ad.RunOnce(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = ad.Stats()
		}()
	}
	wg.Wait()
	ad.Stop()
}

func TestMemoryPrimeInjectsContext(t *testing.T) {
	memStore, err := memory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	ws := t.TempDir()
	_ = memStore.Add(&memory.Memory{
		Insight: "always use context for cancellation in go",
		Tags:    []string{"go", "best-practice"},
		Project: ws,
	})
	_ = memStore.Add(&memory.Memory{
		Insight: "prefer table-driven tests in go",
		Tags:    []string{"go", "testing"},
		Project: ws,
	})

	loop, cleanup, err := Build(context.Background(), Config{
		Workspace:          ws,
		MaxTurns:           5,
		SkipMCP:            true,
		MemoryPrimeEnabled: true,
		MemoryStore:        memStore,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if loop.MemoryPrime == nil {
		t.Fatal("expected MemoryPrime to be wired when MemoryPrimeEnabled=true")
	}

	primed, err := loop.MemoryPrime(context.Background(), "context for cancellation")
	if err != nil {
		t.Fatal(err)
	}
	if primed == "" {
		t.Fatal("expected non-empty primed context")
	}
	if !strings.Contains(strings.ToLower(primed), "context") {
		t.Errorf("expected primed context to mention 'context', got: %s", primed)
	}
}

func TestMemoryPrimeNilWhenDisabled(t *testing.T) {
	loop, cleanup, err := Build(context.Background(), Config{
		Workspace: t.TempDir(),
		MaxTurns:  5,
		SkipMCP:   true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if loop.MemoryPrime != nil {
		t.Fatal("expected MemoryPrime to be nil when not enabled")
	}
}

func TestEpisodicMemoryRecordsOnPlanCompletion(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	epStore, err := orchestrator.NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}

	o := orchestrator.New()
	o.Episodes = epStore

	_, err = o.Run(context.Background(), "fix nil pointer in auth module")
	if err != nil {
		t.Fatalf("orchestrator Run: %v", err)
	}

	similar, err := epStore.Similar(context.Background(), "nil pointer auth", 5)
	if err != nil {
		t.Fatalf("Similar: %v", err)
	}
	if len(similar) == 0 {
		t.Fatal("expected at least 1 recorded episode after plan completion")
	}

	found := false
	for _, ep := range similar {
		if strings.Contains(ep.TaskTitle, "nil pointer") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected episode with 'nil pointer' in task title")
	}
}

func TestEpisodicMemoryInjectsPriorOnPlanCreation(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	epStore, err := orchestrator.NewEpisodeStore(db)
	if err != nil {
		t.Fatal(err)
	}

	_ = epStore.Record(context.Background(), &orchestrator.Episode{
		Intent:    string(orchestrator.IntentCodebase),
		TaskTitle: "fix nil pointer in auth module",
		PlanJSON:  []byte(`{"id":"p1"}`),
		Score:     0.95,
		Passed:    true,
		Rounds:    2,
		CreatedAt: time.Now().UTC(),
	})

	o := orchestrator.New()
	o.Episodes = epStore

	plan := o.Planner.BuildPlan("fix nil pointer in user service")
	if len(plan.Tasks) == 0 {
		t.Fatal("expected at least 1 task in plan")
	}

	similar, _ := epStore.Similar(context.Background(), "fix nil pointer in user service", 3)
	if len(similar) == 0 {
		t.Fatal("expected similar episodes to be found")
	}

	prior := orchestrator.PlanningPrior(similar)
	if prior == "" {
		t.Fatal("expected non-empty planning prior")
	}
	if !strings.Contains(prior, "SUCCEEDED") {
		t.Errorf("expected prior to mention SUCCEEDED, got: %s", prior)
	}
}

func TestEpisodicMemoryNilDBIsSafe(t *testing.T) {
	epStore, err := orchestrator.NewEpisodeStore(nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := epStore.Record(context.Background(), &orchestrator.Episode{
		Intent:    string(orchestrator.IntentGeneral),
		TaskTitle: "test task",
	}); err != nil {
		t.Fatalf("nil-DB Record must be no-op, got: %v", err)
	}

	similar, err := epStore.Similar(context.Background(), "test task", 5)
	if err != nil || similar != nil {
		t.Fatalf("nil-DB Similar must return nil, got: %v err=%v", similar, err)
	}
}

func TestLessonsRecordedOnVerifyFail(t *testing.T) {
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	calls := 0
	gate := verify.NewGate("poc",
		func(ctx context.Context, w string) (bool, string, error) {
			calls++
			if calls == 1 {
				return false, "tests fail: missing test for auth", nil
			}
			return true, "ok", nil
		}, nil)

	loop := &agentloop.Loop{
		Gate:      gate,
		Workspace: ws,
		Lessons:   mem,
		Completion: func(ctx context.Context, msgs []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			return &agentloop.Completion{
				Text: "done",
				Raw:  session.Message{Role: "assistant", Content: "done"},
			}, nil
		},
	}

	_, err = loop.Run(context.Background(), sess, "implement auth feature")
	if err != nil {
		t.Fatalf("loop Run: %v", err)
	}

	entries, err := mem.Query(context.Background(), ws, 50)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, e := range entries {
		if e.Type == lessons.TypeFailedVerification && strings.Contains(e.Lesson, "tests fail") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a TypeFailedVerification lesson to be recorded on verify.fail")
	}
}

func TestLessonsBriefedOnNextRun(t *testing.T) {
	ws := t.TempDir()
	mem, err := lessons.Open(filepath.Join(t.TempDir(), "lessons.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer mem.Close()

	lesson := lessons.Entry{
		Type:      lessons.TypeFailedVerification,
		Workspace: ws,
		Context:   map[string]any{"mode": "poc"},
		Lesson:    "always run migrations before tests",
	}
	_ = mem.Record(context.Background(), lesson)
	_ = mem.Record(context.Background(), lesson)

	store, err := session.Open(filepath.Join(t.TempDir(), "s.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	sess, err := store.StartOrResume("")
	if err != nil {
		t.Fatal(err)
	}

	var captured []session.Message
	loop := &agentloop.Loop{
		Gate:      verify.NewGate("off", nil, nil),
		Workspace: ws,
		Lessons:   mem,
		Completion: func(ctx context.Context, history []session.Message, tools []agentloop.ToolSpec) (*agentloop.Completion, error) {
			captured = append([]session.Message(nil), history...)
			return &agentloop.Completion{
				Text: "done",
				Raw:  session.Message{Role: "assistant", Content: "done"},
			}, nil
		},
	}

	if _, err := loop.Run(context.Background(), sess, "new task"); err != nil {
		t.Fatal(err)
	}

	found := false
	for _, m := range captured {
		if strings.Contains(m.Content, "WORKSPACE KNOWLEDGE") &&
			strings.Contains(m.Content, "migrations before tests") {
			found = true
		}
	}
	if !found {
		t.Fatal("expected knowledge briefing in first-turn history")
	}
}

func TestMemoryWiringRaceFreeConcurrentAccess(t *testing.T) {
	memStore, err := memory.Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer memStore.Close()

	ad := memory.NewAutoDream(memStore, memory.WithInterval(5*time.Millisecond))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for i := 0; i < 10; i++ {
		_ = memStore.Add(&memory.Memory{
			Insight:    "race test memory " + string(rune('A'+i)),
			Tags:       []string{"race"},
			Importance: 0.5,
		})
	}

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(3)
		go func() {
			defer wg.Done()
			_, _ = ad.RunOnce(ctx)
		}()
		go func() {
			defer wg.Done()
			_ = ad.Stats()
		}()
		go func() {
			defer wg.Done()
			_, _ = memStore.List(memory.ListFilter{Limit: 100})
		}()
	}
	wg.Wait()
}
