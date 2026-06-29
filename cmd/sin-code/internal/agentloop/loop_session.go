// SPDX-License-Identifier: MIT
// sin-debt: shrink, upgrade: when a second <type>-related function is needed, merge into a shared file
package agentloop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/session"
)

// saveHistoryHook is a test seam for injecting a mock around session
// SaveHistory calls. When nil, the real Session.SaveHistory is used.
// This exists so tests can force a save-history error without mocking
// the SQLite store.
var saveHistoryHook func(sess *session.Session, msgs []session.Message) error

func (l *Loop) saveHistory(ctx context.Context, sess *session.Session, msgs []session.Message) error {
	if saveHistoryHook != nil {
		return saveHistoryHook(sess, msgs)
	}
	return sess.SaveHistory(msgs)
}

// sessionIDHash returns the first 12 hex chars of sha256(sessionID),
// the workspace-isolation convention shared with internal/session and
// internal/lessons (issue #265).
func sessionIDHash(sessionID string) string {
	if sessionID == "" {
		return "unknown"
	}
	sum := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(sum[:])[:12]
}
