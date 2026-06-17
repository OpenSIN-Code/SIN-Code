// SPDX-License-Identifier: MIT
// Purpose: tests for the telemetry usage provider.
package telemetry

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

func TestLedgerProvider_UsedTools(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "s1", Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_read"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "s1", Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_write"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "s1", Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_read"}}); err != nil {
		t.Fatal(err)
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used["sin_read"] != 2 {
		t.Errorf("expected sin_read=2, got %d", used["sin_read"])
	}
	if used["sin_write"] != 1 {
		t.Errorf("expected sin_write=1, got %d", used["sin_write"])
	}
	if used["sin_edit"] != 0 {
		t.Errorf("expected sin_edit=0, got %d", used["sin_edit"])
	}
}

func TestLedgerProvider_NilStore(t *testing.T) {
	p := NewLedgerProvider(nil)
	used, err := p.UsedTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Errorf("expected empty usage, got %v", used)
	}
}

func TestStub(t *testing.T) {
	used, err := Stub().UsedTools(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 0 {
		t.Errorf("expected empty usage, got %v", used)
	}
}

func TestLedgerProvider_IgnoresNonToolCall(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	ctx := context.Background()
	if _, err := store.Record(ctx, ledger.Entry{SessionID: "s1", Type: ledger.TypeUserPrompt, Data: map[string]any{"tool": "sin_read"}}); err != nil {
		t.Fatal(err)
	}

	p := NewLedgerProvider(store)
	used, err := p.UsedTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if used["sin_read"] != 0 {
		t.Errorf("user prompt should not count as tool call, got %d", used["sin_read"])
	}
}

func TestLedgerProvider_Race(t *testing.T) {
	db := filepath.Join(t.TempDir(), "ledger.db")
	store, err := ledger.Open(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	ctx := context.Background()
	p := NewLedgerProvider(store)
	for i := 0; i < 20; i++ {
		i := i
		t.Run("parallel", func(t *testing.T) {
			t.Parallel()
			_, err := store.Record(ctx, ledger.Entry{SessionID: "race", Type: ledger.TypeToolCall, Data: map[string]any{"tool": "sin_read", "i": i}})
			if err != nil {
				t.Fatal(err)
			}
			_, _ = p.UsedTools(ctx)
		})
	}
}
