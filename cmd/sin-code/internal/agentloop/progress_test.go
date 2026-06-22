// SPDX-License-Identifier: MIT
// Purpose: race-safe tests for the headless structured progress writer.
package agentloop

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
)

func TestProgressWriterWritesValidJSONLines(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf)

	ev := ProgressEvent{Event: "turn.start", Turn: 1, Level: "info"}
	pw.Write(ev)
	pw.Write(ProgressEvent{Event: "tool.pre", Tool: "sin_test"})
	pw.Write(ProgressEvent{Event: "tool.post", Tool: "sin_test", Data: map[string]any{"output_bytes": 42}})
	pw.Close()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 NDJSON lines, got %d: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		var got ProgressEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d is not valid JSON: %v\n%s", i, err, line)
		}
		if got.Ts == "" {
			t.Errorf("line %d: ts field must be set", i)
		}
		if got.Event == "" {
			t.Errorf("line %d: event field must be set", i)
		}
	}
	if !strings.Contains(lines[0], "turn.start") {
		t.Errorf("first line should be turn.start, got %s", lines[0])
	}
}

func TestProgressWriterNilSafe(t *testing.T) {
	var pw *ProgressWriter
	pw.Write(ProgressEvent{Event: "should.not.panic"}) // must not panic
}

func TestProgressWriterNilWriterNoop(t *testing.T) {
	pw := NewProgressWriter(nil)
	pw.Write(ProgressEvent{Event: "should.not.panic"}) // must not panic
}

func TestProgressWriterClosedIsNoop(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf)
	pw.Write(ProgressEvent{Event: "before"})
	pw.Close()
	pw.Write(ProgressEvent{Event: "after"})

	if !strings.Contains(buf.String(), "before") {
		t.Errorf("expected 'before' event to be written")
	}
	if strings.Contains(buf.String(), "after") {
		t.Errorf("closed writer must not emit further events")
	}
}

func TestProgressWriterRaceSafe(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			pw.Write(ProgressEvent{Event: "race", Turn: n})
		}(i)
	}
	wg.Wait()
	pw.Close()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 100 {
		t.Fatalf("expected 100 lines, got %d", len(lines))
	}
	for i, line := range lines {
		var got ProgressEvent
		if err := json.Unmarshal([]byte(line), &got); err != nil {
			t.Fatalf("line %d invalid JSON: %v\n%s", i, err, line)
		}
	}
}

func TestProgressWriterDecorate(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf)
	pw.Decorate = func(ev ProgressEvent) ProgressEvent {
		ev.GoalID = 7
		ev.WorkerID = 3
		return ev
	}
	pw.Write(ProgressEvent{Event: "test"})

	var got ProgressEvent
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.GoalID != 7 || got.WorkerID != 3 {
		t.Errorf("decorate not applied: got goal_id=%d worker_id=%d", got.GoalID, got.WorkerID)
	}
}

func TestLoopEmitProgressNilWriter(t *testing.T) {
	l := &Loop{SessionID: "sess-1"}
	// Should not panic when ProgressWriter is nil.
	l.emitProgress(ProgressEvent{Event: "test"})
}

func TestLoopEmitProgressSetsSessionID(t *testing.T) {
	var buf bytes.Buffer
	pw := NewProgressWriter(&buf)
	l := &Loop{ProgressWriter: pw, SessionID: "sess-1"}
	l.emitProgress(ProgressEvent{Event: "test"})

	var got ProgressEvent
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want sess-1", got.SessionID)
	}
	if got.Event != "test" {
		t.Errorf("event = %q, want test", got.Event)
	}
}
