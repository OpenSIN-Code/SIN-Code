// SPDX-License-Identifier: MIT
// Purpose: backward-compatible methods on the LoopDetector type
// declared in loop_observer.go (issue #377). The simple Record/IsLooping
// API predates the full Observe/LastTrip API and is retained for
// builder.go's RepetitionThreshold wiring.
package agentloop

// NewSimpleLoopDetector returns a detector configured for simple
// consecutive-repeat detection: flags when the same tool call repeats
// maxRepeats times in a row. The windowSize parameter is mapped to the
// observer Window. This is the 2-arg compatibility constructor.
func NewSimpleLoopDetector(maxRepeats, windowSize int) *LoopDetector {
	if maxRepeats < 1 {
		maxRepeats = 1
	}
	if windowSize < 1 {
		windowSize = 1
	}
	return &LoopDetector{
		Window:           windowSize,
		MinPatternLength: 1,
		MinRepeats:       maxRepeats,
	}
}

// Record appends a tool call by name and returns true if a loop is
// detected after appending it. This is the simple-string API used by
// builder.go's RepetitionThreshold wiring.
func (d *LoopDetector) Record(toolCall string) bool {
	if !d.Enabled() {
		return false
	}
	err := d.Observe(ToolCall{Name: toolCall}, "")
	return err != nil
}

// IsLooping reports whether the current recorded history contains a
// loop without appending a new call.
func (d *LoopDetector) IsLooping() bool {
	if d == nil {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.tripped
}
