# stack — Unified SIN-Code Stack Orchestrator

> **Package:** `github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/stack`
> **Since:** v3.8.0
> **Layer:** Glue / orchestration (no domain logic of its own)
> **Status:** stable

## What this package IS

`stack` is the **single entry point** for "install the whole SIN-Code
toolchain" and "tell me whether the toolchain is healthy". It composes
the three independent layers that ship with v3.8.0 and exposes two
ergonomic verbs:

- `Install(opts) Report` — idempotent installer that wires up
  superpowers (methodology), dox (context hierarchy), and vane
  (research / runtime) into a coherent AGENTS.md + MCP config bundle.
- `Doctor(root) Report` — read-only health check that surfaces
  per-layer status in a single greppable view.

This package does NOT contain domain logic. Every layer delegates to
the corresponding `internal/<layer>/` package and records success or
failure in a `Component` row. The only value-add is:

1. **Failure isolation** — one layer being broken does not abort the
   whole install (graceful degradation).
2. **Stable report format** — `Format(r)` produces the canonical
   ✓/✗/- tree shared with the rest of the SIN-Code CLI.
3. **Idempotency contract** — running `Install` twice produces the
   same on-disk state as running it once (marker-based, verified by
   `TestInstallDoxAndVaneOnly`).

## Architecture

```
                    stack.Install / stack.Doctor
                                │
            ┌───────────────────┼────────────────────┐
            ▼                   ▼                    ▼
    superpowers.Install   dox.InjectRoot       vane.SaveConfig
            │                   │                    │
            ▼                   ▼                    ▼
    List / CurrentPin     Check / Build         RegisterMCP
            │                   │                    │
            └─────────► Report ◄┴──────────────┘
                          │
                          ▼
                     Format(r)
                       ✓ / ✗ / -
```

### Component lifecycle

| Field | Type | Meaning |
|---|---|---|
| `Name` | string | Stable identifier (e.g. `"superpowers.install"`, `"vane.health"`) |
| `Layer` | string | One of `"superpowers"`, `"dox"`, `"vane"` |
| `OK` | bool | `true` = healthy / `false` = real failure |
| `Skipped` | bool | `true` = caller asked to skip, never a failure |
| `Detail` | string | Human-readable, machine-greppable status (SHA, URL, error) |

`Report.AllOK` is computed as: every non-Skipped component has
`OK == true`. A skipped layer never drags the overall down.

## Public API

### `Install`

```go
type InstallOptions struct {
    SkipSuperpowers, SkipDox, SkipVane bool
    AgentsMDPath, VaneURL              string
    RepoURL, Branch                    string  // superpowers
    Timeout                            time.Duration
}

func Install(opts InstallOptions) Report
```

Order of operations:

1. **superpowers** — `Install` (clone or pull) → `RegisterMCP` → if
   `AgentsMDPath` exists, `InjectAGENTS` (overlay). Any failure is
   recorded; the rest of the install continues.
2. **dox** — `MkdirAll` on the agents-file parent dir, then
   `dox.InjectRoot` with a stack-managed body. Idempotent: a second
   call with the same body produces a byte-identical file.
3. **vane** — `LoadConfig` → optionally override URL from opts →
   `SaveConfig` → `RegisterMCP`. Configuration failures are fatal
   for the layer; instance reachability is NOT checked at install
   time (that's Doctor's job).

### `Doctor`

```go
func Doctor(root string) Report
```

Read-only. Walks every layer and reports:

| Component | Checks |
|---|---|
| `superpowers` | `List` non-empty + `CurrentPin` non-nil |
| `dox` | `Check` on `root` returns 0 error-severity findings |
| `vane.config` | `LoadConfig` has non-empty URL |
| `vane.health` | `NewClient(cfg).Healthy()` — DOWN is **informational, not fatal** |

**Graceful degradation contract:** a configured-but-down vane
instance is reported as `OK=true` with a `"DOWN"` detail. The user
gets an accurate picture without false alarms when their vane
container is offline.

### `Format`

```go
func Format(r Report) string
```

Renders a Report as a multi-line string with one row per
`Component` and the ✓/✗/- markers. The output is stable across
versions and safe to grep / log-scrape.

## Dependencies

| Package | Why |
|---|---|
| `internal/dox` | AGENTS.md injection, structural Check |
| `internal/superpowers` | skill clone, pin, MCP registration, AGENTS overlay |
| `internal/vane` | config load/save, MCP registration, health check |
| `context`, `errors`, `fmt`, `os`, `path/filepath`, `strings`, `time` | stdlib only |

CGo-free. Stdlib-only. Single-binary safe (mandate M2). Module path
`github.com/OpenSIN-Code/SIN-Code` (mandate M5).

## Idempotency guarantees

| Operation | Idempotent? | Mechanism |
|---|---|---|
| `superpowers.Install` | ✅ | git fetch + reset --hard FETCH_HEAD |
| `superpowers.RegisterMCP` | ✅ | checks for existing entry with same command |
| `superpowers.InjectAGENTS` | ✅ | `<!-- SIN-Code superpowers:begin -->` marker block replace |
| `dox.InjectRoot` | ✅ | `<!-- SIN-Code dox:begin -->` marker block replace |
| `vane.SaveConfig` | ⚠️ best-effort | depends on vane's own implementation |
| `vane.RegisterMCP` | ⚠️ best-effort | depends on vane's own implementation |

`TestInstallDoxAndVaneOnly` asserts that two consecutive `Install`
calls leave exactly **one** `dox:begin` marker in AGENTS.md.

## Race-safety

Every test in `stack_test.go` uses `t.TempDir()` + `t.Setenv` to
redirect `$SIN_CODE_HOME` and the working directory. Tests do not
spawn goroutines, do not share files across tests, and do not depend
on wall-clock time. The package is safe under `go test -race`.

## Tests

```bash
go test ./cmd/sin-code/internal/stack/ -race -count=1 -cover
```

| Test | What it asserts |
|---|---|
| `TestInstallDoxAndVaneOnly` | Idempotency: 2× Install = 1× dox marker |
| `TestDoctorReportsMissingLayers` | Empty install → `AllOK=false` |
| `TestDoctorVaneDownIsNotFatal` | vane DOWN → OK=true, detail contains DOWN |
| `TestFormatOutput` | All three markers (✓ ✗ -) present in output |
| `TestShortSHA` | Boundary cases for SHA truncation |

## Failure isolation example

```go
opts := InstallOptions{ /* SkipSuperpowers: true */ }
rep := Install(opts)
// rep.Components[0]: superpowers — OK=true, Skipped=true
// rep.Components[1]: dox         — OK=true
// rep.Components[2]: vane        — OK=false (network down)
// rep.AllOK                      — false
// But: AGENTS.md IS valid (dox layer succeeded).
// Stack never aborts mid-flight.
```

## Related

- `internal/superpowers` — methodology skills
- `internal/dox` — AGENTS.md hierarchy + validation
- `internal/vane` — research / runtime client
- AGENTS.md §3 (mandates M2/M5/M7), §10 (naming)

