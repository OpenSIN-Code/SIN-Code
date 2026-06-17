// SPDX-License-Identifier: MIT
// Purpose: Tests for typed inter-session messaging (issue #316).
package agentteams

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func newMessageBus(t *testing.T) *MessageBus {
	t.Helper()
	m := newMailbox(t)
	return NewMessageBus(m)
}

func TestMessageTypeString(t *testing.T) {
	cases := []struct {
		typ  MessageType
		want string
	}{
		{MsgHandoff, "TaskHandoff"},
		{MsgQuery, "Query"},
		{MsgResponse, "Response"},
		{MsgConflict, "Conflict"},
		{MsgStatus, "Status"},
		{MessageType(99), "Unknown(99)"},
	}
	for _, c := range cases {
		if got := c.typ.String(); got != c.want {
			t.Fatalf("MessageType(%d).String() = %q, want %q", c.typ, got, c.want)
		}
	}
}

func TestBusSendAndRecv(t *testing.T) {
	bus := newMessageBus(t)
	msg := Message{
		ID:      "m1",
		From:    "session-a",
		To:      "session-b",
		Type:    MsgHandoff,
		Subject: "take over auth refactor",
		Body:    "please handle the auth module",
	}
	if err := bus.Send(msg); err != nil {
		t.Fatal(err)
	}
	recv, err := bus.Recv("session-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 1 {
		t.Fatalf("want 1 message for session-b, got %d", len(recv))
	}
	if recv[0].ID != "m1" || recv[0].Type != MsgHandoff {
		t.Fatalf("unexpected message: %+v", recv[0])
	}
}

func TestBusRecvFiltersBySession(t *testing.T) {
	bus := newMessageBus(t)
	_ = bus.Send(Message{ID: "a", From: "s1", To: "s2", Type: MsgStatus, Subject: "x", Body: "b"})
	_ = bus.Send(Message{ID: "b", From: "s1", To: "s3", Type: MsgStatus, Subject: "x", Body: "b"})
	recv, err := bus.Recv("s2")
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 1 || recv[0].ID != "a" {
		t.Fatalf("want only message a for s2, got %+v", recv)
	}
}

func TestBusRecvIncludesBroadcast(t *testing.T) {
	bus := newMessageBus(t)
	_ = bus.Send(Message{ID: "bc", From: "s1", To: "broadcast", Type: MsgStatus, Subject: "x", Body: "b"})
	_ = bus.Send(Message{ID: "dm", From: "s1", To: "s2", Type: MsgStatus, Subject: "x", Body: "b"})
	recv, err := bus.Recv("s3")
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 1 || recv[0].ID != "bc" {
		t.Fatalf("want broadcast only for s3, got %+v", recv)
	}
}

func TestBusBroadcast(t *testing.T) {
	bus := newMessageBus(t)
	if err := bus.Broadcast(Message{ID: "br", From: "s1", Type: MsgStatus, Subject: "hi", Body: "all"}); err != nil {
		t.Fatal(err)
	}
	for _, sid := range []string{"s2", "s3", "any"} {
		recv, err := bus.Recv(sid)
		if err != nil {
			t.Fatal(err)
		}
		if len(recv) != 1 || recv[0].ID != "br" || recv[0].To != "broadcast" {
			t.Fatalf("session %s: want broadcast msg, got %+v", sid, recv)
		}
	}
}

func TestBusReply(t *testing.T) {
	bus := newMessageBus(t)
	orig := Message{ID: "q1", From: "s1", To: "s2", Type: MsgQuery, Subject: "what files?", Body: "?"}
	if err := bus.Send(orig); err != nil {
		t.Fatal(err)
	}
	if err := bus.Reply("q1", Message{ID: "r1", From: "s2", To: "s1", Subject: "re: what files?", Body: "3 files"}); err != nil {
		t.Fatal(err)
	}
	recv, err := bus.Recv("s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != 1 {
		t.Fatalf("want 1 reply for s1, got %d", len(recv))
	}
	if recv[0].ReplyTo != "q1" || recv[0].Type != MsgResponse {
		t.Fatalf("reply must set ReplyTo and Type=MsgResponse: %+v", recv[0])
	}
}

func TestBusResolveConflict(t *testing.T) {
	bus := newMessageBus(t)
	c := Conflict{ID: "c1", Sessions: []string{"s1", "s2"}, Resource: "main.go"}
	body, _ := json.Marshal(c)
	_ = bus.Send(Message{ID: "c1", From: "s1", To: "s2", Type: MsgConflict, Subject: "edit clash", Body: string(body)})

	if err := bus.ResolveConflict("c1", "s1 wins, s2 rebases"); err != nil {
		t.Fatal(err)
	}
	got, ok := bus.GetConflict("c1")
	if !ok {
		t.Fatal("conflict c1 should be tracked")
	}
	if got.Resolution != "s1 wins, s2 rebases" {
		t.Fatalf("resolution mismatch: %q", got.Resolution)
	}
	all, _ := bus.mailbox.Receive()
	if len(all) != 1 || !all[0].Resolved {
		t.Fatalf("conflict message must be marked resolved: %+v", all[0])
	}
}

func TestBusResolveConflictNotFound(t *testing.T) {
	bus := newMessageBus(t)
	if err := bus.ResolveConflict("nope", "x"); err == nil {
		t.Fatal("should error on unknown conflict")
	}
}

func TestBusSendValidatesFields(t *testing.T) {
	bus := newMessageBus(t)
	if err := bus.Send(Message{From: "x", To: "y"}); err == nil {
		t.Fatal("empty ID must error")
	}
	if err := bus.Send(Message{ID: "x", To: "y"}); err == nil {
		t.Fatal("empty From must error")
	}
	if err := bus.Send(Message{ID: "x", From: "y"}); err == nil {
		t.Fatal("empty To must error")
	}
}

func TestBusRecvEmptySessionID(t *testing.T) {
	bus := newMessageBus(t)
	if _, err := bus.Recv(""); err == nil {
		t.Fatal("empty sessionID must error")
	}
}

func TestBusLoadConflicts(t *testing.T) {
	m := newMailbox(t)
	c := Conflict{ID: "c1", Sessions: []string{"s1", "s2"}, Resource: "main.go"}
	body, _ := json.Marshal(c)
	_, _, _ = m.Send(Message{ID: "c1", From: "s1", To: "s2", Type: MsgConflict, Subject: "clash", Body: string(body)})

	bus := NewMessageBus(m)
	if err := bus.LoadConflicts(); err != nil {
		t.Fatal(err)
	}
	got, ok := bus.GetConflict("c1")
	if !ok {
		t.Fatal("LoadConflicts should index existing conflict")
	}
	if got.Resource != "main.go" {
		t.Fatalf("resource mismatch: %q", got.Resource)
	}
}

func TestBusConcurrentSend(t *testing.T) {
	bus := newMessageBus(t)
	const N = 20
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			_ = bus.Send(Message{
				ID:      fmt.Sprintf("bus-%02d", i),
				From:    "producer",
				To:      "broadcast",
				Type:    MsgStatus,
				Subject: "concurrent",
				Body:    "body",
			})
		}(i)
	}
	wg.Wait()
	recv, err := bus.Recv("any")
	if err != nil {
		t.Fatal(err)
	}
	if len(recv) != N {
		t.Fatalf("want %d messages, got %d", N, len(recv))
	}
}

func TestBusSendSetsTimestamp(t *testing.T) {
	bus := newMessageBus(t)
	before := time.Now().UTC()
	_ = bus.Send(Message{ID: "ts", From: "s1", To: "s2", Type: MsgStatus, Subject: "x", Body: "b"})
	recv, _ := bus.Recv("s2")
	if len(recv) != 1 {
		t.Fatal("want 1 message")
	}
	if recv[0].SentAt.Before(before) {
		t.Fatalf("SentAt should be >= send time: %v < %v", recv[0].SentAt, before)
	}
}
