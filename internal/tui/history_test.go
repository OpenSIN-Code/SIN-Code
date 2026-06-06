// Purpose: Tests for the in-memory search history ring buffer.
// Docs: history.doc.md

package tui

import (
	"fmt"
	"testing"
)

func TestHistoryEmpty(t *testing.T) {
	h := NewHistory()
	if got := h.Prev(); got != "" {
		t.Errorf("empty Prev = %q, want \"\"", got)
	}
	if got := h.Next(); got != "" {
		t.Errorf("empty Next = %q, want \"\"", got)
	}
	if h.Len() != 0 {
		t.Errorf("empty Len = %d, want 0", h.Len())
	}
}

func TestHistoryPushIgnoresEmpty(t *testing.T) {
	h := NewHistory()
	h.Push("")
	if h.Len() != 0 {
		t.Errorf("Push(\"\") added an item; Len = %d", h.Len())
	}
}

func TestHistoryPushAndPrev(t *testing.T) {
	h := NewHistory()
	h.Push("scout")
	h.Push("map")
	h.Push("audit")

	// First Prev returns the newest entry.
	if got := h.Prev(); got != "audit" {
		t.Errorf("Prev #1 = %q, want \"audit\"", got)
	}
	if got := h.Prev(); got != "map" {
		t.Errorf("Prev #2 = %q, want \"map\"", got)
	}
	if got := h.Prev(); got != "scout" {
		t.Errorf("Prev #3 = %q, want \"scout\"", got)
	}
	// Going past the oldest is a no-op.
	if got := h.Prev(); got != "scout" {
		t.Errorf("Prev #4 = %q, want \"scout\" (clamped)", got)
	}
}

func TestHistoryNextReturnsToBlank(t *testing.T) {
	h := NewHistory()
	h.Push("a")
	h.Push("b")

	if got := h.Prev(); got != "b" {
		t.Fatalf("Prev = %q, want \"b\"", got)
	}
	if got := h.Prev(); got != "a" {
		t.Fatalf("Prev = %q, want \"a\"", got)
	}
	if got := h.Next(); got != "b" {
		t.Errorf("Next = %q, want \"b\"", got)
	}
	// Stepping past the newest entry returns blank (cursor goes inactive).
	if got := h.Next(); got != "" {
		t.Errorf("Next at newest+1 = %q, want \"\"", got)
	}
	// Cursor is now inactive — another Next returns blank.
	if got := h.Next(); got != "" {
		t.Errorf("Next after blank = %q, want \"\"", got)
	}
}

func TestHistoryDuplicateOfLastIsIgnored(t *testing.T) {
	h := NewHistory()
	h.Push("scout")
	h.Push("scout")
	h.Push("scout")
	if h.Len() != 1 {
		t.Errorf("Len after 3x same Push = %d, want 1", h.Len())
	}
}

func TestHistoryNonAdjacentDuplicateKept(t *testing.T) {
	h := NewHistory()
	h.Push("scout")
	h.Push("map")
	h.Push("scout")
	if h.Len() != 3 {
		t.Errorf("Len = %d, want 3 (non-adjacent dup kept)", h.Len())
	}
}

func TestHistoryRingBufferEviction(t *testing.T) {
	h := NewHistory()
	// Push MaxHistoryItems + 5 distinct items.
	for i := 0; i < MaxHistoryItems+5; i++ {
		h.Push(fmt.Sprintf("q%d", i))
	}
	if got := h.Len(); got != MaxHistoryItems {
		t.Errorf("Len = %d, want %d", got, MaxHistoryItems)
	}
	// The oldest 5 items should have been evicted; first surviving
	// item is "q5".
	items := h.Items()
	if items[0] != "q5" {
		t.Errorf("oldest after eviction = %q, want \"q5\"", items[0])
	}
	if items[len(items)-1] != fmt.Sprintf("q%d", MaxHistoryItems+4) {
		t.Errorf("newest = %q, want q%d", items[len(items)-1], MaxHistoryItems+4)
	}
}

func TestHistoryPushResetsCursor(t *testing.T) {
	h := NewHistory()
	h.Push("a")
	h.Push("b")
	h.Prev() // cursor on "b"
	h.Prev() // cursor on "a"
	h.Push("c")
	// After Push, the cursor must be reset so Prev returns the newest.
	if got := h.Prev(); got != "c" {
		t.Errorf("Prev after Push = %q, want \"c\"", got)
	}
}

func TestHistoryResetClearsCursorOnly(t *testing.T) {
	h := NewHistory()
	h.Push("a")
	h.Push("b")
	h.Prev() // on "b"
	h.Reset()
	if h.Len() != 2 {
		t.Errorf("Reset dropped items; Len = %d", h.Len())
	}
	if got := h.Prev(); got != "b" {
		t.Errorf("Prev after Reset = %q, want \"b\"", got)
	}
}

func TestHistoryItemsReturnsCopy(t *testing.T) {
	h := NewHistory()
	h.Push("a")
	items := h.Items()
	items[0] = "mutated"
	if h.Items()[0] != "a" {
		t.Errorf("Items() returned a live slice; internal state mutated")
	}
}
