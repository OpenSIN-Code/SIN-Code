# gsd — Get Shit Done

Native Go package for project lifecycle management: project init, phase
CRUD, plan creation, and wave-based execution state.

## Overview

Replaces the npm-based GSD system with pure Go (stdlib only, mandate M2).
Manages markdown state files in a `.gsd/` directory at the project root.

## State files

```
.gsd/
├── PROJECT.md    — project metadata (name, description, tech stack)
├── ROADMAP.md    — ordered phases with priority and status
├── STATE.md      — current phase + history
└── plans/
    └── phase-1.md — task list for a phase
```

## API

### Project

| Function | Description |
|----------|-------------|
| `InitProject(root, name, description) error` | Creates `.gsd/` dir and initial files |
| `LoadProject(root) (*Project, error)` | Reads PROJECT.md |
| `ProjectStatus(root) (*StatusReport, error)` | Returns project info + phase stats |

### Phase CRUD

| Function | Description |
|----------|-------------|
| `AddPhase(root, title, priority) (*Phase, error)` | Appends to ROADMAP.md with auto-incremented ID |
| `InsertPhase(root, afterID, title, priority) (*Phase, error)` | Inserts with decimal ID (e.g. 1.5) |
| `RemovePhase(root, id) error` | Removes phase and renumbers integer IDs |
| `EditPhase(root, id, opts) error` | Updates title/priority/status |
| `ListPhases(root) ([]Phase, error)` | Returns all phases |
| `GetPhase(root, id) (*Phase, error)` | Returns single phase |

### Plan

| Function | Description |
|----------|-------------|
| `SavePlan(root, phaseID, tasks) error` | Writes `.gsd/plans/phase-N.md` |
| `LoadPlan(root, phaseID) (*Plan, error)` | Reads plan file |
| `PlanExists(root, phaseID) bool` | Checks plan existence |

### Execution

| Function | Description |
|----------|-------------|
| `AnalyzeWaves(plan) [][]Task` | Topological sort into dependency waves |
| `UpdateTaskStatus(root, phaseID, taskID, status) error` | Marks task done/blocked/in-progress |
| `ExecuteState(root, phaseID) (*ExecuteReport, error)` | Returns waves + progress |

## Constants

- **Phase status**: `planning`, `in-progress`, `completed`, `blocked`
- **Task status**: `todo`, `in-progress`, `done`, `blocked`
- **Priority**: `P0`, `P1`, `P2`, `P3`
- **Effort**: `S`, `M`, `L`

## Mandates honored

- **M2**: Pure stdlib, no external dependencies, CGO_ENABLED=0 compatible.
- **M7**: Race-free (no shared mutable state; all file I/O is atomic per call).
- File mode: 0o644 for all writes.
