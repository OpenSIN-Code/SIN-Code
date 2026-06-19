// SPDX-License-Identifier: MIT
// Purpose: observer-loop prevention (issue #377). LoopDetector tracks the
// most recent tool calls and flags two failure modes deterministically:
//  1. the same tool call repeated maxRepeats times in a row, and
//  2. the same sequence of windowSize calls repeated maxRepeats times
//     (covers alternating A->B->A->B patterns when windowSize=2).
//
// Record returns true the moment a loop is detected. Thread-safe (M7).
package agentloop

import "sync"

// LoopDetector detects repeated tool-call sequences in the agent loop.
type LoopDetector struct {
	maxRepeats int
	windowSize int

	calls []string
	mu    sync.Mutex
}

// NewLoopDetector returns a detector that flags a loop when the same call
// repeats maxRepeats times in a row, or when a windowSize-length sequence
// repeats maxRepeats times. Values below 1 are clamped to 1.
func NewLoopDetector(maxRepeats, windowSize int) *LoopDetector {
	if maxRepeats < 1 {
		maxRepeats = 1
	}
	if windowSize < 1 {
		windowSize = 1
	}
	return &LoopDetector{maxRepeats: maxRepeats, windowSize: windowSize}
}

// Record appends a tool call and returns true if a loop is detected after
// appending it.
func (d *LoopDetector) Record(toolCall string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, toolCall)
	return d.detectLocked()
}

// Reset clears the recorded call history.
func (d *LoopDetector) Reset() {
	d.mu.Lock()
	d.calls = nil
	d.mu.Unlock()
}

// IsLooping reports whether the current recorded history contains a loop
// without appending a new call.
func (d *LoopDetector) IsLooping() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.detectLocked()
}

// detectLocked evaluates the recorded history. The caller must hold d.mu.
func (d *LoopDetector) detectLocked() bool {
	n := len(d.calls)
	if n == 0 {
		return false
	}

	if n >= d.maxRepeats && d.allSameTail(d.maxRepeats) {
		return true
	}

	need := d.windowSize * d.maxRepeats
	if n >= need && d.repeatedBlock(need) {
		return true
	}

	return false
}

// allSameTail reports whether the last count calls are all identical.
func (d *LoopDetector) allSameTail(count int) bool {
	tail := d.calls[len(d.calls)-count:]
	first := tail[0]
	for i := 1; i < len(tail); i++ {
		if tail[i] != first {
			return false
		}
	}
	return true
}

// repeatedBlock reports whether the last need calls are maxRepeats
// repetitions of the leading windowSize-length block.
func (d *LoopDetector) repeatedBlock(need int) bool {
	tail := d.calls[len(d.calls)-need:]
	block := tail[:d.windowSize]
	for r := 1; r < d.maxRepeats; r++ {
		off := r * d.windowSize
		for j := 0; j < d.windowSize; j++ {
			if tail[off+j] != block[j] {
				return false
			}
		}
	}
	return true
}
