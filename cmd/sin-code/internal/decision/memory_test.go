// SPDX-License-Identifier: MIT
package decision

import (
	"context"
	"testing"
	"time"
)

func TestDecisionStore_RecordAndList(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := Open("test-workspace")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	d := Decision{
		ID:        "test-1",
		SessionID: "sess-1",
		Decision:  "Use Redis for session cache",
		Rationale: "Faster than DB for hot reads",
		Workspace: "test-workspace",
	}
	if err := store.Record(context.Background(), d); err != nil {
		t.Fatal(err)
	}

	list, err := store.List(context.Background(), "test-workspace", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("len = %d, want 1", len(list))
	}
	if list[0].Decision != "Use Redis for session cache" {
		t.Errorf("Decision = %q", list[0].Decision)
	}
}

func TestDecisionStore_Search(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := Open("test-ws")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	decisions := []Decision{
		{ID: "1", Decision: "Use Redis for cache", Rationale: "fast", Workspace: "test-ws"},
		{ID: "2", Decision: "Use PostgreSQL for persistence", Rationale: "reliable", Workspace: "test-ws"},
		{ID: "3", Decision: "Use Kafka for events", Rationale: "scalable", Workspace: "test-ws"},
	}
	for _, d := range decisions {
		d.Timestamp = time.Now()
		if err := store.Record(context.Background(), d); err != nil {
			t.Fatal(err)
		}
	}

	results, err := store.Search(context.Background(), "test-ws", "redis")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("search results = %d, want 1", len(results))
	}
	if results[0].Decision != "Use Redis for cache" {
		t.Errorf("Decision = %q", results[0].Decision)
	}
}

func TestDecisionStore_WorkspaceIsolation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := Open("ws-a")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	d1 := Decision{ID: "1", Decision: "A decision", Workspace: "ws-a"}
	d2 := Decision{ID: "2", Decision: "B decision", Workspace: "ws-b"}
	store.Record(context.Background(), d1)
	store.Record(context.Background(), d2)

	list, _ := store.List(context.Background(), "ws-a", 10)
	if len(list) != 1 {
		t.Fatalf("ws-a list = %d, want 1", len(list))
	}
	list, _ = store.List(context.Background(), "ws-b", 10)
	if len(list) != 1 {
		t.Fatalf("ws-b list = %d, want 1", len(list))
	}
}
