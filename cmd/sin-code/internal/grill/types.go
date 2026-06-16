// SPDX-License-Identifier: MIT
// Purpose: the data model for a grilling session. Mirrors the
// external SIN-Code-Grill-Me-Skill concepts (Decision, Branch,
// Session) so the SKILL.md catalog of anti-patterns can be ported
// 1:1 in v1.
package grill

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Session is a single grilling interview. The ID is a hex-encoded
// SHA-256 of the topic + a start-time salt, so two operators
// running "grill start <topic>" on the same day get different
// sessions unless they collide on the 64-bit prefix.
type Session struct {
	ID        string    `json:"id"`
	Topic     string    `json:"topic"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Decisions is the tree of resolved + open questions. The
	// first decision is the root; further answers may push to
	// the tree via ParentID.
	Decisions []Decision `json:"decisions"`
	// OpenQuestions is a derived list (rebuilt on every render).
	// Cached in the JSON for fast CLI startup.
	OpenQuestions int `json:"open_questions"`
}

// Decision is one Q&A in the interview. Status reflects where the
// operator is in the resolution flow:
//
//   - "open"      — asked, no answer yet
//   - "answered"  — operator answered, follow-up may follow
//   - "resolved"  — operator is satisfied, the decision is final
//   - "deferred"  — operator wants to come back later
type Decision struct {
	ID        string    `json:"id"`
	ParentID  string    `json:"parent_id,omitempty"`
	Question  string    `json:"question"`
	Answer    string    `json:"answer,omitempty"`
	Status    string    `json:"status"`
	AskedAt   time.Time `json:"asked_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

// Synthesize is the output of `grill synthesize`. It summarizes
// the resolved decisions + open questions + assumptions surfaced
// during the interview. The CLI emits this in both human-readable
// text and JSON.
type Synthesize struct {
	Resolved []string `json:"resolved"`
	Open     []string `json:"open"`
	Assumptions []string `json:"assumptions"`
}

// newSessionID returns a hex-encoded SHA-256 of topic+time, which
// is unique enough for operator-side session management.
func newSessionID(topic string, now time.Time) string {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write([]byte(now.Format(time.RFC3339Nano)))
	return "grill-" + hex.EncodeToString(h.Sum(nil)[:8])
}
