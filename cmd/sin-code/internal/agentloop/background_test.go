// SPDX-License-Identifier: MIT
// Purpose: tests for issue #195 — background task registry.
package agentloop

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTaskRegistry_AddGet(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("first")
	if t1.ID == "" {
		t.Fatal("expected non-empty id")
	}
	if t1.Status != "running" {
		t.Errorf("expected status=running, got %q", t1.Status)
	}
	got, err := r.Get(t1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != t1.ID {
		t.Errorf("id mismatch: %q != %q", got.ID, t1.ID)
	}
}

func TestTaskRegistry_GetNotFound(t *testing.T) {
	r := NewTaskRegistry()
	_, err := r.Get("bg-999")
	if !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskRegistry_Finish(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("x")
	res := &Result{Summary: "ok", Verified: true, Turns: 1}
	r.Finish(t1.ID, "verified", res, nil)
	got, _ := r.Get(t1.ID)
	if got.Status != "verified" {
		t.Errorf("expected status=verified, got %q", got.Status)
	}
	if got.Result == nil || got.Result.Summary != "ok" {
		t.Errorf("expected result.summary=ok, got %+v", got.Result)
	}
	if got.FinishedAt.IsZero() {
		t.Error("expected FinishedAt to be set after Finish")
	}
}

func TestTaskRegistry_FinishWithError(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("x")
	r.Finish(t1.ID, "failed", nil, errors.New("boom"))
	got, _ := r.Get(t1.ID)
	if got.Status != "failed" {
		t.Errorf("expected status=failed, got %q", got.Status)
	}
	if got.Err != "boom" {
		t.Errorf("expected err=boom, got %q", got.Err)
	}
}

func TestTaskRegistry_ListNewestFirst(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("a")
	time.Sleep(time.Millisecond)
	_ = r.Add("b")
	time.Sleep(time.Millisecond)
	t3 := r.Add("c")
	list := r.List()
	if len(list) != 3 {
		t.Fatalf("expected 3, got %d", len(list))
	}
	if list[0].ID != t3.ID {
		t.Errorf("expected newest first (t3), got %s", list[0].ID)
	}
	if list[2].ID != t1.ID {
		t.Errorf("expected oldest last (t1), got %s", list[2].ID)
	}
}

func TestTaskRegistry_Cancel(t *testing.T) {
	r := NewTaskRegistry()
	t1 := r.Add("x")
	ctx, cancel := context.WithCancel(context.Background())
	r.SetCancel(t1.ID, cancel)
	if err := r.Cancel(t1.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(100 * time.Millisecond):
		t.Error("expected ctx to be canceled")
	}
}

func TestTaskRegistry_CancelNotFound(t *testing.T) {
	r := NewTaskRegistry()
	if err := r.Cancel("bg-999"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("expected ErrTaskNotFound, got %v", err)
	}
}

func TestTaskRegistry_Concurrent(t *testing.T) {
	r := NewTaskRegistry()
	const n = 50
	done := make(chan struct{}, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			t1 := r.Add("x")
			r.SetCancel(t1.ID, func() {})
			r.Finish(t1.ID, "verified", nil, nil)
			r.Get(t1.ID)
			r.List()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < n; i++ {
		<-done
	}
	if got := len(r.List()); got != n {
		t.Errorf("expected %d tasks, got %d", n, got)
	}
}

func TestItoa3(t *testing.T) {
	cases := map[int]string{
		0:   "000",
		1:   "001",
		10:  "010",
		100: "100",
		999: "999",
	}
	for in, want := range cases {
		if got := itoa3(in); got != want {
			t.Errorf("itoa3(%d) = %q, want %q", in, got, want)
		}
	}
}
