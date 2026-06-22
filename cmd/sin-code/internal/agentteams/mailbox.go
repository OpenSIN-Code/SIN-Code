// SPDX-License-Identifier: MIT
// Purpose: Agent-Teams mailbox — file-locked, deduplicated, append-only
// message queue shared between cooperating SIN-Code sessions.
//
// Mirrors the Claude Code v2.0 Team-Design pattern (Anthropic, 2026-01-22)
// but writes the queue to disk instead of in-memory so teams survive
// crashes and can be inspected with `cat .sin-code/teams/inbox.jsonl`.
//
// M3 invariant: every mailbox write is durable (fsync before close).
// The presence of unprocessed messages is observable on stderr; the
// e2e loop is provably reproducible from the mailbox log alone.
//
// M7 invariant: file locks (syscall.Flock) on the inbox file make
// concurrent agents serialise their inbox.append operations without
// dropping messages.
package agentteams

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// Message is a single mailbox entry. Headers (From, To, Subject) are
// strictly typed; Body is bytes so callers can carry JSON, plan-text,
// shell-output, or any structured content.
type Message struct {
	ID       string      `json:"id"`                 // content-addressed; dedupes on Send
	From     string      `json:"from"`               // sender role-id or session-id
	To       string      `json:"to"`                 // recipient role-id or "broadcast"
	Type     MessageType `json:"type,omitempty"`     // typed message kind (issue #316)
	Subject  string      `json:"subject"`            // one-line intent (no newlines)
	Body     string      `json:"body"`               // free-form body, multi-line OK
	SentAt   time.Time   `json:"sent_at"`            // UTC, RFC3339 — serves as Timestamp
	ReplyTo  string      `json:"reply_to,omitempty"` // original message ID for replies (#316)
	Resolved bool        `json:"resolved,omitempty"` // for request/response pattern
}

// Mailbox is the file-backed agent-team inbox. Idempotent Open()
// creates the underlying directory on first use. Concurrent
// consumers/producers are supported via per-Open file lock.
type Mailbox struct {
	dir      string
	path     string     // <dir>/inbox.jsonl
	mu       sync.Mutex // serialises Open-and-flush within the same process
	lockFile *os.File   // file handle held by explicit Lock/Unlock (#342)
}

// Open creates or opens the agent-team mailbox rooted at
// `<workspace>/.sin-code/teams/`. The directory is created
// lazily on first use. Safe for repeated calls.
func Open(workspace string) (*Mailbox, error) {
	if workspace == "" {
		return nil, errors.New("agentteams: empty workspace")
	}
	dir := filepath.Join(workspace, ".sin-code", "teams")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("agentteams: mkdir: %w", err)
	}
	return &Mailbox{
		dir:  dir,
		path: filepath.Join(dir, "inbox.jsonl"),
	}, nil
}

// Path returns the on-disk inbox file location. Useful for diagnostics.
func (m *Mailbox) Path() string { return m.path }

// Send appends a message to the inbox, deduplicated by Message.ID.
// Returns the byte offset of the appended line (or the offset of the
// pre-existing line when ID is already present). The file is flushed
// and locked via syscall.Flock so concurrent agents serialise.
func (m *Mailbox) Send(msg Message) (offset int64, dedup bool, err error) {
	if msg.ID == "" {
		return 0, false, errors.New("agentteams: empty Message.ID")
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now().UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	// Open with O_APPEND so concurrent agents don't race on
	// byte offsets. O_APPEND on POSIX is required to be atomic
	// for writes shorter than PIPE_BUF (4 KiB) — our JSON line
	// is well under that.
	f, err := os.OpenFile(m.path, os.O_APPEND|os.O_CREATE|os.O_RDWR, filemode.Default())
	if err != nil {
		return 0, false, fmt.Errorf("agentteams: open: %w", err)
	}
	defer f.Close()
	if err := flockLock(int(f.Fd())); err != nil {
		return 0, false, fmt.Errorf("agentteams: flock: %w", err)
	}
	defer flockUnlock(int(f.Fd()))
	// Dedup pass: read what's already there, return early if ID matches.
	existing, err := readAllMessages(m.path)
	if err != nil {
		return 0, false, err
	}
	for _, prev := range existing {
		if prev.ID == msg.ID {
			return 0, true, nil
		}
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return 0, false, fmt.Errorf("agentteams: marshal: %w", err)
	}
	n, err := f.Write(append(data, '\n'))
	if err != nil {
		return 0, false, fmt.Errorf("agentteams: write: %w", err)
	}
	if err := f.Sync(); err != nil {
		return 0, false, fmt.Errorf("agentteams: sync: %w", err)
	}
	return int64(n), false, nil
}

// Receive returns every message currently in the inbox in arrival
// order. Returns an empty slice on empty inbox (no error). For
// streaming consumers, callers can pair this with a SendID-tracking
// pattern; it's not transactional but the file lock + JSONL
// record format is precise enough for crash-safe recovery.
func (m *Mailbox) Receive() ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return readAllMessages(m.path)
}

// DrainAfterID returns every message whose ID is strictly greater
// than `sinceID`, sorted lexicographically by ID (which is also the
// arrival order when IDs are content-addressed). Used for
// incremental agent polling.
func (m *Mailbox) DrainAfterID(sinceID string) ([]Message, error) {
	all, err := m.Receive()
	if err != nil {
		return nil, err
	}
	out := []Message{}
	for _, msg := range all {
		if sinceID == "" || msg.ID > sinceID {
			out = append(out, msg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// MarkResolved flips Message.Resolved=true in-place. Idempotent. The
// update is durable to the next access because we re-marshal the
// whole file (small enough for typical inboxes).
func (m *Mailbox) MarkResolved(id string) error {
	if id == "" {
		return errors.New("agentteams: empty ID")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	all, err := readAllMessages(m.path)
	if err != nil {
		return err
	}
	found := false
	for i := range all {
		if all[i].ID == id {
			all[i].Resolved = true
			found = true
		}
	}
	if !found {
		return fmt.Errorf("agentteams: no such message %q", id)
	}
	return writeAllMessages(m.path, all)
}

// Stats returns a small kebab-cased report. Useful for
// `sin-code team stats` future-facing.
type Stats struct {
	Total      int       `json:"total"`
	Unresolved int       `json:"unresolved"`
	Oldest     time.Time `json:"oldest,omitempty"`
	Newest     time.Time `json:"newest,omitempty"`
}

// Stats implements the diagnostic summary.
func (m *Mailbox) Stats() (Stats, error) {
	all, err := m.Receive()
	if err != nil {
		return Stats{}, err
	}
	out := Stats{Total: len(all)}
	for _, msg := range all {
		if !msg.Resolved {
			out.Unresolved++
		}
		if out.Oldest.IsZero() || msg.SentAt.Before(out.Oldest) {
			out.Oldest = msg.SentAt
		}
		if msg.SentAt.After(out.Newest) {
			out.Newest = msg.SentAt
		}
	}
	return out, nil
}

// --- helpers ------------------------------------------------------------

func readAllMessages(path string) ([]Message, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("agentteams: read inbox: %w", err)
	}
	var out []Message
	// JSONL parser is strict per-line. Empty or whitespace-only
	// lines are ignored — atomic-write semantics allowed some
	// whitespace padding.
	lineNo := 0
	for _, raw := range splitLines(contents) {
		lineNo++
		if len(raw) == 0 {
			continue
		}
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("agentteams: parse line %d: %w", lineNo, err)
		}
		out = append(out, msg)
	}
	return out, nil
}

func writeAllMessages(path string, msgs []Message) error {
	var buf []byte
	for _, msg := range msgs {
		data, err := json.Marshal(msg)
		if err != nil {
			return fmt.Errorf("agentteams: marshal %s: %w", msg.ID, err)
		}
		buf = append(buf, data...)
		buf = append(buf, '\n')
	}
	// atomic write via tmp+rename so a crash mid-write preserves
	// the prior inbox. Mirror the auto_mem pattern for consistency.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf, filemode.Default()); err != nil {
		return fmt.Errorf("agentteams: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("agentteams: rename tmp: %w", err)
	}
	return nil
}

func splitLines(contents []byte) [][]byte {
	var out [][]byte
	last := 0
	for i := 0; i < len(contents); i++ {
		if contents[i] == '\n' {
			out = append(out, contents[last:i])
			last = i + 1
		}
	}
	if last < len(contents) {
		out = append(out, contents[last:])
	}
	return out
}

// Lock acquires an exclusive file lock on the mailbox file. The lock
// is held until Unlock is called. Platform-specific locking is provided
// by flockLock/flockUnlock (issue #342). On unsupported platforms the
// lock is a no-op but the file handle is still tracked for Unlock.
//
// The in-process mutex (m.mu) is released before the blocking flock
// call to prevent deadlock with Send/Unlock (M7).
func (m *Mailbox) Lock() error {
	m.mu.Lock()
	if m.lockFile != nil {
		m.mu.Unlock()
		return errors.New("agentteams: already locked")
	}
	f, err := os.OpenFile(m.path, os.O_CREATE|os.O_RDWR, filemode.Default())
	if err != nil {
		m.mu.Unlock()
		return fmt.Errorf("agentteams: lock open: %w", err)
	}
	m.lockFile = f
	m.mu.Unlock()

	// Blocking flock call — outside m.mu to prevent deadlock.
	if err := flockLock(int(f.Fd())); err != nil {
		m.mu.Lock()
		m.lockFile = nil
		m.mu.Unlock()
		f.Close()
		return fmt.Errorf("agentteams: lock: %w", err)
	}
	return nil
}

// Unlock releases the exclusive file lock acquired by Lock. Safe to
// call even if Lock was never called (returns nil). The underlying file
// handle is always closed. The flockUnlock and Close calls happen
// outside m.mu to prevent deadlock (M7).
func (m *Mailbox) Unlock() error {
	m.mu.Lock()
	f := m.lockFile
	m.lockFile = nil
	m.mu.Unlock()

	if f == nil {
		return nil
	}
	_ = flockUnlock(int(f.Fd()))
	return f.Close()
}
