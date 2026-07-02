# GSD Templates — Output Format

## Project Status Output

```
Project: <name>
Description: <description>
Phases: <N>
Completed: <N>
Completion: <X>%
Current phase: <phase title or "(all done)">
```

## Phase List Output

```
ID     PRIORITY STATUS       TITLE
1      P0       planning     Backend API
2      P1       in-progress  Frontend
3      P2       completed    Documentation
```

## Execute Output

```
Phase: 1 — Backend API
Waves: 3
Completed: 2/5 tasks (40%)
Next wave: 2

Wave 1 (ready):
  [done] T1: Set up database schema
  [done] T2: Create models

Wave 2 (next):
  [ ] T3: Implement API handlers (deps: T1, T2)
  [ ] T4: Add middleware (deps: T1)

Wave 3 (blocked):
  [ ] T5: Integration tests (deps: T3, T4)
```
