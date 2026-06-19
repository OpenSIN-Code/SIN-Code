// SPDX-License-Identifier: MIT
// Purpose: Typed inter-session messaging on top of the file-locked
// Mailbox (issue #316). Provides 5 message kinds — TaskHandoff,
// Query, Response, Conflict, Status — plus a MessageBus that wraps
// the Mailbox with session-targeted Send/Recv/Reply/Broadcast and
// conflict resolution.
//
// M7 invariant: the MessageBus delegates all disk I/O to the
// Mailbox, which is already protected by sync.Mutex + file locks.
// The in-memory conflict index is guarded by its own RWMutex.
package agentteams

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// MessageType classifies a Message into one of five typed kinds.
type MessageType int

const (
	MsgHandoff  MessageType = iota + 1 // task delegation between sessions
	MsgQuery                           // request for information
	MsgResponse                        // reply to a Query
	MsgConflict                        // resource conflict between sessions
	MsgStatus                          // status update / progress report
)

// String returns the human-readable name of the message type.
func (t MessageType) String() string {
	switch t {
	case MsgHandoff:
		return "TaskHandoff"
	case MsgQuery:
		return "Query"
	case MsgResponse:
		return "Response"
	case MsgConflict:
		return "Conflict"
	case MsgStatus:
		return "Status"
	default:
		return fmt.Sprintf("Unknown(%d)", int(t))
	}
}

// Conflict represents a resource conflict between two or more sessions.
// Stored as a MsgConflict message whose Body is the JSON encoding of
// this struct.
type Conflict struct {
	ID         string   `json:"id"`
	Sessions   []string `json:"sessions"`
	Resource   string   `json:"resource"`
	Resolution string   `json:"resolution,omitempty"`
}

// MessageBus provides typed messaging between agent sessions on top of
// a file-locked Mailbox. The bus is safe for concurrent use (M7).
type MessageBus struct {
	mailbox   *Mailbox
	mu        sync.RWMutex // guards conflicts index
	conflicts map[string]Conflict
}

// NewMessageBus creates a MessageBus backed by the given Mailbox.
func NewMessageBus(mailbox *Mailbox) *MessageBus {
	return &MessageBus{
		mailbox:   mailbox,
		conflicts: make(map[string]Conflict),
	}
}

// Send writes a message to the recipient's mailbox. If msg.Type is
// MsgConflict, the conflict is also indexed for ResolveConflict.
func (b *MessageBus) Send(msg Message) error {
	if msg.ID == "" {
		return errors.New("agentteams: empty Message.ID")
	}
	if msg.From == "" {
		return errors.New("agentteams: empty Message.From")
	}
	if msg.To == "" {
		return errors.New("agentteams: empty Message.To")
	}
	if msg.SentAt.IsZero() {
		msg.SentAt = time.Now().UTC()
	}
	_, _, err := b.mailbox.Send(msg)
	if err != nil {
		return fmt.Errorf("agentteams: bus send: %w", err)
	}
	if msg.Type == MsgConflict {
		b.indexConflict(msg)
	}
	return nil
}

// Recv returns all messages addressed to the given session (To ==
// sessionID or To == "broadcast"), in arrival order.
func (b *MessageBus) Recv(sessionID string) ([]Message, error) {
	if sessionID == "" {
		return nil, errors.New("agentteams: empty sessionID")
	}
	all, err := b.mailbox.Receive()
	if err != nil {
		return nil, fmt.Errorf("agentteams: bus recv: %w", err)
	}
	var out []Message
	for _, msg := range all {
		if msg.To == sessionID || msg.To == "broadcast" {
			out = append(out, msg)
		}
	}
	return out, nil
}

// Reply sends a message that is a reply to the message identified by
// originalID. The ReplyTo field is set to originalID.
func (b *MessageBus) Reply(originalID string, msg Message) error {
	if originalID == "" {
		return errors.New("agentteams: empty originalID for reply")
	}
	msg.ReplyTo = originalID
	if msg.Type == 0 {
		msg.Type = MsgResponse
	}
	return b.Send(msg)
}

// Broadcast sends a message to all sessions by setting To to
// "broadcast".
func (b *MessageBus) Broadcast(msg Message) error {
	msg.To = "broadcast"
	return b.Send(msg)
}

// ResolveConflict marks the conflict identified by conflictID as
// resolved with the given resolution string. The conflict message in
// the mailbox is updated and marked resolved.
func (b *MessageBus) ResolveConflict(conflictID string, resolution string) error {
	if conflictID == "" {
		return errors.New("agentteams: empty conflictID")
	}
	b.mu.Lock()
	c, ok := b.conflicts[conflictID]
	if !ok {
		b.mu.Unlock()
		return fmt.Errorf("agentteams: no such conflict %q", conflictID)
	}
	c.Resolution = resolution
	b.conflicts[conflictID] = c
	b.mu.Unlock()

	// Update the conflict message body and mark resolved.
	body, err := json.Marshal(c)
	if err != nil {
		return fmt.Errorf("agentteams: marshal conflict: %w", err)
	}
	all, err := b.mailbox.Receive()
	if err != nil {
		return err
	}
	for i := range all {
		if all[i].ID == conflictID {
			all[i].Body = string(body)
			all[i].Resolved = true
			return writeAllMessages(b.mailbox.path, all)
		}
	}
	return fmt.Errorf("agentteams: conflict message %q not found in mailbox", conflictID)
}

// GetConflict returns the conflict with the given ID, if tracked.
func (b *MessageBus) GetConflict(conflictID string) (Conflict, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	c, ok := b.conflicts[conflictID]
	return c, ok
}

// indexConflict parses a MsgConflict message body and adds it to the
// in-memory index.
func (b *MessageBus) indexConflict(msg Message) {
	var c Conflict
	if err := json.Unmarshal([]byte(msg.Body), &c); err != nil {
		return
	}
	if c.ID == "" {
		c.ID = msg.ID
	}
	b.mu.Lock()
	b.conflicts[c.ID] = c
	b.mu.Unlock()
}

// LoadConflicts scans the mailbox for existing MsgConflict messages
// and indexes them. Call this after creating a MessageBus from an
// existing mailbox to restore the conflict index.
func (b *MessageBus) LoadConflicts() error {
	all, err := b.mailbox.Receive()
	if err != nil {
		return err
	}
	for _, msg := range all {
		if msg.Type == MsgConflict {
			b.indexConflict(msg)
		}
	}
	return nil
}
