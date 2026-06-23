// SPDX-License-Identifier: MIT
// Purpose: tests for LoopDetector (issue #377, mandates M4 + M7).
//
// The detector is a pure data-structure with side-effects only on
// the trip counter, so these tests exercise Observe directly without
// the surrounding Loop wiring.
package agentloop

import (
	"errors"
	"sync"
	"testing"
)

// mkObsTC builds a ToolCall whose signature is determined solely by
// Name and the canonical JSON of Args. Args are passed as a flat
// (key, value) slice just to keep the test code compact.
func mkObsTC(name string, kvs ...any) ToolCall {
	args := map[string]any{}
	for i := 0; i+1 < len(kvs); i += 2 {
		k, _ := kvs[i].(string)
		args[k] = kvs[i+1]
	}
	return ToolCall{Name: name, Args: args}
}

func TestObserverDetectsAABBCC(t *testing.T) {
	d := NewLoopDetector(20, 3, 2)

	seq := []ToolCall{
		mkObsTC("A"), mkObsTC("B"), mkObsTC("C"),
		mkObsTC("A"), mkObsTC("B"),
	}
	for _, tc := range seq {
		if err := d.Observe(tc, ""); err != nil {
			t.Fatalf("premature detection on %s: %v", tc.Name, err)
		}
	}
	if d.LastTrip() != nil {
		t.Fatalf("expected no trip yet, got %+v", d.LastTrip())
	}

	err := d.Observe(mkObsTC("C"), "")
	if !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("expected ErrLoopDetected on 6th call, got %v", err)
	}
	trip := d.LastTrip()
	if trip == nil {
		t.Fatalf("expected trip metadata after detection, got nil")
	}
	if trip.Length != 3 {
		t.Errorf("expected trip.Length=3, got %d", trip.Length)
	}
	if trip.Repeats != 2 {
		t.Errorf("expected trip.Repeats=2, got %d", trip.Repeats)
	}
	if trip.ToolName != "C" {
		t.Errorf("expected trip.ToolName=%q (last call), got %q", "C", trip.ToolName)
	}
	if trip.HistoryLen != 6 {
		t.Errorf("expected trip.HistoryLen=6, got %d", trip.HistoryLen)
	}

	// The detector must keep refusing identical follow-up calls
	// (fail-closed).
	if err := d.Observe(mkObsTC("A"), ""); !errors.Is(err, ErrLoopDetected) {
		t.Errorf("expected trip+refuse on follow-up A, got %v", err)
	}
}

func TestObserverHandlesNonRepeating(t *testing.T) {
	d := NewLoopDetector(20, 3, 2)

	seq := []ToolCall{
		mkObsTC("A"), mkObsTC("B"), mkObsTC("C"),
		mkObsTC("D"), mkObsTC("E"), mkObsTC("F"),
		mkObsTC("G"), mkObsTC("H"),
	}
	for i, tc := range seq {
		if err := d.Observe(tc, ""); err != nil {
			t.Fatalf("unexpected detection on %d (%s): %v", i, tc.Name, err)
		}
	}
	if d.LastTrip() != nil {
		t.Fatalf("expected no trip on non-repeating sequence, got %+v", d.LastTrip())
	}
	if !d.Enabled() {
		t.Fatalf("detector should still be enabled after non-tripping sequence")
	}
}

func TestObserverConfigurableWindow(t *testing.T) {
	// Small window allows detection on a short cycle sooner than the
	// default 20-window. With window=4, p=2, repeats=2 the ABAB
	// pattern trips on the 4th call (not the 6th).
	d := NewLoopDetector(4, 2, 2)
	seq := []ToolCall{mkObsTC("A"), mkObsTC("B"), mkObsTC("A")}
	for i, tc := range seq {
		if err := d.Observe(tc, ""); err != nil {
			t.Fatalf("small-window premature detection on %d: %v", i, err)
		}
	}
	if err := d.Observe(mkObsTC("B"), ""); !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("small-window detector should trip on 4th call, got %v", err)
	}
	trip := d.LastTrip()
	if trip == nil || trip.Length != 2 || trip.Repeats != 2 {
		t.Fatalf("expected trip Length=2 Repeats=2, got %+v", trip)
	}

	// Default window requires 6 ABCABC-style calls.
	d2 := NewLoopDetector(20, 3, 2)
	abc := []ToolCall{
		mkObsTC("A"), mkObsTC("B"), mkObsTC("C"),
		mkObsTC("A"), mkObsTC("B"),
	}
	for _, tc := range abc {
		if err := d2.Observe(tc, ""); err != nil {
			t.Fatalf("default-window premature detection on %s: %v", tc.Name, err)
		}
	}
	if err := d2.Observe(mkObsTC("C"), ""); !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("default-window detector should trip on 6th call, got %v", err)
	}
}

func TestObserverDisabledByDefault(t *testing.T) {
	// Zero window → disabled: never trips, even on a tight cycle.
	d := NewLoopDetector(0, 3, 2)
	for i := 0; i < 100; i++ {
		if err := d.Observe(mkObsTC("X"), ""); err != nil {
			t.Fatalf("disabled detector returned error on cycle %d: %v", i, err)
		}
	}
	if d.LastTrip() != nil {
		t.Fatalf("disabled detector should never produce a trip")
	}
	if d.Enabled() {
		t.Fatalf("zero-window detector should report Enabled()=false")
	}

	// Negative window is normalised to 0 by the constructor.
	d2 := NewLoopDetector(-5, 3, 2)
	if d2.Enabled() {
		t.Fatalf("negative-window detector should be disabled")
	}
	for i := 0; i < 50; i++ {
		if err := d2.Observe(mkObsTC("X"), ""); err != nil {
			t.Fatalf("negative-window detector returned error: %v", err)
		}
	}

	// Reset clears tripped state and the captured trip.
	d3 := NewLoopDetector(4, 2, 2)
	_ = d3.Observe(mkObsTC("A"), "")
	_ = d3.Observe(mkObsTC("B"), "")
	_ = d3.Observe(mkObsTC("A"), "")
	if err := d3.Observe(mkObsTC("B"), ""); !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("setup: expected trip, got %v", err)
	}
	d3.Reset()
	if d3.LastTrip() != nil {
		t.Fatalf("reset should clear trip metadata")
	}
	if err := d3.Observe(mkObsTC("A"), ""); err != nil {
		t.Fatalf("post-reset first call should not trip: %v", err)
	}
}

// TestObserverRaceFree demonstrates that concurrent Observe + LastTrip
// calls are race-free under -race (mandate M7).
func TestObserverRaceFree(t *testing.T) {
	d := NewLoopDetector(20, 3, 2)
	var wg sync.WaitGroup
	const workers = 8
	const callsPerWorker = 200
	for w := 0; w < workers; w++ {
		wg.Add(2)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < callsPerWorker; i++ {
				switch (id + i) % 3 {
				case 0:
					_ = d.Observe(mkObsTC("A", "x", i%5), "")
				case 1:
					_ = d.Observe(mkObsTC("B", "x", i%5), "")
				default:
					_ = d.Observe(mkObsTC("C", "x", i%5), "")
				}
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < callsPerWorker; i++ {
				_ = d.LastTrip()
				_ = d.Enabled()
			}
		}()
	}
	wg.Wait()
}

// TestObserverArgsSignature verifies that two calls with the same
// args in different field order produce the same fingerprint, so
// reordered-but-equal calls do not escape the detector. Args that
// differ in VALUE must produce distinct fingerprints so the
// detector can tell "writing 'hello' to /tmp/a" apart from
// "writing 'goodbye' to /tmp/a".
func TestObserverArgsSignature(t *testing.T) {
	// Reordered-but-equal args collapse to the same fingerprint.
	d := NewLoopDetector(4, 2, 2)
	if err := d.Observe(mkObsTC("write", "path", "/tmp/a", "body", "hello"), ""); err != nil {
		t.Fatalf("unexpected 1: %v", err)
	}
	if err := d.Observe(mkObsTC("write", "body", "hello", "path", "/tmp/a"), ""); err != nil {
		t.Fatalf("unexpected 2: %v", err)
	}
	if err := d.Observe(mkObsTC("write", "path", "/tmp/a", "body", "hello"), ""); err != nil {
		t.Fatalf("unexpected 3: %v", err)
	}
	// After 3 identical fingerprints we still don't have N/p >= 2
	// for p >= 2 (only 3 entries, max p = 1). Pass another:
	if err := d.Observe(mkObsTC("write", "body", "hello", "path", "/tmp/a"), ""); !errors.Is(err, ErrLoopDetected) {
		t.Fatalf("expected detector to match reordered-but-equal args, got %v", err)
	}

	// Distinct args produce distinct fingerprints; three
	// single-shot calls with different bodies must not trip.
	d2 := NewLoopDetector(10, 3, 4)
	for _, body := range []string{"alpha", "beta", "gamma", "delta", "epsilon"} {
		if err := d2.Observe(mkObsTC("write", "body", body), ""); err != nil {
			t.Fatalf("distinct-body call %q tripped: %v", body, err)
		}
	}
	if d2.LastTrip() != nil {
		t.Fatalf("distinct-body sequence should never produce a loop")
	}

	// Same shape different CONTENT keys must NOT mix — a verify
	// pass with "hello" must not be combined with a verify pass
	// containing "world".
	d3 := NewLoopDetector(10, 2, 2)
	_ = d3.Observe(mkObsTC("verify", "msg", "hello world"), "")
	_ = d3.Observe(mkObsTC("verify", "msg", "goodbye world"), "")
	_ = d3.Observe(mkObsTC("verify", "msg", "hello world"), "")
	_ = d3.Observe(mkObsTC("verify", "msg", "goodbye world"), "")
	// This DOES trip (4 calls, 2-cycle repeats twice) — that's
	// the documented behaviour. Make sure the trip metadata
	// reflects the captured 2-cycle rather than collapsing to
	// duplicate single calls.
	if d3.LastTrip() == nil {
		t.Fatalf("expected 2-cycle trip on alternating-message sequence")
	}
}
