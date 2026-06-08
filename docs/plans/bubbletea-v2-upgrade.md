# Plan: Bubbletea v2 Upgrade (v3.0.0)

**Status:** proposed
**Owner:** jeremy
**Target release:** v3.0.0 (Q4 2026)
**Related issue:** [st-bvm3](../issues/st-bvm3-bubbletea-v2-migration.md)
**Deprecated plan:** `docs/bubbletea-v2-migration.md` (will be removed)

---

## Executive Summary

The TUI uses Bubbletea v1.3.10 today (with `lipgloss v1`). Bubbletea v2 is stable and brings better async primitives, native clipboard/paste events, and improved focus management. We have a **working workaround** for image paste (synthetic `KeyMsg.Paste` field) but it's brittle.

This is a **quality-of-life and maintainability** upgrade — no user-facing bug to fix. Deferred to v3.0 because the cost of API churn outweighs the benefit until we have a clear trigger (e.g. dependency vulnerability, breaking change in v1).

---

## Trigger Conditions (any of these kicks off the upgrade)

1. Bubbletea v1.3.10 has a security advisory → forced upgrade
2. Go 1.25+ drops v1.x from `go mod tidy` default → forced upgrade
3. A new feature we need (e.g. native multi-pane focus groups) → upgrade enables it
4. v1.3.10 stops building on next Go release → forced upgrade

**Current state (June 2026):** No trigger active. v1.3.10 works on Go 1.21-1.24.

---

## Goals

1. Migrate from Bubbletea v1.3.10 → v2.x with **zero functional regressions**
2. Replace the synthetic paste workaround with native `tea.PasteMsg`
3. Lock `lipgloss` to v2 in the same release
4. All 7 TUI views pass smoke test
5. All TUI unit tests pass

## Non-Goals

- **Visual redesign** — keep current look-and-feel
- **New TUI features** — pure migration, no scope creep
- **lipgloss v2 style audit** — accept minor style diffs; only fix breaking changes
- **Mouse drag-and-drop** — defer to a later release

---

## Architecture Impact

### Files to Migrate (estimate)

| File | LOC | Complexity | Notes |
|------|-----|-----------|-------|
| `tui/model.go` | ~250 | Low | Main `tea.Model` impl |
| `tui/update.go` | ~180 | Low | `Update()` handler |
| `tui/chat_input.go` | ~140 | Medium | Remove paste workaround |
| `tui/chat_view.go` | ~120 | Low | View rendering |
| `tui/tools_view.go` | ~90 | Low | |
| `tui/sessions_view.go` | ~90 | Low | |
| `tui/efm_view.go` | ~80 | Low | |
| `tui/config_view.go` | ~100 | Low | |
| `tui/history_view.go` | ~90 | Low | |
| `tui/todos_view.go` | ~110 | Low | |
| `tui/chat/runner.go` | ~150 | Medium | Goroutine-based LLM calls |
| `tui/chat_program.go` | ~80 | Low | tea.Program wrapper |
| **Total** | **~1,480** | **Mostly Low** | |

### Dependencies to Bump

```diff
require (
-   github.com/charmbracelet/bubbletea v1.3.10
-   github.com/charmbracelet/lipgloss v1.0.0
+   github.com/charmbracelet/bubbletea/v2 v2.0.0
+   github.com/charmbracelet/lipgloss/v2 v2.0.0
)
```

---

## Migration Phases

### Phase 0: Spike (Day 1, 2 hours)

**Goal:** Validate the upgrade is feasible and identify the breaking changes we'll hit.

**Steps:**
1. Create branch `bubbletea-v2-spike`
2. Bump `go.mod` to `bubbletea/v2` latest stable
3. Run `go build ./...` — collect compile errors
4. Categorize errors by view file
5. Estimate per-view migration effort
6. Document the breaking changes in `docs/bubbletea-v2-migration.md` (UPDATE the existing doc, not new)
7. Decide: GO or NO-GO

**GO criteria:**
- < 50% of TUI LOC needs manual changes
- No fundamental API redesign needed (e.g. v2 is not a "new framework")
- All blocking issues have known workarounds

**NO-GO criteria (any one):**
- Bubbletea v2 is still beta (no stable v2.0.0+)
- lipgloss v2 has unfixed bugs we hit
- TUI requires > 1 week of refactor

---

### Phase 1: Core Model (Day 1-2, 4 hours)

**Goal:** Migrate `tui/model.go` + `tui/update.go` to v2 API.

**Breaking changes to handle:**
- `tea.KeyMsg` struct shape (likely backward compatible)
- `tea.WindowSizeMsg` field changes (if any)
- `tea.Cmd` return signatures (should be backward compatible)
- `tea.Program` constructor options (likely similar)

**Steps:**
1. Bump deps in `go.mod` and `go.sum`
2. Update imports: `github.com/charmbracelet/bubbletea` → `github.com/charmbracelet/bubbletea/v2`
3. Fix compile errors in `model.go`, `update.go`
4. Run `tui/...` unit tests
5. Manual smoke test: `go run ./cmd/sin-code tui` and verify Views render

---

### Phase 2: Paste Workaround Removal (Day 2, 2 hours)

**Goal:** Replace synthetic `KeyMsg.Paste` with native `tea.PasteMsg`.

**Current workaround in `tui/chat_input.go:42`:**
```go
type KeyMsg struct {
    Paste bool
    Runes []rune
}
```

**v2 replacement:**
```go
// v2 has a native paste message:
type pasteMsg struct{ content string }
func (p *chatInput) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.PasteMsg:
        // handle native paste
        p.insertImage(msg.Content)  // or insert text
    case tea.KeyMsg:
        // handle key press
    }
    return p, nil
}
```

**Steps:**
1. Remove synthetic `Paste` field from any internal types
2. Add `case tea.PasteMsg:` in `Update()`
3. Test image paste (base64-encoded PNG detection) + text paste
4. Test on macOS, Linux (Windows Terminal optional)

---

### Phase 3: View Migration (Day 2-3, 6 hours)

**Goal:** Update all 7 view files + chat runner.

**Views to migrate:**
- `tools_view.go`, `sessions_view.go`, `efm_view.go`, `config_view.go`, `history_view.go`, `todos_view.go`, `chat_view.go`

**Per-view checklist:**
- [ ] Imports updated to `bubbletea/v2`
- [ ] `Update()` method compiles
- [ ] `View()` method renders without panic
- [ ] lipgloss v1 → v2 styles work (or have v2 equivalents)
- [ ] Any tests pass

**Special case: `tui/chat/runner.go`** — uses `tea.Cmd` for async LLM calls. Should be largely compatible but verify.

---

### Phase 4: Lipgloss v2 Audit (Day 3, 2 hours)

**Goal:** Update all `lipgloss.Style` calls to v2 API.

**Likely breaking changes:**
- `Render()` signature may change
- `Style.Render()` method chaining may be different
- Color names: `lipgloss.Color("#FF0000")` should still work

**Steps:**
1. Run `gofmt -r 'lipgloss.NewStyle() -> lipgloss.NewStyle()'` (no-op if compatible)
2. Fix all compile errors
3. Visual diff: side-by-side screenshot of each view before/after
4. Acceptable: minor font/color diffs

---

### Phase 5: Testing (Day 3, 2 hours)

**Unit tests:**
- [ ] `tui/chat_view_test.go` passes
- [ ] `tui/chat/runner_test.go` passes
- [ ] All other `tui/*_test.go` pass

**E2E testscripts:**
- [ ] `tui_basic.txt` exists or NEW: smoke test launching TUI in test mode

**Manual smoke test:**
- [ ] `go run ./cmd/sin-code tui` launches
- [ ] All 7 views navigable via Tab/arrow keys
- [ ] Chat view accepts text input
- [ ] Chat view submits to NIM, displays response
- [ ] Image paste works (drag-and-drop a PNG)
- [ ] Ctrl+C exits cleanly

---

## Risk Assessment

| Risk | Severity | Mitigation |
|------|----------|------------|
| Bubbletea v2 has unfixed bugs | High | Pin to v2.0.0+; have v1.3.10 as fallback (revert) |
| lipgloss v2 visual diffs | Medium | Accept minor changes; document in release notes |
| Mouse event handling breaks | Medium | Manual smoke test on each OS; v2 has better mouse support, so this is unlikely |
| Async `tea.Cmd` patterns change | Low | v2 should be backward compatible; minimal code change |
| v2 deprecates something we use heavily | High | Spike phase identifies this before commit |
| Hidden state in `Update()` | Low | Tests catch most regressions |

---

## Rollback Plan

If v3.0.0 ships Bubbletea v2 and we discover a critical bug:
1. Cut v3.0.1 with v1.3.10 reverted
2. Bump `go.mod` back
3. Re-add synthetic `KeyMsg.Paste` workaround
4. v3.0.0 becomes a "preview" release, v3.0.1 is the "stable" release

**Cost of rollback:** ~4 hours (revert the v2-specific changes, ~300 LOC of diff)

---

## Timeline

| Phase | Effort | Day |
|-------|--------|-----|
| Phase 0: Spike | 2h | 1 |
| Phase 1: Core model | 4h | 1-2 |
| Phase 2: Paste workaround | 2h | 2 |
| Phase 3: Views | 6h | 2-3 |
| Phase 4: Lipgloss audit | 2h | 3 |
| Phase 5: Testing | 2h | 3 |
| Buffer (review, fixes) | 4h | 4 |
| **Total** | **~2 days** | **4 working days** |

---

## Acceptance Criteria (v3.0.0)

- [ ] `go.mod` uses `bubbletea/v2` and `lipgloss/v2` latest stable
- [ ] All TUI unit tests pass
- [ ] No synthetic `KeyMsg.Paste` workaround in code
- [ ] Native `tea.PasteMsg` handles image paste
- [ ] All 7 views render correctly
- [ ] Live smoke test on macOS passes
- [ ] `docs/bubbletea-v2-migration.md` is **removed** (no longer relevant)
- [ ] CHANGELOG.md v3.0.0 entry
- [ ] Git tag `v3.0.0`
- [ ] Release notes highlight "Bubbletea v2" as a major change

---

## References

- Current workaround: `tui/chat_input.go:42` (`KeyMsg.Paste` field)
- Original migration plan: `docs/bubbletea-v2-migration.md` (will be removed)
- Bubbletea v2 repo: https://github.com/charmbracelet/bubbletea (watch for v2.0+ stable)
- Issue: [st-bvm3](../issues/st-bvm3-bubbletea-v2-migration.md)
