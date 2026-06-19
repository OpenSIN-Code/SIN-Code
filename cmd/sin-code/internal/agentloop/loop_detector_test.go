// SPDX-License-Identifier: MIT
// Purpose: unit tests for LoopDetector (issue #377, M7 race-clean).
package agentloop

import (
	"sync"
	"testing"
)

func TestNewLoopDetector_Defaults(t *testing.T) {
	d := NewSimpleLoopDetector(0, 0)
	if d.maxRepeats != 1 || d.windowSize != 1 {
		t.Fatalf("clamped values = (%d,%d), want (1,1)", d.maxRepeats, d.windowSize)
	}
	if d.IsLooping() {
		t.Fatal("empty detector should not loop")
	}
}

func TestLoopDetector_SameCallRepeated(t *testing.T) {
	d := 	NewSimpleLoopDetector(3, 2)
	if d.Record("sin_read") {
		t.Fatal("single call should not loop")
	}
	if d.Record("sin_read") {
		t.Fatal("two calls should not loop")
	}
	if !d.Record("sin_read") {
		t.Fatal("three identical calls in a row should loop")
	}
}

func TestLoopDetector_NoLoop(t *testing.T) {
	d := 	NewSimpleLoopDetector(3, 2)
	for _, c := range []string{"sin_read", "sin_edit", "sin_read", "sin_edit", "sin_test"} {
		if d.Record(c) {
			t.Fatalf("Record(%q) unexpectedly detected a loop", c)
		}
	}
	if d.IsLooping() {
		t.Fatal("non-repeating history should not loop")
	}
}

func TestLoopDetector_SequenceRepeated(t *testing.T) {
	d := 	NewSimpleLoopDetector(3, 2)
	// A,B repeated 3 times => loop on the 6th call.
	seq := []string{"sin_read", "sin_edit", "sin_read", "sin_edit", "sin_read", "sin_edit"}
	for i, c := range seq {
		got := d.Record(c)
		if i == 5 {
			if !got {
				t.Fatal("expected loop on final repeated-sequence call")
			}
		} else if got {
			t.Fatalf("call %d (%q) unexpectedly looped", i, c)
		}
	}
}

func TestLoopDetector_AlternatingPattern(t *testing.T) {
	d := 	NewSimpleLoopDetector(2, 2)
	// A,B,A,B => windowSize=2, maxRepeats=2 => need=4.
	if d.Record("sin_read") {
		t.Fatal("1 should not loop")
	}
	if d.Record("sin_edit") {
		t.Fatal("2 should not loop")
	}
	if d.Record("sin_read") {
		t.Fatal("3 should not loop")
	}
	if !d.Record("sin_edit") {
		t.Fatal("4 (A,B,A,B) should loop")
	}
}

func TestLoopDetector_Reset(t *testing.T) {
	d := 	NewSimpleLoopDetector(2, 1)
	d.Record("sin_read")
	d.Record("sin_read")
	if !d.IsLooping() {
		t.Fatal("expected loop before reset")
	}
	d.Reset()
	if d.IsLooping() {
		t.Fatal("expected no loop after reset")
	}
	if d.Record("sin_read") {
		t.Fatal("single call after reset should not loop")
	}
}

func TestLoopDetector_IsLoopingIdempotent(t *testing.T) {
	d := 	NewSimpleLoopDetector(2, 1)
	d.Record("sin_read")
	d.Record("sin_read")
	if !d.IsLooping() {
		t.Fatal("expected loop")
	}
	if !d.IsLooping() {
		t.Fatal("IsLooping should remain true without mutation")
	}
}

func TestLoopDetector_Concurrent(t *testing.T) {
	d := 	NewSimpleLoopDetector(100, 2)
	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				_ = d.Record("sin_read")
				_ = d.IsLooping()
			}
		}()
	}
	wg.Wait()
	// 1000 identical calls recorded; allSameTail(100) must be true.
	if !d.IsLooping() {
		t.Fatal("expected loop after 1000 concurrent identical calls")
	}
	d.Reset()
	if d.IsLooping() {
		t.Fatal("expected no loop after concurrent reset")
	}
}
