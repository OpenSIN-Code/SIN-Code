# Issue: st-bvm3 — Bubbletea v2 upgrade

| Field       | Value                                                       |
|-------------|-------------------------------------------------------------|
| ID          | st-bvm3                                                     |
| Title       | Migrate TUI from Bubbletea v1.3.10 → v2.x                   |
| Status      | open                                                        |
| Priority    | P2 (deferrable past v3.0)                                   |
| Created     | 2026-06-08T13:52:00Z                                        |
| Reporter    | jeremy (via sin-code v2.4.0 audit)                          |
| Plan        | [docs/plans/bubbletea-v2-upgrade.md](../plans/bubbletea-v2-upgrade.md) |
| Component   | tui (Bubbletea)                                             |
| Effort      | 1-2 days                                                    |
| Blocks      | none                                                        |

## Summary

The TUI uses Bubbletea v1.3.10 (Charmbracelet). Bubbletea v2 has been released and brings:
- Composability via `tea.Model` interface with `Init()`, `Update()`, `View()` unchanged
- New `tea.Cmd` async composition primitives
- Better `lipgloss` v2 integration (no v1→v2 migration headaches if we lock to v2)
- Native OS clipboard via `tea.ClipboardMsg`
- Improved mouse handling and focus management

We have a workaround in place for image paste (`KeyMsg{Paste:true}` synthetic event in `tui/chat_input.go:42`) but this is a hack — v2 has native paste events.

## Why P2 (not P0)

v1.3.10 works. All TUI features (7 views, chat runner, paste workaround) are functional. The v2 upgrade is **quality-of-life** and **long-term maintainability**, not a bug.

## Current Workaround (v1.3.10)

In `tui/chat_input.go`:
```go
// Synthetic paste event for images — Bubbletea v1.3.10 doesn't have
// native clipboard paste. See docs/bubbletea-v2-migration.md.
type KeyMsg struct {
    Paste bool
    Runes []rune
}
```

This is read by `handleChatSubmit` and detected via reflection on the `Paste` field. Brittle, but works.

## v2 Migration Plan (high-level)

### Phase 1: Lock-in v2 compatible API surface
- Update `tui/model.go` to import `github.com/charmbracelet/bubbletea/v2`
- Add `tea.WithFilter` to handle paste events
- Remove the `KeyMsg.Paste` synthetic field

### Phase 2: Update all 7 views
- `tui/tools_view.go`, `tui/sessions_view.go`, `tui/efm_view.go`, `tui/config_view.go`, `tui/history_view.go`, `tui/todos_view.go`, `tui/chat_view.go`
- Verify each `Update()` method signature is compatible
- Migrate `lipgloss.Style` calls to v2 API

### Phase 3: Add native paste support
- Use `tea.PasteMsg` (new in v2) instead of synthetic event
- Test on macOS, Linux, Windows Terminal

### Phase 4: Cleanup
- Remove `docs/bubbletea-v2-migration.md` (no longer relevant)
- Update `AGENTS.md` to reference v2

## Acceptance Criteria

- [ ] All 7 TUI views render correctly under Bubbletea v2
- [ ] Image paste works via `tea.PasteMsg` (remove synthetic workaround)
- [ ] `go test ./tui/...` green with no skipped tests
- [ ] `tui/chat/runner_test.go` still passes
- [ ] Live test: TUI launches, all 7 views navigable, chat submits to NIM
- [ ] Documentation updated

## Risk Assessment

| Risk | Mitigation |
|------|------------|
| Bubbletea v2 breaking changes in `Update()` signature | Run `go vet ./tui/...` after dep upgrade; fix incrementally |
| lipgloss v2 API changes | Lock to v1 until TUI views migrated, then bump |
| Mouse event handling regressions | Manual smoke test on each OS |
| v2 still beta? | Pin to stable v2.0.0+; do NOT use rc/pre-release |

## References

- Current workaround: `tui/chat_input.go:42`
- Original plan: `docs/bubbletea-v2-migration.md`
- Bubbletea v2 release notes: https://github.com/charmbracelet/bubbletea/releases (check for stable v2.0+)

## Definition of Done

1. All 7 views verified under v2
2. Image paste works natively
3. All TUI tests pass
4. Live smoke test on macOS (other OS optional)
5. `docs/bubbletea-v2-migration.md` removed
6. v3.0.0 release notes mention "TUI: Bubbletea v2"
