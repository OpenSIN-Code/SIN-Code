// SPDX-License-Identifier: MIT
// Purpose: coverage-boost tests for telemetry — DefaultProvider,
// defaultPath, and edge cases in UsedTools (error paths, empty tool
// name, non-string tool data).
package telemetry

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

// ── DefaultProvider (telemetry.go:63) — was 0% ──────────────────

func TestDefaultProvider_SIN_CODE_HOME(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SIN_CODE_HOME", tmp)

	p, err := DefaultProvider()
	if err != nil {
		t.Fatalf("DefaultProvider: %v", err)
	}
	if p == nil {
		t.Fatal("DefaultProvider returned nil")
	}
	used, err := p.UsedTools(context.Background())
	if err != nil {
		t.Fatalf("UsedTools: %v", err)
	}
	if len(used) != 0 {
		t.Fatalf("fresh ledger should have empty usage, got %v", used)
	}
}

func TestDefaultProvider_InvalidPathReturnsEmptyProvider(t *testing.T) {
	// Point SIN_CODE_HOME to a path that will fail to open as a
	// SQLite DB (a directory, not a file).
	tmp := t.TempDir()
	t.Setenv("SIN_CODE_HOME", tmp)

	// Create a file that is NOT a valid SQLite database to force
	// ledger.Open to fail.
	dbPath := filepath.Join(tmp, "ledger.db")
	if err := os.WriteFile(dbPath, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := DefaultProvider()
	if err != nil {
		t.Fatalf("DefaultProvider should not error on bad ledger: %v", err)
	}
	if p == nil {
		t.Fatal("DefaultProvider returned nil")
	}
	// Should return empty usage (fail-open).
	used, err := p.UsedTools(context.Background())
	if err != nil {
		t.Fatalf("UsedTools: %v", err)
	}
	if len(used) != 0 {
		t.Fatalf("expected empty usage from stub provider, got %v", used)
	}
}

// ── defaultPath (telemetry.go:73) — was 0% ──────────────────────

func TestDefaultPath_WithSIN_CODE_HOME(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "/custom/home")
	got := defaultPath()
	want := filepath.Join("/custom/home", "ledger.db")
	if got != want {
		t.Fatalf("defaultPath = %q, want %q", got, want)
	}
}

func TestDefaultPath_FallbackToLedgerDefault(t *testing.T) {
	t.Setenv("SIN_CODE_HOME", "")
	got := defaultPath()
	want := ledger.DefaultPath()
	if got != want {
		t.Fatalf("defaultPath = %q, want %q (ledger.DefaultPath)", got, want)
	}
}

// ── UsedTools edge cases ────────────────────────────────────────

func TestUsedTools_EmptyToolNameIgnored(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	// Record a tool call with an empty tool name — should be ignored.
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "s1",
		Type:      ledger.TypeToolCall,
		Data:      map[string]any{"tool": ""},
	}); err != nil {
		t.Fatal(err)
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Fatalf("empty tool name should be ignored, got %v", used)
	}
}

func TestUsedTools_NonStringToolFieldIgnored(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	// Record a tool call where "tool" is an int, not a string.
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "s1",
		Type:      ledger.TypeToolCall,
		Data:      map[string]any{"tool": 42},
	}); err != nil {
		t.Fatal(err)
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Fatalf("non-string tool field should be ignored, got %v", used)
	}
}

func TestUsedTools_MissingToolFieldIgnored(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	// Record a tool call with no "tool" key at all.
	if _, err := store.Record(ctx, ledger.Entry{
		SessionID: "s1",
		Type:      ledger.TypeToolCall,
		Data:      map[string]any{"other": "value"},
	}); err != nil {
		t.Fatal(err)
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Fatalf("missing tool field should be ignored, got %v", used)
	}
}

func TestUsedTools_MultipleSessions(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	for _, sid := range []string{"s1", "s2", "s3"} {
		if _, err := store.Record(ctx, ledger.Entry{
			SessionID: sid, Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_read"},
		}); err != nil {
			t.Fatal(err)
		}
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used["sin_read"] != 3 {
		t.Fatalf("expected sin_read=3 across 3 sessions, got %d", used["sin_read"])
	}
}

// ── Stub provider (telemetry.go:89) ─────────────────────────────

func TestStub_AlwaysEmpty(t *testing.T) {
	p := Stub()
	used, err := p.UsedTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Fatalf("stub should always return empty, got %v", used)
	}
}

// ── NewLedgerProvider with closed store ─────────────────────────

func TestUsedTools_StoreErrorReturnsEmpty(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	store.Close() // close immediately

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(context.Background())
	if err != nil {
		t.Fatalf("UsedTools should fail-open on store error: %v", err)
	}
	if len(used) != 0 {
		t.Fatalf("closed store should return empty usage, got %v", used)
	}
}
