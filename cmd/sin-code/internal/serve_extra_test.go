// SPDX-License-Identifier: MIT

package internal

import (
	"context"
	"strings"
	"testing"
)

// TestHandleTodoShow_MissingID verifies that handleTodoShow requires
// an id argument. (st-cov1)
func TestHandleTodoShow_MissingID(t *testing.T) {
	_, err := handleTodoShow(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing id")
	}
	if !strings.Contains(err.Error(), "id is required") {
		t.Errorf("expected 'id is required' in error, got %v", err)
	}
}

// TestHandleTodoComplete_MissingID verifies that handleTodoComplete
// requires an id argument. (st-cov1)
func TestHandleTodoComplete_MissingID(t *testing.T) {
	_, err := handleTodoComplete(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing id")
	}
}

// TestHandleTodoClaim_MissingID verifies that handleTodoClaim
// requires an id argument. (st-cov1)
func TestHandleTodoClaim_MissingID(t *testing.T) {
	_, err := handleTodoClaim(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing id")
	}
}

// TestHandleTodoAdd_MissingTitle verifies that handleTodoAdd requires
// a title. (st-cov1)
func TestHandleTodoAdd_MissingTitle(t *testing.T) {
	_, err := handleTodoAdd(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing title")
	}
	if !strings.Contains(err.Error(), "title is required") {
		t.Errorf("expected 'title is required' in error, got %v", err)
	}
}

// TestHandleMemoryAdd_MissingContent verifies that
// handleMemoryAdd requires a content argument. (st-cov1)
func TestHandleMemoryAdd_MissingContent(t *testing.T) {
	_, err := handleMemoryAdd(context.Background(), map[string]any{})
	if err == nil {
		t.Error("expected error for missing content")
	}
}
