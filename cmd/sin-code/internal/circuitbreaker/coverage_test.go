// SPDX-License-Identifier: MIT
// Purpose: additional coverage tests for circuitbreaker package to reach 100% statement coverage.
package circuitbreaker

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestStateString_AllStates(t *testing.T) {
	cases := []struct {
		state State
		want  string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "state(99)"},
	}
	for _, c := range cases {
		if got := c.state.String(); got != c.want {
			t.Errorf("%v: got %q, want %q", c.state, got, c.want)
		}
	}
}

func TestConfigString(t *testing.T) {
	if got := (*Config)(nil).String(); got != "breaker(none)" {
		t.Errorf("nil config: got %q", got)
	}
	if got := (&Config{}).String(); got != "breaker(unnamed)" {
		t.Errorf("empty config: got %q", got)
	}
	if got := (&Config{Name: "upstream"}).String(); got != "breaker(upstream)" {
		t.Errorf("named config: got %q", got)
	}
}

func TestNew_NilConfig(t *testing.T) {
	b := New(nil)
	if b.cfg.Name != "unnamed" {
		t.Fatalf("expected default name, got %q", b.cfg.Name)
	}
	if b.cfg.FailureThreshold == 0 {
		t.Fatal("expected default failure threshold")
	}
}

func TestRecordSuccess_ClosedClearsFailures(t *testing.T) {
	b := New(&Config{FailureThreshold: 3})
	b.consecFails = 2
	b.RecordSuccess()
	if b.Stats().ConsecutiveFails != 0 {
		t.Fatal("expected consecutive fails reset")
	}
}

func TestRecordFailure_Directly(t *testing.T) {
	b := New(&Config{FailureThreshold: 1})
	b.RecordFailure(errors.New("direct"))
	if b.State() != StateOpen {
		t.Fatal("expected open after direct failure")
	}
}

func TestMaybeAdmit_HalfOpenFull(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cl := newClock(start)
	b := New(&Config{
		FailureThreshold: 1, OpenDuration: 5 * time.Second,
		HalfOpenProbes: 1, SuccessThreshold: 1, Now: cl.Now,
	})
	_ = b.Execute(failingFn(errors.New("trip")))
	cl.Advance(10 * time.Second)
	// First probe admitted.
	if err := b.IsAllowed(); err != nil {
		t.Fatalf("first probe should be admitted: %v", err)
	}
	// Second probe rejected because halfInFlight == HalfOpenProbes.
	if err := b.IsAllowed(); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen for over-probe, got %v", err)
	}
}

func TestInvalidState_AdmitAndRecord(t *testing.T) {
	b := New(&Config{})
	b.state = State(99)
	if err := b.IsAllowed(); err == nil {
		t.Fatal("expected rejection in invalid state")
	}
	b.RecordSuccess()
	b.RecordFailure(errors.New("x"))
}

func TestRecordSuccess_OpenState(t *testing.T) {
	b := New(&Config{})
	b.state = StateOpen
	b.halfInFlight = 1
	b.RecordSuccess()
	if b.halfInFlight != 0 {
		t.Fatal("expected halfInFlight decremented in open state")
	}
}

func TestRecordFailure_OpenState(t *testing.T) {
	b := New(&Config{})
	b.state = StateOpen
	b.halfInFlight = 1
	b.RecordFailure(errors.New("x"))
	if b.halfInFlight != 0 {
		t.Fatal("expected halfInFlight decremented in open state")
	}
}

func TestApplyDefaults_AllBranches(t *testing.T) {
	c := Config{
		FailureThreshold: 0, OpenDuration: 0,
		HalfOpenProbes: 0, SuccessThreshold: 0, Now: nil,
	}
	c.applyDefaults()
	if c.FailureThreshold != 5 || c.OpenDuration != 10*time.Second ||
		c.HalfOpenProbes != 1 || c.SuccessThreshold != 1 || c.Now == nil {
		t.Fatalf("defaults not applied: %+v", c)
	}
}

func TestRoundTripper_NilBreaker(t *testing.T) {
	rt := RoundTripper(http.DefaultTransport, nil)
	if rt != http.DefaultTransport {
		t.Fatal("expected nil breaker to return inner transport")
	}
}

func TestRoundTripper_NilInner(t *testing.T) {
	b := New(&Config{})
	rt := RoundTripper(nil, b)
	if rt == nil {
		t.Fatal("expected non-nil round tripper")
	}
}

func TestClassifyHTTPError_Transport(t *testing.T) {
	transportErr := errors.New("dial fail")
	if err := classifyHTTPError(nil, transportErr); err == nil || !strings.Contains(err.Error(), "transport error") {
		t.Fatalf("expected transport error, got %v", err)
	}
}

func TestClassifyHTTPError_NilResponse(t *testing.T) {
	if err := classifyHTTPError(nil, nil); err == nil || !strings.Contains(err.Error(), "nil response") {
		t.Fatalf("expected nil response error, got %v", err)
	}
}
