// SPDX-License-Identifier: MIT
// Purpose: telemetry — usage tracking for sin-code tools. The default
// provider reads tool-call events from the semantic session ledger
// (internal/ledger). If the ledger is unavailable or empty, the provider
// returns an empty usage map (fail-open for catalog discovery).
// Docs: telemetry.doc.md
package telemetry

import (
	"context"
	"os"
	"path/filepath"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ledger"
)

// Provider returns tool usage counts.
type Provider interface {
	// UsedTools returns a map from tool name (or namespaced name) to the
	// number of times it has been recorded in the ledger.
	UsedTools(ctx context.Context) (map[string]int64, error)
}

// LedgerProvider reads TypeToolCall entries from a ledger.Store.
type LedgerProvider struct {
	store *ledger.Store
}

// NewLedgerProvider wraps a ledger store. The store may be nil; in that case
// UsedTools returns an empty map.
func NewLedgerProvider(store *ledger.Store) *LedgerProvider {
	return &LedgerProvider{store: store}
}

// UsedTools implements Provider.
func (p *LedgerProvider) UsedTools(ctx context.Context) (map[string]int64, error) {
	out := make(map[string]int64)
	if p.store == nil {
		return out, nil
	}
	// List all sessions (bounded) and then scan their tool calls.
	sessions, err := p.store.Sessions(ctx, 1000)
	if err != nil {
		return out, nil
	}
	for _, sid := range sessions {
		entries, err := p.store.QueryByType(ctx, sid, ledger.TypeToolCall, 10000)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if name, ok := e.Data["tool"].(string); ok && name != "" {
				out[name]++
			}
		}
	}
	return out, nil
}

// DefaultProvider opens the default ledger path and returns a Provider.
// If the ledger cannot be opened, it returns a nil-store provider that
// reports empty usage.
func DefaultProvider() (Provider, error) {
	path := defaultPath()
	store, err := ledger.Open(path)
	if err != nil {
		return NewLedgerProvider(nil), nil
	}
	return NewLedgerProvider(store), nil
}

// defaultPath returns the default ledger path, respecting SIN_CODE_HOME.
func defaultPath() string {
	if h := os.Getenv("SIN_CODE_HOME"); h != "" {
		return filepath.Join(h, "ledger.db")
	}
	return ledger.DefaultPath()
}

// stubProvider is a Provider that always returns empty usage.
type stubProvider struct{}

func (stubProvider) UsedTools(_ context.Context) (map[string]int64, error) {
	return map[string]int64{}, nil
}

// Stub returns a Provider that reports no usage. Used as a fallback when
// no real telemetry backend is available.
func Stub() Provider { return stubProvider{} }
