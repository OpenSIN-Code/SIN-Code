# RFC: Test-First Verify-Loop for SIN-Code

> Status: Phase 2 Active  
> Author: SIN-Code Agent (research synthesis)  
> Date: 2026-06-17  
> Tracking: #241 (sin_test), #242 (sin_test_generate), #243 (quality gate), #240 (tool.post hooks)

---

## 1. Problem Statement

SIN-Code already enforces a mandatory verification gate (M3) and ships a `sin_test` tool, but the tool is only a **test runner**. It does not generate tests, does not measure coverage, does not run race detection, and does not close the loop from "code written" to "tests verified". This means agent-generated code can pass a superficial `go test ./...` while still containing real bugs, weak oracles, or uncovered edge cases.

The State-of-the-Art in agentic coding (TestForge, SWE-Mutation, OpenHands, AutoCodeRover) shows that the correct pattern is a **closed feedback loop**:

```
Code written → Generate tests → Execute → Collect coverage/mutation/fuzz feedback
      ↑                                              ↓
   Repair ←───── feed failure back to agent ←────── Fail
```

This RFC proposes the architecture, tools, and integration points to add this capability to SIN-Code while respecting the hard mandates (single binary M2, verification gate M3, permission engine M4, race-free M7).

---

## 2. Goals & Non-Goals

### 2.1 Goals

1. Make every code edit automatically testable (via `tool.post` hooks).
2. Extend `sin_test` to run race detection, coverage, and emit structured JSON.
3. Add `sin_test_generate` that produces table-driven Go tests from source files.
4. Provide the foundation for later mutation testing, fuzzing, and property-based testing.
5. Keep all new code inside the single static Go binary; external tools are orchestrated only when present on PATH.

### 2.2 Non-Goals

- This RFC does **not** replace the human verification gate or the stop-gate.
- It does not attempt to bundle `gremlins`, `gotests`, or `ollama` inside the binary.
- It does not change the existing MCP tool names or the public JSON contract.

---

## 3. State-of-the-Art Context

### 3.1 Key Research Findings

| Source | Insight |
|--------|---------|
| **TestForge** (Jain & Le Goues, arXiv:2503.14713) | LLM test generation must be iterative: zero-shot → execute → coverage feedback → re-prompt. Achieves 84.3% pass@1, 44.4% coverage, 33.8% mutation score at ~$0.63/file. |
| **SWE-Mutation** (arXiv:2605.22175) | Even SOTA LLMs generate tests that are gamed by mutated solutions. Mutation testing is the harder gate. |
| **Coverage Isn't Enough** (arXiv:2512.11223) | Auto-generated tests can have high coverage but poor fault-localisation power. |
| **OpenHands** | Event-driven agent loop: every action and observation is an event; test failures are observations returned to the LLM. |
| **AutoCodeRover** | Reproduce-first workflow: run the failing test before editing, then rerun after editing. |
| **Qodo / CodeRabbit** | Commercial review layers that generate missing unit tests and enforce coverage in PRs/IDEs/CLIs. |

### 3.2 Go-Specific Tooling

| Tool | Role | Single-binary compatible? |
|------|------|---------------------------|
| `go test` / `go test -race` | Native runner, race detection | Yes (stdlib) |
| `go test -fuzz` | Native coverage-guided fuzzing | Yes (stdlib) |
| `gotests` | Table-driven test scaffolding | Orchestrated (PATH) |
| `gremlins` | Mutation testing | Orchestrated (PATH) |
| `rapid` / `testing/quick` | Property-based testing | Library / stdlib |
| `staticcheck`, `gosec`, `govulncheck` | Static analysis / security | Orchestrated (PATH) |

---

## 4. Proposed Architecture

### 4.1 High-Level Flow

```
Agent writes code via sin_edit/sin_write
        │
        ▼
┌─────────────────────┐
│  tool.post hook     │  ← auto-triggered after mutating tools
│  sin_test_generate  │
└─────────────────────┘
        │
        ▼
┌─────────────────────┐
│  sin_test           │  ← run tests with -race, -cover, -count=1
└─────────────────────┘
        │
        ▼
┌─────────────────────┐
│  coverage / failure │
│  report             │
└─────────────────────┘
        │
        ▼
┌─────────────────────┐
│  verify.pre hook     │  ← hard gate before task is "done"
│  sin_quality_gate    │
└─────────────────────┘
        │
        ▼
   verify.pass / fail
```

### 4.2 New Internal Packages

```
cmd/sin-code/internal/testgen/
├── generator.go          # Generate test scaffolding via gotests + LLM
├── parser.go             # Parse test results and coverage
└── templates.go          # Test templates for common patterns

cmd/sin-code/internal/testgate/
├── runner.go             # Quality gate pipeline
├── report.go             # Structured JSON report
└── config.go             # Thresholds, timeouts
```

### 4.3 New / Extended Tools

| Tool | Status | Description |
|------|--------|-------------|
| `sin_test` | Extend | Run tests with `-race`, `-cover`, `-count=1`, `-timeout`, and emit structured JSON. |
| `sin_test_generate` | New | Generate table-driven Go tests for a file or package. Uses `gotests` if available; otherwise pure-stdlib scaffolding. Optional LLM fills in test cases. |
| `sin_quality_gate` | ✅ Implemented | Pipeline: build → test → vet → staticcheck → gosec → govulncheck → coverage threshold. |
| `sin_mutation` | ✅ Implemented | Wrap `gremlins unleash` and enforce mutation score threshold. |
| `sin_fuzz` | ✅ Implemented | Generate / run `go test -fuzz` targets. |
| `sin_property` | ✅ Implemented | Generate property-based tests via `rapid` or `testing/quick`. |

---

## 5. Detailed Design

### 5.1 `sin_test` Extension

Current implementation (`cmd/sin-code/chat_tools_extra.go:160`):

```go
cmd = exec.CommandContext(cctx, "go", "test", pkg, "-count=1")
```

Proposed change:

```go
args := []string{"test", pkg, "-count=1", "-race", "-coverprofile=.sin-code/coverage.out", "-covermode=atomic", "-timeout=5m"}
if !raceEnabled { // config or project override
    args = removeArg(args, "-race")
}
cmd = exec.CommandContext(cctx, "go", args...)
```

Return structured JSON:

```json
{
  "status": "PASS",
  "package": "./...",
  "coverage": "82.4%",
  "race_clean": true,
  "output": "...",
  "duration_ms": 1234
}
```

Input schema additions:

```json
{
  "target": "optional package/file filter",
  "race": "true|false (default true)",
  "cover": "true|false (default true)",
  "json": "true|false (default false)",
  "timeout": "e.g. 5m"
}
```

### 5.2 `sin_test_generate` Design

Input schema:

```json
{
  "file": "path/to/file.go",
  "package": "./internal/foo",
  "llm": "true|false (default true)",
  "overwrite": "true|false (default false)"
}
```

Behaviour:

1. If `gotests` is on PATH: run `gotests -all -w <file>`.
2. Parse generated file; find `// TODO: Add test cases.`.
3. If `llm=true` and LLM is available, send the function signature + body to the LLM and ask for realistic test cases.
4. Insert the generated cases into the table-driven test.
5. Run `go test` on the generated test to verify it compiles.
6. Return the generated test file path + compilation result.

Pure-stdlib fallback (if `gotests` is missing):

- Use `go/parser` to read function signatures.
- Generate a minimal table-driven test skeleton.
- LLM fills in cases.

### 5.3 Hook Integration

Example `.sin-code/hooks.yaml` ( Phase 2):

```yaml
hooks:
  - event: "tool.post"
    matcher: "sin_edit|sin_write"
    type: "command"
    command: "sin-code test_generate --auto --target ${SIN_HOOK_DATA_PATH}"
    timeout_seconds: 120

  - event: "verify.pre"
    type: "command"
    command: "sin-code quality_gate --coverage 80"
    timeout_seconds: 300
```

The existing `hooks.go` already supports `tool.post` and `verify.pre` as blockable events. We only need to expose the right data in the hook payload.

### 5.4 Verify-Gate Runner

`verify.go` uses a `Runner` function. We extend `commandRunner` to parse a JSON report:

```go
func commandRunner(cmd string) verify.Runner {
    return func(ctx context.Context, workspace string) (bool, string, error) {
        // run command, parse JSON report
        // return passed, report, err
    }
}
```

This is already possible; the RFC standardises the JSON schema.

---

## 6. Single-Binary & Mandate Compliance

### 6.1 M2 — Single static binary

- New packages are pure Go.
- External tools (`gotests`, `gremlins`, `staticcheck`, `gosec`, `govulncheck`) are **orchestrated**, not vendored.
- The agent checks `exec.LookPath` and degrades gracefully.

### 6.2 M3 — Verification gate

- The new test generation is **input** to the existing gate, not a replacement.
- `verify.pre` quality gate fails closed.
- Stop-gate remains authoritative for completion.

### 6.3 M4 — Permission engine

- `sin_test_generate` is a mutating tool (writes files) and should be `ask` by default.
- `sin_test` is read-only and `allow`.

### 6.4 M7 — Race-free

- All new code must pass `go test -race -count=1 ./cmd/sin-code/internal/testgen/...`.
- No shared mutable state between hook invocations.

---

## 7. Implementation Roadmap

### Phase 1 — Foundation (implemented)

- [x] RFC: Approve this document.
- [x] Extend `sin_test` with race, coverage, JSON, timeout.
- [x] Implement `sin_test_generate` prototype.
- [x] Add unit tests for both tools.
- [ ] Add integration testscript for `sin_test` and `sin_test_generate`.
- [x] Update `AGENTS.md` if behaviour changes.

### Phase 2 — Hook Automation (completed)

- [x] Add `tool.post` auto-trigger for `sin_write`/`sin_edit` (via `SIN_AUTO_GENERATE_TESTS=1`).
- [x] Implement `sin_quality_gate` as verify runner.
- [x] Add `tool.post` hook payload support for `sin_edit`/`sin_write` through `hooks.json` / `hooks.yaml`.
- [x] Add `.sin-code/hooks.yaml` example.

### Phase 3 — Advanced Gates (completed)

- [x] `sin_mutation` (gremlins integration).
- [x] `sin_fuzz` (native Go fuzzing).
- [x] `sin_property` (rapid/testing/quick).
- [x] Coverage + mutation thresholds in config.

### Phase 4 — Golden Dataset (completed)

- [x] Add eval cases for test generation in `evals/`.
- [x] Add mutation, fuzzing, property, and quality-gate datasets.
- [x] Wire into CI via n8n (M1).
- [x] Document dataset format in `evals/README.md`.

### Phase 5 — LLM Case Filling

- [x] Wire `sin_test_generate` to `internal/llm.Client` when `llm=true` or `test.use_llm=true`.
- [x] Implement prompt builder that asks the LLM for realistic test cases from a function signature.
- [x] Add `test.use_llm` config key and `SIN_TEST_GENERATE_USE_LLM` env var.
- [x] Add unit tests for LLM case filling and fallback behaviour.
- [ ] Insert LLM-generated cases into the table-driven test (template splice; Phase 5+).
- [ ] Add generate/execute/repair loop (max 3 retries; Phase 5+).

---

## 8. Success Metrics

| Metric | Target |
|--------|--------|
| `sin_test` JSON output | 100% of invocations parseable |
| `sin_test_generate` compile rate | ≥ 90% of generated tests compile |
| `sin_test_generate` coverage delta | ≥ 20% on previously untested files |
| Race-free tests | all new packages pass `-race` |
| Hook integration | `tool.post` after `sin_edit` triggers generation in < 5s |

---

## 9. Open Questions

1. Should `sin_test_generate` default to LLM=true or false? (Privacy/cost trade-off)
2. Should generated tests be committed automatically or presented as a diff?
3. Which mutation threshold is realistic for a Go codebase of SIN-Code's size?
4. Should we support Python/JS test generation too, or focus on Go first?

---

## 10. References

- TestForge: https://arxiv.org/abs/2503.14713
- SWE-Mutation: https://arxiv.org/abs/2605.22175
- Coverage Isn't Enough: https://arxiv.org/abs/2512.11223
- gotests: https://github.com/cweill/gotests
- gremlins: https://github.com/go-gremlins/gremlins
- OpenHands: https://github.com/All-Hands-AI/OpenHands
- SIN-Code AGENTS.md §3 (M1–M7), §8 (roadmap), §10 (naming rules)
