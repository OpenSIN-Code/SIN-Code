// SPDX-License-Identifier: MIT
package hooks

import (
	"context"
	"errors"
	"testing"
)

func TestTriggerThreadsContext(t *testing.T) {
	hm := NewManager()
	hm.Register(AfterToolExecution, func(hc *Context) (*Context, error) {
		hc.LoopCount++
		return hc, nil
	})
	hm.Register(AfterToolExecution, func(hc *Context) (*Context, error) {
		hc.LoopCount += 10
		return hc, nil
	})
	out, err := hm.Trigger(AfterToolExecution, &Context{Ctx: context.Background()})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if out.LoopCount != 11 {
		t.Errorf("LoopCount = %d, want 11 (hooks ran in order)", out.LoopCount)
	}
}

func TestTriggerStopsOnError(t *testing.T) {
	hm := NewManager()
	ran := 0
	hm.Register(BeforeToolExecution, func(hc *Context) (*Context, error) {
		ran++
		return hc, errors.New("boom")
	})
	hm.Register(BeforeToolExecution, func(hc *Context) (*Context, error) {
		ran++
		return hc, nil
	})
	_, err := hm.Trigger(BeforeToolExecution, &Context{})
	if err == nil {
		t.Fatal("expected error")
	}
	if ran != 1 {
		t.Errorf("ran = %d, want 1 (second hook should be skipped)", ran)
	}
}

func TestDefaultHookRegistered(t *testing.T) {
	hm := GetManager()
	if hm.Count(AfterToolExecution) < 1 {
		t.Error("expected default AfterToolExecution hook")
	}
}

func TestRegisterAndCount(t *testing.T) {
	hm := NewManager()
	if hm.Count(OnLoopFailure) != 0 {
		t.Error("expected 0 hooks initially")
	}
	hm.Register(OnLoopFailure, func(hc *Context) (*Context, error) { return hc, nil })
	if hm.Count(OnLoopFailure) != 1 {
		t.Error("expected 1 hook after register")
	}
}

func TestEmptyTriggerNoop(t *testing.T) {
	hm := NewManager()
	hc := &Context{ToolName: "x"}
	out, err := hm.Trigger(OnLoopFailure, hc)
	if err != nil || out != hc {
		t.Errorf("empty trigger should return input unchanged: out=%v err=%v", out, err)
	}
}
