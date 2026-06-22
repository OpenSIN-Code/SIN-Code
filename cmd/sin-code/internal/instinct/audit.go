// SPDX-License-Identifier: MIT
// Purpose: provenance / event history. Append-only JSONL so a curious
// operator can ask "why did the agent learn this?". Best-effort:
// audit failures never block the learning loop.
// Docs: audit.doc.md
package instinct

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// AuditEvent records why an instinct changed.
type AuditEvent struct {
	Time       time.Time `json:"time"`
	InstinctID string    `json:"instinct_id"`
	Kind       string    `json:"kind"` // created|reinforced|contradicted|evolved|promoted|pruned
	Confidence float64   `json:"confidence"`
	Detail     string    `json:"detail,omitempty"`
}

// timeNowUTC is the package clock, overridable in tests.
var timeNowUTC = func() time.Time { return time.Now().UTC() }

// Append writes one audit line (JSONL) under the store base. Best-effort.
func (s *Store) Append(ev AuditEvent) {
	if ev.Time.IsZero() {
		ev.Time = timeNowUTC()
	}
	path := filepath.Join(s.base, "audit.jsonl")
	if err := os.MkdirAll(s.base, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, filemode.Default())
	if err != nil {
		return
	}
	defer f.Close()
	b, _ := json.Marshal(ev)
	f.Write(append(b, '\n'))
}

// ReadAudit returns the most recent N audit events.
func (s *Store) ReadAudit(limit int) ([]AuditEvent, error) {
	path := filepath.Join(s.base, "audit.jsonl")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var all []AuditEvent
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var ev AuditEvent
		if json.Unmarshal(line, &ev) == nil {
			all = append(all, ev)
		}
	}
	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			out = append(out, b[start:i])
			start = i + 1
		}
	}
	if start < len(b) {
		out = append(out, b[start:])
	}
	return out
}
