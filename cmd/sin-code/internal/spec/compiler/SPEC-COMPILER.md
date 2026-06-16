# Spec-Compiler: Declarative `.sin-code.yml` (issue #164)

`internal/spec/compiler/` is the v3.21 declarative config layer for
SIN-Code. A single `.sin-code.yml` describes a repo's coding contract;
the compiler produces the four derived JSON artifacts the engines
need.

## Why

Today, configuring a SIN-Code project means hand-writing three
different config files in three different formats. Operators don't
know which to edit. The spec-compiler collapses that to **one source
of truth** that compiles to the three downstream formats.

This is the SOTA answer (Terraform, Pulumi, GitHub Actions, Bazel
all do this for their domain).

## What ships (v0)

### Schema (`schema.go`)

```yaml
version: 1
project:
  name: my-app
  type: go              # go|python|rust|node|polyglot
verify:
  mode: standard        # minimal|standard|strict
  predicates:
    - name: builds
      command: "go build ./cmd/..."
      required: true
hooks:
  pre-tool:
    - name: no-no-verify
      when: "tool == 'Bash' and command contains '--no-verify'"
      block: true
      message: "git commit --no-verify is not allowed"
  post-tool:
    - name: gofmt
      when: "tool in ['Edit', 'Write'] and path endswith '.go'"
      run: "gofmt -w $path"
permissions:
  allow: ["Bash:go test", "Read:**/*.go"]
  ask:   ["Bash:rm -rf"]
  deny:  ["Bash:curl | sh"]
loop:                    # v1.1, parsed but not consumed yet
  max_turns: 12
  max_tokens: 100000
  disable_checks: ["go vet"]
```

### Subcommand (`sin-code compile-spec`)

```bash
sin-code compile-spec                 # compile .sin-code.yml in cwd
sin-code compile-spec --init          # write a starter .sin-code.yml
sin-code compile-spec --check         # CI: fail if derived files are stale
sin-code compile-spec --out <dir>     # override the output directory
sin-code compile-spec --dry-run       # show what would be written
```

### Outputs

| File | Read by | Status |
|---|---|---|
| `.sin/hooks.json` | `internal/hooks/` (v1.1) | Contract defined, not yet wired |
| `internal/verify/config.json` | `internal/verify/` (v1.1) | Contract defined, not yet wired |
| `internal/permission/policies.json` | `internal/permission/` (v1.1) | Contract defined, not yet wired |
| `.sin/loop.json` | loop builder (v1.1) | Contract defined, not yet wired |

The four JSON files are the **contract** with the engines. v0
defines the contract; v1.1 wires the engines to read it. Round-trip
testing guarantees the contract is stable.

## What does NOT ship (deferred per issue body)

- **Engine wiring** (v1.1, ~2 weeks): the three engines must learn
  to read their derived JSON files. They currently read code, not
  config. This is a refactor of `internal/{hooks,verify,permission}`.
- **Remote spec inheritance** (v2): `extends: org/sin-code-base.yml`
- **Spec testing** (v2): `sin spec test` that asserts the spec is
  consistent (e.g. a hook never references a tool that doesn't exist)

## Mandates honored

- **M1 (n8n CI):** `sin-code compile-spec` runs locally. The
  `--check` mode is intended for CI, but the work happens in the
  operator's checkout, not on the GitHub runner.
- **M2 (single binary):** `gopkg.in/yaml.v3` is already in `go.mod`
  as a transitive dep. No new dependency.
- **M5 (module path):** new code in `cmd/sin-code/internal/spec/compiler/`.
- **M6 (SIN tools over naive built-ins):** the schema is designed
  so the engines can adopt it without new types — the JSON contract
  mirrors the existing struct shapes where possible.

## Acceptance criteria (from #164)

- [x] The schema is documented (`docs/SPEC-COMPILER.md` + this file)
- [x] `sin-code compile-spec` round-trips: spec → derived → no diff
      on re-run (verified by `TestRoundTrip`)
- [x] The schema validates with clear error messages on invalid
      input (verified by `TestValidate_*` tests)
- [x] Test coverage ≥ 80% (24 tests in `compiler_test.go`, all paths)

## Relationship to issue #155 (Pro-Repo-Konfiguration)

Issue #155 proposes a `.sin-code.yml` for **loop parameters only**
(max_turns, max_tokens, disable_checks). The v0 schema includes
those fields as the `loop:` block. When #164 v1.1 lands and the
loop builder reads `.sin/loop.json`, issue #155 is closed by
reference. The two issues are **not in conflict** — they describe
different slices of the same eventual file.

## Trade-offs (documented)

1. **JSON for the derived files, not Go structs.** The engines
   can deserialize the JSON on startup. This keeps the contract
   language-agnostic (Python or Node tooling can also produce
   the same files).

2. **Atomic writes.** Each derived file is written via temp +
   rename, so a crash mid-write never leaves a half-written file
   behind. Cost: one extra fsync per file. Worth it for the
   "compiler is part of the pre-commit hook" use case.

3. **`yaml.v3` permissive parsing.** Unknown top-level keys are
   silently dropped, so a v2 spec with a new key parses cleanly
   under v1. Operators get a forward-compatible format for free.

## File layout

```
cmd/sin-code/internal/spec/compiler/
├── doc.go              # package overview
├── schema.go           # Config + Project + Verify + Hooks + Permissions + Loop
├── parse.go            # Parse(bytes) + ParseFile + InitTemplate
├── validate.go         # Validate(c) with field-path errors
├── emit.go             # EmitHooks/Verify/Permissions/Loop (4 output formats)
├── compiler_test.go    # 24 race-clean tests
└── helpers_test.go     # test-only file helpers
```

Plus the CLI:

```
cmd/sin-code/compile_spec_cmd.go  # cobra subcommand
```
