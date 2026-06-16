// SPDX-License-Identifier: MIT
// Purpose: Event envelope and Artifact types for the CDP ground-truth log.
// Every captured CDP event is serialised as one JSON line (JSONL) so the
// on-disk record is streamable and self-describing. The Artifact type
// references out-of-band blobs (response bodies, screenshots, DOM dumps)
// that are written as separate files alongside the JSONL log.
// This package is the low-level capture layer; diagnostics and the
// deterministic findings engine live in the same package.
package cdp

import "encoding/json"

// Event is one line in the JSONL ground-truth log.
// The JSONL file is the single source of truth; every report or finding
// is a read-only view derived from it.
type Event struct {
	// Seq is a global monotonically increasing sequence number assigned
	// at emit time under a mutex, so JSONL order == causal order.
	Seq uint64 `json:"seq"`

	// WallTime is an RFC3339Nano wall-clock timestamp — useful for
	// forensic cross-referencing with external logs.
	WallTime string `json:"wall_time"`

	// MonoNanos is nanoseconds elapsed since the Recorder was started.
	// Use this for all duration / latency calculations because wall-clock
	// can jump on NTP adjustments.
	MonoNanos int64 `json:"mono_nanos"`

	// SessionID is the CDP session identifier. Non-empty only for
	// events from auto-attached child targets (OOPIFs, workers).
	SessionID string `json:"session_id,omitempty"`

	// Domain is the CDP protocol domain, e.g. "Network", "Audits".
	Domain string `json:"domain"`

	// Method is the CDP event name within the domain, e.g. "responseReceived".
	Method string `json:"method"`

	// StepID correlates this event to an agent action / navigation step.
	// Set by calling Recorder.SetStep before the action.
	StepID string `json:"step_id,omitempty"`

	// Params is the raw CDP event payload; kept as json.RawMessage so
	// callers can unmarshal into the concrete cdproto type when needed
	// without paying for a double-decode on every event.
	Params json.RawMessage `json:"params"`
}

// Artifact references an out-of-band blob written alongside the JSONL log.
// Examples: response bodies, screenshots, DOM snapshots, accessibility trees.
type Artifact struct {
	// Seq links the artifact to the Event that triggered its capture.
	Seq uint64 `json:"seq"`

	// Kind identifies the artifact type: "response_body", "screenshot",
	// "dom_snapshot", "a11y_tree", "computed_styles", "box_model".
	Kind string `json:"kind"`

	// RequestID is set for response-body artifacts.
	RequestID string `json:"request_id,omitempty"`

	// MimeType is the Content-Type of the captured body (if known).
	MimeType string `json:"mime_type,omitempty"`

	// Path is the file path of the artifact relative to the output directory.
	Path string `json:"path"`

	// Bytes is the size of the artifact after any truncation.
	Bytes int `json:"bytes"`

	// Truncated is true when the body was capped by MaxBodyBytes.
	Truncated bool `json:"truncated,omitempty"`
}
