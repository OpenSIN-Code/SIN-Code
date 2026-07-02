# GSD Tasks — Workflow

## Standard Workflow

1. `sin-code gsd init --name "Project" --description "Build X"`
2. `sin-code gsd phase add "Phase 1" --priority P0`
3. Create plan for phase (use `plan v2`)
4. `sin-code gsd execute 1` — analyze waves
5. Execute each wave (use `delegate-subagents`)
6. `self-review` + `dodone check`
7. `sin-code gsd phase edit 1 --status completed`
8. Repeat for next phase

## Quick Start (Single Phase)

```bash
sin-code gsd init --name "Quick Fix" --description "Fix bug X"
sin-code gsd phase add "Fix" --priority P0
# ... execute ...
sin-code gsd phase edit 1 --status completed
```
