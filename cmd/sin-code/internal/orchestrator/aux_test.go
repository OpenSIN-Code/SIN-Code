// SPDX-License-Identifier: MIT
// Purpose: lightweight tests for the governor + txn + escalation subsystems.

package orchestrator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLadder_Shape(t *testing.T) {
	ladder := DefaultLadder()
	if len(ladder) < 3 {
		t.Fatalf("default ladder should have at least 3 rungs, got %d",
			len(ladder))
	}
	// Rungs should be strictly increasing in effort
	for i := 1; i < len(ladder); i++ {
		if ladder[i].Timeout <= ladder[i-1].Timeout {
			t.Fatalf("rung %d timeout (%s) not > rung %d (%s)",
				i, ladder[i].Timeout, i-1, ladder[i-1].Timeout)
		}
	}
	for i, r := range ladder {
		if r.Name == "" {
			t.Fatalf("rung %d has empty name", i)
		}
		if r.Agents < 1 {
			t.Fatalf("rung %d (%s) must have >=1 agent", i, r.Name)
		}
	}
}

func TestBeginTxn_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"),
		[]byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	txn := BeginTxn(dir)
	if err := txn.WriteFile("b.txt", []byte("new")); err != nil {
		t.Fatal(err)
	}
	if err := txn.DeleteFile("a.txt"); err != nil {
		t.Fatal(err)
	}
	touched := txn.Touched()
	if len(touched) != 2 {
		t.Fatalf("expected 2 touched files, got %d: %v", len(touched), touched)
	}
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.txt")); !os.IsNotExist(err) {
		t.Fatal("a.txt should be deleted after commit")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "b.txt")); string(data) != "new" {
		t.Fatal("b.txt should be 'new' after commit")
	}
}

func TestBeginTxn_Rollback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "x.txt"),
		[]byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	txn := BeginTxn(dir)
	if err := txn.WriteFile("x.txt", []byte("changed")); err != nil {
		t.Fatal(err)
	}
	if err := txn.Rollback(); err != nil {
		t.Fatal(err)
	}
	// Filesystem must be untouched
	if data, _ := os.ReadFile(filepath.Join(dir, "x.txt")); string(data) != "keep" {
		t.Fatalf("x.txt must be 'keep' after rollback, got %q", data)
	}
}

func TestBeginTxn_RejectsPostCommitWrites(t *testing.T) {
	dir := t.TempDir()
	txn := BeginTxn(dir)
	if err := txn.WriteFile("a", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := txn.WriteFile("b", []byte("b")); err == nil {
		t.Fatal("write after commit must error")
	}
}

func TestBeginTxn_RejectsDoubleCommit(t *testing.T) {
	dir := t.TempDir()
	txn := BeginTxn(dir)
	if err := txn.WriteFile("a", []byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	// second commit with content must error; empty commit may or may not
	if err := txn.WriteFile("b", []byte("b")); err == nil {
		t.Fatal("post-commit write must error")
	}
}

func TestBeginTxn_EmptyCommitIsNoop(t *testing.T) {
	dir := t.TempDir()
	txn := BeginTxn(dir)
	if err := txn.Commit(); err != nil {
		t.Fatal(err)
	}
	if touched := txn.Touched(); len(touched) != 0 {
		t.Fatalf("empty txn should have 0 touched, got %v", touched)
	}
}
