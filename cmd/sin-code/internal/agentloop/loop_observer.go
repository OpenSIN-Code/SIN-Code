// SPDX-License-Identifier: MIT
// Purpose: LoopDetector observes consecutive tool calls and refuses
// dispatch when the worker falls into a repeated-sequence cycle
// (issue #377, AGENTS.md §3 M4). Fail-closed: a detected loop
// blocks the next identical call AND emits the loop.detected hook
// event so downstream telemetry / UI / audit pipelines can react.
//
// Algorithm: rolling fingerprint of (tool name, message hash, args
// signature). The args signature is a sha256 over the canonical JSON
// of Args (encoding/json already sorts map keys alphabetically, so
// two calls with the same fields in any order are treated as
// identical). After each observation the detector scans the rolling
// history for the LONGEST pattern p >= MinPatternLength whose last p
// entries equal the immediately preceding p entries. If the
// resulting repeat count (len(hist)/p) meets MinRepeats the call is
// refused and ErrLoopDetected is returned.
//
// All public methods are safe for concurrent use (mandate M7).
package agentloop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

const (
	// DefaultObserverWindow is the rolling-history size used when
	// the operator does not specify one.
	DefaultObserverWindow = 20
	// DefaultObserverMinPatternLength is the minimum suffix length
	// to qualify as a loop candidate.
	DefaultObserverMinPatternLength = 3
	// DefaultObserverMinRepeats is the minimum repeat count required
	// to refuse dispatch.
	DefaultObserverMinRepeats = 2
)

// ErrLoopDetected is returned by LoopDetector.Observe when the
// observed tool call would complete (or repeat) a flagged sequence.
// The dispatch site MUST NOT call the local tool when this error is
// non-nil (mandate M4 — destructive actions stay permission-gated).
var ErrLoopDetected = errors.New("agentloop: tool-call loop detected; refusing dispatch")

// LoopTrip carries the metadata captured at the moment of detection.
// Loop.Run reads it via LoopDetector.LastTrip to populate the
// hooks.LoopDetected payload.
type LoopTrip struct {
	ToolName   string
	Length     int
	Repeats    int
	Key        string
	HistoryLen int
}

// LoopDetector observes consecutive tool calls and detects repeated
// sequences. Constructed with NewLoopDetector; nil receivers and
// zero-Window detectors are no-ops.
type LoopDetector struct {
	Window           int
	MinPatternLength int
	MinRepeats       int

	mu      sync.Mutex
	history []loopRecord
	// tripped flips true after the first detection. While tripped the
	// detector returns ErrLoopDetected for any call whose key matches
	// the captured pattern's first element, so a stuck worker cannot
	// incrementally chip its way out of the cycle.
	tripped bool
	// pattern is one captured cycle from the detection instant — the
	// keys the worker is hammering on. Used to refuse the next
	// identical call without re-scanning the entire history.
	pattern []string
	// trip captures the metadata from the first detection so the
	// loop can fire the hook payload even after subsequent observes
	// mutate .history.
	trip *LoopTrip
}

// loopRecord is the per-call record kept in the rolling history.
type loopRecord struct {
	Key         string
	Name        string
	ArgsSig     string
	MessageHash string
}

// NewLoopDetector constructs a detector. window <= 0 disables the
// detector (Observe becomes a no-op). MinPatternLength and
// MinRepeats fall back to documented defaults when non-positive.
func NewLoopDetector(window, minPatternLength, minRepeats int) *LoopDetector {
	if minPatternLength < 1 {
		minPatternLength = DefaultObserverMinPatternLength
	}
	if minRepeats < 1 {
		minRepeats = DefaultObserverMinRepeats
	}
	if window < 0 {
		window = 0
	}
	return &LoopDetector{
		Window:           window,
		MinPatternLength: minPatternLength,
		MinRepeats:       minRepeats,
	}
}

// Enabled reports whether the detector is active. nil receivers and
// zero-Window detectors return false so callers can guard trivially.
func (d *LoopDetector) Enabled() bool {
	return d != nil && d.Window > 0
}

// LastTrip returns a copy of the most recent trip metadata, or nil
// when no loop has been detected yet on this detector. The returned
// value is a copy and is safe to read without the lock.
func (d *LoopDetector) LastTrip() *LoopTrip {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.trip == nil {
		return nil
	}
	cp := *d.trip
	return &cp
}

// Observe records a tool call and returns ErrLoopDetected when the
// new history (after the call) contains a qualifying loop OR the
// detector was already tripped and the call extends the captured
// pattern. messageHash may be empty — the detector then derives a
// stable per-call digest from (Name, Args).
func (d *LoopDetector) Observe(tc ToolCall, messageHash string) error {
	if !d.Enabled() {
		return nil
	}
	rec := loopRecord{
		Key:         loopKey(tc, messageHash),
		Name:        tc.Name,
		ArgsSig:     loopArgsSig(tc.Args),
		MessageHash: messageHash,
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	d.history = append(d.history, rec)
	if len(d.history) > d.Window {
		// FIFO-evict the oldest entries; copy so the underlying
		// slice does not pin them.
		d.history = append([]loopRecord(nil), d.history[len(d.history)-d.Window:]...)
	}

	// Once tripped we keep refusing identical calls even when the
	// rolling window no longer contains enough history to recompute
	// the repeat — the worker is still on the same key.
	if d.tripped {
		if matchesPatternStart(d.history, d.pattern) {
			return ErrLoopDetected
		}
	}

	return d.evaluateLocked()
}

// evaluateLocked re-scans the history for the longest qualifying
// repetition and trips the detector on the first match. Caller holds
// d.mu.
func (d *LoopDetector) evaluateLocked() error {
	n := len(d.history)
	if n < d.MinPatternLength*d.MinRepeats {
		return nil
	}
	p := longestMatchingPrefix(d.history)
	if p < d.MinPatternLength {
		return nil
	}
	repeats := n / p
	if repeats < d.MinRepeats {
		return nil
	}
	pattern := make([]string, p)
	for i := 0; i < p; i++ {
		pattern[i] = d.history[n-p+i].Key
	}
	d.tripped = true
	d.pattern = pattern
	name := d.history[n-1].Name
	if name == "" {
		name = "unknown"
	}
	d.trip = &LoopTrip{
		ToolName:   name,
		Length:     p,
		Repeats:    repeats,
		Key:        d.history[n-1].Key,
		HistoryLen: n,
	}
	return ErrLoopDetected
}

// Reset clears the detector state so a new run / session starts
// fresh. Safe on nil. The detector stays enabled; only the history is
// dropped.
func (d *LoopDetector) Reset() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.history = nil
	d.tripped = false
	d.pattern = nil
	d.trip = nil
}

// longestMatchingPrefix returns the longest p >= 1 such that the
// last p entries of hist equal the immediately preceding p entries.
// Returns 0 when no such prefix exists.
func longestMatchingPrefix(hist []loopRecord) int {
	n := len(hist)
	upper := n / 2
	for p := upper; p >= 1; p-- {
		if equalAt(hist, n-p, n-2*p, p) {
			return p
		}
	}
	return 0
}

func equalAt(hist []loopRecord, a, b, length int) bool {
	for i := 0; i < length; i++ {
		if hist[a+i].Key != hist[b+i].Key {
			return false
		}
	}
	return true
}

// matchesPatternStart reports whether the last len(pattern) entries
// of hist match pattern exactly. Used to refuse the next identical
// call after the detector has tripped.
func matchesPatternStart(hist []loopRecord, pattern []string) bool {
	if len(hist) < len(pattern) {
		return false
	}
	start := len(hist) - len(pattern)
	for i := 0; i < len(pattern); i++ {
		if hist[start+i].Key != pattern[i] {
			return false
		}
	}
	return true
}

// loopArgsSig computes a stable signature for the args map. encoding/
// json sorts map keys alphabetically, so a canonical JSON dump +
// sha256 is enough to treat reordering-but-equality as identical.
func loopArgsSig(args map[string]any) string {
	b, _ := json.Marshal(args)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// loopKey assembles the full detection signature for a tool call.
// Two calls are equal iff (Name, ArgsSig, MessageHash) match.
func loopKey(tc ToolCall, messageHash string) string {
	argsSum := loopArgsSig(tc.Args)
	if messageHash == "" {
		h := sha256.New()
		h.Write([]byte(tc.Name))
		h.Write([]byte{0})
		h.Write([]byte(argsSum))
		messageHash = hex.EncodeToString(h.Sum(nil))
	}
	h := sha256.New()
	h.Write([]byte(tc.Name))
	h.Write([]byte{0})
	h.Write([]byte(argsSum))
	h.Write([]byte{0})
	h.Write([]byte(messageHash))
	return hex.EncodeToString(h.Sum(nil))
}

// assistantMessageHash returns a stable digest of the assistant
// message that produced a tool call. Empty messages (zero-value
// session.Message) hash to "" so the detector falls back to its own
// (name, args) digest for unit tests that don't wire the raw.
func assistantMessageHash(m session.Message) string {
	if m.Role == "" && m.Content == "" && m.ToolCallID == "" && len(m.ToolCalls) == 0 {
		return ""
	}
	// session.Message is JSON-tagged so a canonical marshal is
	// already deterministic for the role / content / tool_call_id
	// axes; raw ToolCalls bytes are appended verbatim.
	canon := struct {
		Role       string          `json:"role"`
		Content    string          `json:"content"`
		ToolCallID string          `json:"tool_call_id"`
		ToolCalls  json.RawMessage `json:"tool_calls"`
	}{m.Role, m.Content, m.ToolCallID, m.ToolCalls}
	b, _ := json.Marshal(canon)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
