// SPDX-License-Identifier: MIT
// Purpose: native Honcho peer-model integration (issue #356).
// Uses the memory Store directly — no external MCP server required.
// Graceful degradation when the store is nil.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	tagPreference = "preference"
	tagPeerModel  = "peer-model"
)

// HonchoIntegration provides native behavioral-memory operations
// using the bbolt-backed memory Store. It does not require an
// external Honcho MCP server — preferences and peer models are
// stored as tagged memories.
type HonchoIntegration struct {
	store *Store
}

// NewHonchoIntegration creates a native Honcho integration backed
// by the given Store. A nil store is valid: all methods degrade
// gracefully (return empty results, no error).
func NewHonchoIntegration(store *Store) *HonchoIntegration {
	return &HonchoIntegration{store: store}
}

// GetUserPreferences extracts user preferences from memory.
// Preferences are memories tagged "preference". Returns their
// insight strings as a slice. Returns an empty slice (no error)
// when the store is nil or no preferences exist.
func (h *HonchoIntegration) GetUserPreferences(ctx context.Context) ([]string, error) {
	if h == nil || h.store == nil {
		return nil, nil
	}
	mems, err := h.store.List(ListFilter{Tag: tagPreference, Limit: 1000})
	if err != nil {
		return nil, fmt.Errorf("honcho: list preferences: %w", err)
	}
	out := make([]string, 0, len(mems))
	for _, m := range mems {
		out = append(out, m.Insight)
	}
	return out, nil
}

// GetPeerModel retrieves the peer model for the given user ID.
// The peer model is stored as a JSON-encoded memory tagged
// "peer-model" with the user ID as the Actor. Returns nil (no
// error) when the store is nil or no peer model exists.
func (h *HonchoIntegration) GetPeerModel(ctx context.Context, userID string) (map[string]any, error) {
	if h == nil || h.store == nil {
		return nil, nil
	}
	mems, err := h.store.List(ListFilter{Tag: tagPeerModel, Actor: userID, Limit: 1})
	if err != nil {
		return nil, fmt.Errorf("honcho: list peer model: %w", err)
	}
	if len(mems) == 0 {
		return nil, nil
	}
	var model map[string]any
	if err := json.Unmarshal([]byte(mems[0].Insight), &model); err != nil {
		return nil, fmt.Errorf("honcho: parse peer model: %w", err)
	}
	return model, nil
}

// SavePreference stores a user preference as a tagged memory.
// The preference is stored as "key: value" in the Insight field
// with tag "preference". Returns nil (no-op) when the store is nil.
func (h *HonchoIntegration) SavePreference(ctx context.Context, key, value string) error {
	if h == nil || h.store == nil {
		return nil
	}
	insight := fmt.Sprintf("%s: %s", key, value)
	m := &Memory{
		Insight:    insight,
		Tags:       []string{tagPreference},
		Importance: 0.5,
	}
	if err := h.store.Add(m); err != nil {
		return fmt.Errorf("honcho: save preference: %w", err)
	}
	return nil
}

// SavePeerModel stores a peer model for the given user ID.
// The model map is JSON-encoded and stored as the Insight field
// with tag "peer-model" and Actor set to userID. Returns nil
// (no-op) when the store is nil.
func (h *HonchoIntegration) SavePeerModel(ctx context.Context, userID string, model map[string]any) error {
	if h == nil || h.store == nil {
		return nil
	}
	raw, err := json.Marshal(model)
	if err != nil {
		return fmt.Errorf("honcho: marshal peer model: %w", err)
	}
	m := &Memory{
		Insight:    string(raw),
		Tags:       []string{tagPeerModel},
		Actor:      userID,
		Importance: 0.8,
	}
	if err := h.store.Add(m); err != nil {
		return fmt.Errorf("honcho: save peer model: %w", err)
	}
	return nil
}

// FormatPreference renders a preference key-value pair as "key: value".
func FormatPreference(key, value string) string {
	return fmt.Sprintf("%s: %s", key, value)
}

// ParsePreference splits a "key: value" string back into key and value.
// Returns ok=false if the string does not contain a colon.
func ParsePreference(s string) (key, value string, ok bool) {
	idx := strings.Index(s, ": ")
	if idx < 0 {
		return "", "", false
	}
	return s[:idx], s[idx+2:], true
}
