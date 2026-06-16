# SIN-Code Instincts

The continuous-learning subsystem. An **Instinct** is a small
confidence-weighted learned behavior (trigger → action) discovered
from session activity. The model sees its own past behavior injected
as a system-prompt block on every turn and adjusts future actions
accordingly.

> **Code:** `cmd/sin-code/internal/instinct/` (10 files, ~2700 LOC,
> ~1100 LOC tests). Clean-room Go reimplementation of the
> `continuous-learning-v2` pattern; no Node dependency.
>
> **CLI:** `sin instinct ...` (10 subcommands).
>
> **Spec:** AGENTS.md §1 (this repo's source of truth).

## What an Instinct is

A Markdown file with YAML frontmatter:

```markdown
---
id: when-committing-2b9c7e
trigger: "when committing"
confidence: 0.71
domain: git
scope: project
project_id: 19f88a6bdfc1
project_name: sin-code
status: active
observations: 4
created_at: 2026-06-16T00:00:00Z
updated_at: 2026-06-16T01:23:45Z
---

# When committing

## Action

Run the test suite first.

## Evidence

- go test ./...
- git commit -m "..."
```

| Field | Meaning |
|---|---|
| `id` | Stable slug + 6-char SHA-256 suffix of (trigger+domain). Used as filename and dedup key. |
| `trigger` | Natural-language condition, e.g. `"when committing"`, `"when adding tests"`. |
| `confidence` | 0.30–0.90, floating point. `< 0.60` = pending, `≥ 0.60` = active, `≥ 0.70` = evolve-eligibel. |
| `domain` | One of: `git`, `testing`, `build`, `infra`, `dependencies`, `network`, `database`, `code-style`, `security`, `go`, `rust`, `python`, `typescript`, `docs`, `navigation`, `general`. Set by `Classify(tool, meta)`. |
| `scope` | `project` (only in the originating repo) or `global` (every project). Promotion: ≥ 2 distinct projects at `PromotionThreshold`. |
| `status` | `pending` / `active` / `evolved` / `archived`. Set by confidence math. |
| `observations` | Counter; each Reinforce() bumps by 1. |

## Lifecycle

```
                  observe
observe ──► Manager.Observe(candidate) ──► New | Reinforce
                                              │
                                       ┌──────┴──────┐
                                       ▼              ▼
                                pending (0.30)  active (≥0.60)
                                                  │
                                       ┌──────────┼──────────┐
                                       ▼          ▼          ▼
                                  evolved     decay      contradict
                                  (≥0.70,   (over time,  (one counter-
                                   ≥3 obs)   lower conf)  signal)
```

The 0.30 floor and 0.90 ceiling are non-configurable. The
activation/evolve thresholds and the reinforce/contradict steps are
env-overridable:

| Env var | Default | Effect |
|---|---|---|
| `SIN_INSTINCT_ACTIVATION` | 0.60 | pending → active cutoff |
| `SIN_INSTINCT_EVOLVE` | 0.70 | evolve eligibility (also needs ≥ 3 observations) |
| `SIN_INSTINCT_REINFORCE` | 0.25 | fraction of gap-to-max added on Reinforce |
| `SIN_INSTINCT_CONTRADICT` | 0.40 | fraction of gap-to-floor subtracted on Contradict |
| `SIN_INSTINCT_PROMOTE_N` | 2 | min distinct projects for promotion |
| `SIN_INSTINCT_TTL_DAYS` | 30 | pending TTL for `sin instinct prune` |

All read once at startup via `LoadConfig()` + `ApplyConfig()`. Hot-path
math is lock-free via `atomic.Value`.

## Storage

```
$XDG_DATA_HOME/sin-code/instinct/    (defaults to ~/.local/share/...)
├── global/
│   └── instincts/<id>.md
├── projects/
│   └── <project_id>/
│       ├── meta.json
│       └── instincts/<id>.md
└── audit.jsonl                     # append-only learning event log
```

`<project_id>` = first 12 hex chars of SHA-256(normalized_git_remote).
The project is detected at startup by `DetectProject(workdir)`:

1. `git config --get remote.origin.url`
2. Normalize (strip credentials, drop `.git`, lowercase, drop protocol)
3. SHA-256 → first 12 hex chars

Same checkout, different machines → same ID. Two unrelated repos
with similar remotes → different IDs (collision probability
~ 1/2^48).

Atomic writes: every save uses `tmp` + `os.Rename`. Concurrent
readers see either old or new, never half-written.

## CLI

| Command | Purpose |
|---|---|
| `sin instinct status` | Show active instincts for the current project + global |
| `sin instinct projects` | List known projects with counts |
| `sin instinct show <id>` | Print one instinct (effective scope) |
| `sin instinct evolve [--apply]` | Cluster eligible instincts into Skill/Command/Agent proposals |
| `sin instinct promote [--apply]` | Move cross-project instincts to global |
| `sin instinct prune [--ttl-days N]` | Delete stale pending instincts and decay the rest |
| `sin instinct export [-o file]` | Write all instincts as JSONL (lossless) |
| `sin instinct import <file>` | Import from a JSONL export |
| `sin instinct forget <id> [--global]` | Delete one instinct |
| `sin instinct history [--limit N]` | Show recent learning events (audit.jsonl tail) |

## Wiring (the closed loop)

Three places in the agent loop the instinct system lives:

1. **Before each tool call:** `instinct.Classify(tool, meta)` decides
   the domain (git, testing, ...). Cheap heuristic, runs inline.
2. **After each tool call:** `obs.Record(observation)` buffers
   `{Tool, Action, Domain, Success, Meta}`. Thread-safe mutex.
3. **End of turn / before context compaction:** `obs.Flush(ctx)`
   runs the configured extractor (Heuristic + optional LLM-backed)
   over the buffered observations and folds each candidate into
   the manager via `Observe` (create) or `Reinforce` (update).

The system-prompt block is built by `Manager.SystemBlockForProject(max)`
which calls `RenderSystemBlock(active, max)`. The rendered block is
appended to the system prompt at the start of every turn.

## Failure modes & guarantees

- **Best-effort, never blocking.** A failed flush, a failed extract,
  a missing memory mirror — all degrade silently. The session
  continues. The audit log will show the gap, the operator can
  investigate.
- **Per-session, per-project.** No cross-user or cross-machine
  leak. Two developers working on the same repo at the same time
  have the same project_id; if they share `$XDG_DATA_HOME` (rare
  but possible on a shared host), they share the instinct set.
- **Reinforce/Contradict math is bounded.** `Reinforce` adds
  `gap-to-max * 0.25`, `Contradict` subtracts `gap-to-floor * 0.40`.
  No infinite loops, no over-confidence. After ~10 reinforces the
  confidence saturates near 0.85.
- **No model training.** The block is prompt-time injection only.
  Switch models, the same instincts apply. The same instincts apply
  to humans using `sin-code -p "..."` in headless mode.

## Related

- AGENTS.md §1 (this repo's source of truth)
- `cmd/sin-code/internal/instinct/types.go` — domain model
- `cmd/sin-code/internal/learning/learner.go` — the bridge to the agent loop
- `cmd/sin-code/internal/hooklife/` — the hook subsystem (PostToolUse feeds the observer)
- `docs/HOOKS.md` — hook event types
- `docs/CI-RUNBOOK.md` — recovery procedures
