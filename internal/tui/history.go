// SPDX-License-Identifier: MIT
// Purpose: In-memory ring buffer holding the most recent search terms typed
// in the TUI so up/down arrows recall prior queries.
// Docs: history.doc.md

package tui

// MaxHistoryItems caps the number of past search terms kept in memory.
// 20 ≈ a session's worth of distinct queries without bloating recall;
// chosen to match shell HISTSIZE conventions where 10–50 is typical.
const MaxHistoryItems = 20

// History is an in-memory ring buffer with a single cursor for up/down
// recall. It is intentionally not persisted — search history can contain
// PII (paths, secrets typed by accident) and the TUI is a developer tool,
// not a system shell.
//
// Cursor semantics:
//   - cursor == -1  →  no recall active; Next returns "".
//   - cursor in [0, len) → currently recalling items[cursor].
type History struct {
	items  []string
	cursor int
}

// NewHistory returns an empty history with no active cursor.
func NewHistory() *History {
	return &History{cursor: -1}
}

// Push appends a query and resets the cursor. Empty strings and exact
// duplicates of the most recent entry are ignored (matches readline/bash
// HISTCONTROL=ignoredups default). At capacity, the oldest entry is evicted.
func (h *History) Push(q string) {
	if q == "" {
		h.cursor = -1
		return
	}
	if n := len(h.items); n > 0 && h.items[n-1] == q {
		h.cursor = -1
		return
	}
	if len(h.items) >= MaxHistoryItems {
		// Drop the oldest. Re-allocates the backing array; acceptable at
		// 20 strings per second worst-case.
		h.items = h.items[1:]
	}
	h.items = append(h.items, q)
	h.cursor = -1
}

// Prev recalls the previous (older) term. Returns "" if history is empty.
// On the first call after Push/Reset, returns the newest entry.
func (h *History) Prev() string {
	if len(h.items) == 0 {
		return ""
	}
	if h.cursor == -1 {
		h.cursor = len(h.items) - 1
	} else if h.cursor > 0 {
		h.cursor--
	}
	return h.items[h.cursor]
}

// Next recalls the following (newer) term. Returns "" when stepping past
// the newest entry, which the UI should treat as "back to a blank input".
func (h *History) Next() string {
	if len(h.items) == 0 || h.cursor == -1 {
		return ""
	}
	if h.cursor < len(h.items)-1 {
		h.cursor++
		return h.items[h.cursor]
	}
	h.cursor = -1
	return ""
}

// Reset clears the cursor without dropping items. Call when the user
// dismisses the search bar so the next Prev starts from the newest entry.
func (h *History) Reset() {
	h.cursor = -1
}

// Len returns the number of stored items.
func (h *History) Len() int {
	return len(h.items)
}

// Items returns a snapshot of stored items, oldest → newest. The slice is
// a copy so callers cannot mutate the buffer.
func (h *History) Items() []string {
	out := make([]string, len(h.items))
	copy(out, h.items)
	return out
}
