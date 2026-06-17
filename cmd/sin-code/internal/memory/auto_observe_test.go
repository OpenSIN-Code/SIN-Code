// SPDX-License-Identifier: MIT
// Purpose: tests for auto-observation capture (issue #349). Covers
// tool filtering, observation creation, grouping, and race-free
// concurrency (mandate M7).
package memory

import (
	"sync"
	"testing"
)

func TestAutoObserverShouldObserveMutating(t *testing.T) {
	o := NewAutoObserver(nil)
	for _, tool := range []string{"edit", "write", "execute", "test", "sin_edit", "sin_write", "sin_execute", "sin_test"} {
		if !o.ShouldObserve(tool) {
			t.Errorf("ShouldObserve(%q) should be true", tool)
		}
	}
}

func TestAutoObserverShouldNotObserveReadOnly(t *testing.T) {
	o := NewAutoObserver(nil)
	for _, tool := range []string{"discover", "scout", "map", "read", "sin_discover", "sin_scout", "sin_map", "sin_read"} {
		if o.ShouldObserve(tool) {
			t.Errorf("ShouldObserve(%q) should be false", tool)
		}
	}
}

func TestAutoObserverShouldNotObserveUnknown(t *testing.T) {
	o := NewAutoObserver(nil)
	for _, tool := range []string{"unknown", "", "grep", "ls"} {
		if o.ShouldObserve(tool) {
			t.Errorf("ShouldObserve(%q) should be false", tool)
			break
		}
	}
}

func TestAutoObserverObserveCreates(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	o.Observe("edit", map[string]any{"path": "/tmp/foo.go"}, "edited successfully", true)

	list, err := s.List(ListFilter{Tag: "observation"})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(list))
	}
	if !containsStr(list[0].Tags, "observation") {
		t.Errorf("tags should contain 'observation', got %v", list[0].Tags)
	}
}

func TestAutoObserverObserveSkipsReadOnly(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	o.Observe("discover", map[string]any{}, "found 3 files", true)
	o.Observe("scout", map[string]any{}, "match found", true)
	o.Observe("map", map[string]any{}, "mapped", true)

	list, _ := s.List(ListFilter{Tag: "observation"})
	if len(list) != 0 {
		t.Fatalf("expected 0 observations for read-only tools, got %d", len(list))
	}
}

func TestAutoObserverGroupsSimilar(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	args := map[string]any{"path": "/tmp/main.go"}
	o.Observe("edit", args, "first edit", true)
	o.Observe("edit", args, "second edit", true)
	o.Observe("edit", args, "third edit", true)

	list, _ := s.List(ListFilter{Tag: "observation"})
	if len(list) != 1 {
		t.Fatalf("expected 1 grouped observation, got %d", len(list))
	}
	if list[0].AccessCount < 2 {
		t.Errorf("access count should be >= 2, got %d", list[0].AccessCount)
	}
}

func TestAutoObserverDifferentFilesSeparate(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	o.Observe("edit", map[string]any{"path": "/tmp/a.go"}, "edit A", true)
	o.Observe("edit", map[string]any{"path": "/tmp/b.go"}, "edit B", true)

	list, _ := s.List(ListFilter{Tag: "observation"})
	if len(list) != 2 {
		t.Fatalf("expected 2 observations for different files, got %d", len(list))
	}
}

func TestAutoObserverFailedTool(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	o.Observe("execute", map[string]any{"command": "go build"}, "exit code 1", false)

	list, _ := s.List(ListFilter{Tag: "observation"})
	if len(list) != 1 {
		t.Fatalf("expected 1 observation, got %d", len(list))
	}
}

func TestAutoObserverNilStore(t *testing.T) {
	o := NewAutoObserver(nil)
	// Should not panic.
	o.Observe("edit", map[string]any{"path": "/tmp/foo"}, "result", true)
}

func TestAutoObserverRaceFree(t *testing.T) {
	s := tempStore(t)
	o := NewAutoObserver(s)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			o.Observe("edit", map[string]any{"path": "/tmp/race.go"}, "edit", true)
		}(i)
	}
	wg.Wait()
	// All 10 goroutines share the same tool+file → 1 grouped observation.
	list, _ := s.List(ListFilter{Tag: "observation"})
	if len(list) != 1 {
		t.Errorf("expected 1 grouped observation, got %d", len(list))
	}
}

func TestAutoObserverExtractFilePath(t *testing.T) {
	cases := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"path": "/tmp/x.go"}, "/tmp/x.go"},
		{map[string]any{"file": "main.py"}, "main.py"},
		{map[string]any{"filePath": "/abs/path"}, "/abs/path"},
		{map[string]any{"filename": "test.txt"}, "test.txt"},
		{map[string]any{"dest": "/out"}, "/out"},
		{map[string]any{"other": "val"}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		got := extractFilePath(c.args)
		if got != c.want {
			t.Errorf("extractFilePath(%v) = %q, want %q", c.args, got, c.want)
		}
	}
}
