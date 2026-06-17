// SPDX-License-Identifier: MIT
// Purpose: race-clean tests for the agent-team mailbox.
package agentteams

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func newMailbox(t *testing.T) *Mailbox {
	t.Helper()
	root := t.TempDir()
	m, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestSendAndReceive(t *testing.T) {
	m := newMailbox(t)
	_, dedup, err := m.Send(Message{
		ID:      "msg-1",
		From:    "alice",
		To:      "broadcast",
		Subject: "kickoff",
		Body:    "team up",
	})
	if err != nil {
		t.Fatal(err)
	}
	if dedup {
		t.Fatal("first send must not deduplicate")
	}
	all, err := m.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("want 1 message, got %d", len(all))
	}
	if all[0].ID != "msg-1" || all[0].From != "alice" {
		t.Fatalf("unexpected content: %+v", all[0])
	}
}

func TestSendDedupe(t *testing.T) {
	m := newMailbox(t)
	for i := 0; i < 5; i++ {
		_, dedup, err := m.Send(Message{ID: "dup", From: "x", To: "y", Subject: "s", Body: "b"})
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && dedup {
			t.Fatal("first send must not dedupe")
		}
		if i > 0 && !dedup {
			t.Fatalf("repeat send (%d) must dedupe", i)
		}
	}
	all, _ := m.Receive()
	if len(all) != 1 {
		t.Fatalf("want 1 message after dedupe, got %d", len(all))
	}
}

func TestConcurrentSend(t *testing.T) {
	m := newMailbox(t)
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_, _, err := m.Send(Message{
				ID:      fmt.Sprintf("msg-%02d", i),
				From:    "alice",
				To:      "broadcast",
				Subject: "concurrent",
				Body:    fmt.Sprintf("body %d", i),
			})
			if err != nil {
				t.Errorf("send %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	all, _ := m.Receive()
	if len(all) != N {
		t.Fatalf("want %d messages, got %d", N, len(all))
	}
	// spot-check that all 20 distinct IDs are present.
	seen := map[string]bool{}
	for _, msg := range all {
		seen[msg.ID] = true
	}
	for i := 0; i < N; i++ {
		want := fmt.Sprintf("msg-%02d", i)
		if !seen[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestDrainAfterID(t *testing.T) {
	m := newMailbox(t)
	_, _, err := m.Send(Message{ID: "a", From: "x", Subject: "sub", Body: "a"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Send(Message{ID: "b", From: "x", Subject: "sub", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := m.DrainAfterID("a")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 1 || after[0].ID != "b" {
		t.Fatalf("want [b], got %v", after)
	}
}

func TestMarkResolved(t *testing.T) {
	m := newMailbox(t)
	_, _, err := m.Send(Message{ID: "r1", Subject: "sub", From: "x", Body: "b"})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkResolved("r1"); err != nil {
		t.Fatal(err)
	}
	all, _ := m.Receive()
	if len(all) != 1 || !all[0].Resolved {
		t.Fatalf("resolved flag must be set: %+v", all[0])
	}
	if err := m.MarkResolved("missing"); err == nil {
		t.Fatal("missing message must error")
	}
}

func TestStats(t *testing.T) {
	m := newMailbox(t)
	now := time.Date(2026, 6, 17, 1, 0, 0, 0, time.UTC)
	_, _, err := m.Send(Message{ID: "s1", Subject: "x", Body: "x", SentAt: now})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = m.Send(Message{ID: "s2", Subject: "x", Body: "x", SentAt: now.Add(time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.MarkResolved("s1"); err != nil {
		t.Fatal(err)
	}
	stats, err := m.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Unresolved != 1 {
		t.Fatalf("want Total=2 Unresolved=1, got %+v", stats)
	}
}

func TestEmptySubjectHoldsNewlines(t *testing.T) {
	m := newMailbox(t)
	_, _, err := m.Send(Message{
		ID:      "multi",
		From:    "alice",
		Subject: "single-line",
		Body:    "line one\nline two\nline three",
	})
	if err != nil {
		t.Fatal(err)
	}
	all, _ := m.Receive()
	if len(all) != 1 {
		t.Fatal("want 1 message")
	}
	if !strings.Contains(all[0].Body, "line one\nline two\nline three") {
		t.Fatalf("body must roundtrip newlines: %q", all[0].Body)
	}
}

func TestEmptyInboxReceive(t *testing.T) {
	m := newMailbox(t)
	all, err := m.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("want empty, got %d", len(all))
	}
}

func TestSendRefusesEmptyID(t *testing.T) {
	m := newMailbox(t)
	if _, _, err := m.Send(Message{From: "x", Subject: "y"}); err == nil {
		t.Fatal("empty ID must error")
	}
}

func TestOpenNilWorkspace(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("empty workspace must error")
	}
}
