# Pocock Workflow Tools

The Pocock Workflow implements the **Matt Pocock System-Design Paradigm** for SIN-Code agents — a structured approach to software engineering that emphasizes:

1. **Socratic Alignment** — Ask clarifying questions before writing code
2. **TDD Enforcement** — Red-Green-Refactor cycle with lock files
3. **DAG Orchestration** — Parallelize work via topological sorting
4. **Safe Bootstrapping** — Environment patching and cleanup
5. **Multi-Agent Swarms** — Coordinated agent collaboration

## Tools

| Tool | CLI Command | Purpose | Type |
|------|-------------|---------|------|
| **grill-me** | `sin pocock grill-me` | Socratic PRD generation | Python |
| **tdd-enforcer** | `sin pocock tdd-enforcer` | TDD cycle lock files | Python |
| **dag-kanban** | `sin pocock dag-kanban` | DAG-based task orchestration | Python |
| **zod-patch** | Manual / auto-injected | Zod v3/v4 compatibility | JS |
| **safe-start** | `sin pocock safe-start` | Safe env var bootstrap | Shell |
| **cleanup-hook** | `sin pocock cleanup` | Post-flight cleanup | Shell |
| **teammate** | Manual | Multi-agent swarm adapter | JS |

## Quick Start

### 1. Generate a PRD (Socratic Alignment)

Interactive mode:
```bash
sin pocock grill-me "Build a real-time chat system"
```

Non-interactive mode (CI/CD):
```bash
sin pocock grill-me "Build a real-time chat system" \
  --non-interactive \
  --answers '{"problem_definition":"...","stakeholders":"...","constraints":"...","edge_cases":"...","migration":"...","boundaries":"...","success_metrics":"...","integration":"...","rollback":"...","dependencies":"..."}' \
  --output PRD.md
```

### 2. Parse PRD into DAG

```bash
sin pocock dag-kanban --prd PRD.md --json
```

With Docker Compose export:
```bash
sin pocock dag-kanban --prd PRD.md --docker --output docker-compose.yml
```

### 3. Enforce TDD Cycle

Check lock status:
```bash
sin pocock tdd-enforcer "pytest tests/" src/feature.py --check --json
```

Enforce TDD cycle (requires tests to pass before editing):
```bash
sin pocock tdd-enforcer "pytest tests/" src/feature.py
```

Reset TDD state:
```bash
sin pocock tdd-enforcer "pytest tests/" src/feature.py --reset
```

### 4. Safe Bootstrap

```bash
sin pocock safe-start
```

### 5. Cleanup

```bash
sin pocock cleanup
```

## PRD Format

The `dag-kanban` tool expects PRD files in this format:

```markdown
# My Feature

## Technische Spezifikation

- [ ] Slice 1: Foundation - Create basic structure
- [ ] Slice 2: Core Feature - Implement main logic
- [ ] Slice 3: Polish - Add tests and docs
```

Checkboxes (`- [ ]`) are required for the parser to recognize tasks.

## TDD Lock Files

The TDD enforcer creates lock files in `.tdd-locks/`:
- `*.red.lock` — Waiting for failing test (RED phase)
- `*.green.lock` — Waiting for passing test (GREEN phase)
- `*.refactor.lock` — Refactoring allowed (REFACTOR phase)

These are automatically added to `.gitignore`.

## Multi-Agent Swarms

The `teammate-adapter.js` enables coordinated multi-agent collaboration:

```bash
node scripts/pocock/teammate-adapter.js
```

Agents communicate via shared memory (`/tmp/.opencode-swarm`) with:
- **State machine**: IDLE → PLANNING → EXECUTING → REVIEWING → SYNCING
- **Round-robin scheduling** with 100ms quantum
- **Conflict resolution** via agent ranking (senior > junior)
- **Heartbeat monitoring** with 30s timeout

## Zod Compatibility

The `opencode-zod-patch.js` fixes Zod v3/v4 compatibility issues:

```bash
node -r scripts/pocock/opencode-zod-patch.js your-script.js
```

Or let `safe-start` auto-inject it:
```bash
sin pocock safe-start
```

## Architecture

```
sin_code_bundle/tools/pocock/
├── __init__.py          # Module exports
├── grill_me.py          # Socratic alignment engine
├── tdd_enforcer.py      # TDD cycle enforcer
├── dag_kanban.py         # DAG orchestrator
└── *.doc.md              # CoDocs documentation

scripts/pocock/
├── opencode-zod-patch.js      # Zod compatibility patch
├── opencode-safe-start.sh     # Safe bootstrap
├── opencode-cleanup-hook.sh   # Post-flight cleanup
├── teammate-adapter.js        # Multi-agent swarm
└── *.doc.md                   # CoDocs documentation
```

## Workflow

```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│  grill-me   │───▶│  dag-kanban │───▶│  teammate   │
│  (PRD)      │    │  (Tasks)    │    │  (Swarm)    │
└─────────────┘    └─────────────┘    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │ tdd-enforcer│
                    │ (TDD cycle) │
                    └─────────────┘
                           │
                           ▼
                    ┌─────────────┐
                    │   cleanup   │
                    │ (post-flight)│
                    └─────────────┘
```

## Documentation

Each tool has a `.doc.md` companion file following the CoDocs standard:
- `grill_me.py` → `grill_me.doc.md`
- `tdd_enforcer.py` → `tdd_enforcer.doc.md`
- `dag_kanban.py` → `dag_kanban.doc.md`
- `opencode-zod-patch.js` → `opencode-zod-patch.doc.md`
- `opencode-safe-start.sh` → `opencode-safe-start.doc.md`
- `opencode-cleanup-hook.sh` → `opencode-cleanup-hook.doc.md`
- `teammate-adapter.js` → `teammate-adapter.doc.md`

## License

MIT — same as SIN-Code-Bundle.
