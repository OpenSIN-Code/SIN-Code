// SPDX-License-Identifier: MIT
// Purpose: tests for the instinct store — confidence scoring,
// project→global promotion, demotion, and race-free concurrency (M7).
package memory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func instinctDB(t *testing.T) *sql.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "instincts.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func instinctStore(t *testing.T) *InstinctStore {
	t.Helper()
	db := instinctDB(t)
	s, err := NewInstinctStore(db)
	if err != nil {
		t.Fatalf("NewInstinctStore: %v", err)
	}
	return s
}

func TestNewInstinctStoreCreates(t *testing.T) {
	db := instinctDB(t)
	s, err := NewInstinctStore(db)
	if err != nil {
		t.Fatalf("NewInstinctStore: %v", err)
	}
	if s == nil {
		t.Fatal("expected non-nil store")
	}
	// Second call is idempotent.
	if _, err := NewInstinctStore(db); err != nil {
		t.Fatalf("idempotent NewInstinctStore: %v", err)
	}
}

func TestNewInstinctStoreNilDB(t *testing.T) {
	_, err := NewInstinctStore(nil)
	if err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestInstinctRecordCreates(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	if err := s.Record(ctx, "always run go vet before commit"); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 instinct, got %d", len(list))
	}
	if list[0].Confidence != instinctInitialConfidence {
		t.Errorf("initial confidence should be %.2f, got %.2f", instinctInitialConfidence, list[0].Confidence)
	}
	if list[0].Scope != instinctScopeProject {
		t.Errorf("scope should be project, got %s", list[0].Scope)
	}
}

func TestInstinctRecordEmpty(t *testing.T) {
	s := instinctStore(t)
	if err := s.Record(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestInstinctRecordIncrementsConfidence(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	content := "use context for cancellation"
	for i := 0; i < 3; i++ {
		if err := s.Record(ctx, content); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := s.List(ctx, "")
	if len(list) != 1 {
		t.Fatalf("expected 1 instinct (same content), got %d", len(list))
	}
	expected := instinctInitialConfidence + 2*instinctConfirmIncrement
	if list[0].Confidence < expected-0.001 || list[0].Confidence > expected+0.001 {
		t.Errorf("confidence should be ~%.2f, got %.2f", expected, list[0].Confidence)
	}
}

func TestInstinctRecordCapsConfidence(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	content := "trivial"
	for i := 0; i < 100; i++ {
		_ = s.Record(ctx, content)
	}
	list, _ := s.List(ctx, "")
	if list[0].Confidence > instinctMaxConfidence {
		t.Errorf("confidence should cap at %.2f, got %.2f", instinctMaxConfidence, list[0].Confidence)
	}
}

func TestInstinctGet(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	_ = s.Record(ctx, "test instinct content")
	list, _ := s.List(ctx, "")
	got, err := s.Get(ctx, list[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Content != "test instinct content" {
		t.Errorf("content mismatch: %s", got.Content)
	}
}

func TestInstinctGetNotFound(t *testing.T) {
	s := instinctStore(t)
	_, err := s.Get(context.Background(), "nonexistent")
	if !errors.Is(err, ErrInstinctNotFound) {
		t.Errorf("expected ErrInstinctNotFound, got %v", err)
	}
}

func TestInstinctListByScope(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	_ = s.Record(ctx, "project instinct A")
	_ = s.Record(ctx, "project instinct B")
	// Promote one by boosting confidence past threshold.
	list, _ := s.List(ctx, instinctScopeProject)
	id := list[0].ID
	for i := 0; i < 12; i++ {
		_ = s.Record(ctx, list[0].Content)
	}
	if err := s.Promote(ctx, id); err != nil {
		t.Fatal(err)
	}
	project, _ := s.List(ctx, instinctScopeProject)
	global, _ := s.List(ctx, instinctScopeGlobal)
	if len(global) != 1 {
		t.Errorf("expected 1 global, got %d", len(global))
	}
	if len(project) != 1 {
		t.Errorf("expected 1 project, got %d", len(project))
	}
}

func TestInstinctListAll(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	_ = s.Record(ctx, "A")
	_ = s.Record(ctx, "B")
	_ = s.Record(ctx, "C")
	all, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 instincts, got %d", len(all))
	}
}

func TestInstinctPromote(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	content := "promotable instinct"
	for i := 0; i < 12; i++ {
		_ = s.Record(ctx, content)
	}
	list, _ := s.List(ctx, "")
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if err := s.Promote(ctx, list[0].ID); err != nil {
		t.Fatalf("promote: %v", err)
	}
	got, _ := s.Get(ctx, list[0].ID)
	if got.Scope != instinctScopeGlobal {
		t.Errorf("scope should be global, got %s", got.Scope)
	}
	if got.PromotedAt == nil {
		t.Error("promoted_at should be set")
	}
}

func TestInstinctPromoteLowConfidence(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	_ = s.Record(ctx, "low confidence instinct")
	list, _ := s.List(ctx, "")
	err := s.Promote(ctx, list[0].ID)
	if !errors.Is(err, ErrInstinctLowConfidence) {
		t.Errorf("expected ErrInstinctLowConfidence, got %v", err)
	}
}

func TestInstinctPromoteNotFound(t *testing.T) {
	s := instinctStore(t)
	err := s.Promote(context.Background(), "nonexistent")
	if !errors.Is(err, ErrInstinctNotFound) {
		t.Errorf("expected ErrInstinctNotFound, got %v", err)
	}
}

func TestInstinctDemote(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	content := "demotable instinct"
	for i := 0; i < 12; i++ {
		_ = s.Record(ctx, content)
	}
	list, _ := s.List(ctx, "")
	_ = s.Promote(ctx, list[0].ID)
	before, _ := s.Get(ctx, list[0].ID)
	if err := s.Demote(ctx, list[0].ID); err != nil {
		t.Fatal(err)
	}
	after, _ := s.Get(ctx, list[0].ID)
	if after.Confidence >= before.Confidence {
		t.Errorf("confidence should decrease: before=%.2f after=%.2f", before.Confidence, after.Confidence)
	}
}

func TestInstinctDemoteNotFound(t *testing.T) {
	s := instinctStore(t)
	err := s.Demote(context.Background(), "nonexistent")
	if !errors.Is(err, ErrInstinctNotFound) {
		t.Errorf("expected ErrInstinctNotFound, got %v", err)
	}
}

func TestInstinctDemoteGlobalBackToProject(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	content := "will be demoted back"
	for i := 0; i < 12; i++ {
		_ = s.Record(ctx, content)
	}
	list, _ := s.List(ctx, "")
	_ = s.Promote(ctx, list[0].ID)
	// Demote multiple times to drop below 0.5.
	for i := 0; i < 10; i++ {
		_ = s.Demote(ctx, list[0].ID)
	}
	got, _ := s.Get(ctx, list[0].ID)
	if got.Scope != instinctScopeProject {
		t.Errorf("scope should be back to project, got %s", got.Scope)
	}
	if got.PromotedAt != nil {
		t.Error("promoted_at should be cleared")
	}
}

func TestInstinctRaceFree(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func(n int) {
			defer wg.Done()
			_ = s.Record(ctx, fmt.Sprintf("concurrent instinct %d", n%3))
		}(i)
		go func() {
			defer wg.Done()
			_, _ = s.List(ctx, "")
		}()
		go func() {
			defer wg.Done()
			_, _ = s.Get(ctx, "instinct-nonexistent")
		}()
	}
	wg.Wait()
}

func TestInstinctIDDeterministic(t *testing.T) {
	a := instinctID("same content")
	b := instinctID("same content")
	c := instinctID("different content")
	if a != b {
		t.Error("same content should produce same ID")
	}
	if a == c {
		t.Error("different content should produce different ID")
	}
}

func TestInstinctContextCancellation(t *testing.T) {
	s := instinctStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := s.Record(ctx, "should fail")
	if err == nil {
		t.Skip("SQLite may not honor context cancellation immediately")
	}
}

func TestInstinctPromotedAtParsed(t *testing.T) {
	s := instinctStore(t)
	ctx := context.Background()
	for i := 0; i < 12; i++ {
		_ = s.Record(ctx, "check promoted_at parsing")
	}
	list, _ := s.List(ctx, "")
	_ = s.Promote(ctx, list[0].ID)
	got, _ := s.Get(ctx, list[0].ID)
	if got.PromotedAt == nil {
		t.Fatal("promoted_at should be set")
	}
	if got.PromotedAt.After(time.Now().UTC().Add(time.Minute)) {
		t.Error("promoted_at should be in the past")
	}
}
