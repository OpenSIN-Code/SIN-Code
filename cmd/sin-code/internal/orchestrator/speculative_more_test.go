// SPDX-License-Identifier: MIT
// Purpose: lightweight tests for the speculative execution runner.

package orchestrator

import "testing"

func TestNewSpeculativeRunner_NoRepo(t *testing.T) {
	s := NewSpeculativeRunner("", nil)
	if s == nil {
		t.Fatal("nil runner")
	}
	if dir := s.worktreeDir("id"); dir == "" {
		t.Fatal("worktreeDir should be deterministic, even without repo")
	}
}

func TestNewSpeculativeRunner_RemovesCleanly(t *testing.T) {
	s := NewSpeculativeRunner("/tmp/nonexistent", nil)
	// removeWorktree on a never-added dir must be a safe no-op
	s.removeWorktree("/tmp/sin-nonexistent-xyz")
}

func TestMaxInt(t *testing.T) {
	if maxInt(1, 2) != 2 {
		t.Fatal("maxInt broken")
	}
	if maxInt(5, 5) != 5 {
		t.Fatal("maxInt equal broken")
	}
	if maxInt(9, 3) != 9 {
		t.Fatal("maxInt broken")
	}
}
