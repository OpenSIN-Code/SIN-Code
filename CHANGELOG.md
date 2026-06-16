# Changelog

All notable changes to the SIN-Code unified binary will be documented in this file.

## [Unreleased] - 2026-06-16

### Added — Shop-Center skill integration (issue #142 fusion)
- **`KnownSkills()` registry extended** with the three shop skills
  (issue #142 acceptance criterion #2 — installable via
  `sin-code skill install <name>`):
  - `shop-cj-dropshipping` → `cj-dropshipping-skill`
  - `shop-stripe`          → `SIN-Stripe-Bundle`
  - `shop-tiktok`          → `SIN-eCommerce-Scraper-Bundle`
- **Bundled `SKILL.md` files** (cj-dropshipping, stripe, tiktok)
  already carry `lifecycle: external` + `sources:` frontmatter
  (added by PR #218 / issue #139). The `sources:` field points
  to the canonical external repos so the operator can discover
  the upstream implementation directly from the bundled skill.
- **New test** `TestKnownSkillsHasShopEntries` in
  `cmd/sin-code/internal/skillmgr/manager_test.go` asserts the
  three entries are present with the correct repo mapping.
- **Validation passes** — `validate_skill.py --all-bundled
  --strict` reports `0 failed` for the 34 skills (the 3 shop
  skills were migrated by PR #218).
- **Long-term fusion strategy** documented in the issue body:
  phase 1 (external canonical) → phase 2 (bundled doc, done) →
  phase 3 (native subcommand) → phase 4 (deprecate upstream).
  Phase 3 is deferred until the shop domain matures.

### Added — `sin-code grill` (issue #141 fusion, native implementation)
- **New package** `cmd/sin-code/internal/grill/` with 4 source
  files (`types.go`, `catalog.go`, `manager.go`, `grill_test.go`)
  + 14 race-clean unit tests. The native Go implementation of
  the external `SIN-Code-Grill-Me-Skill` Python MCP server
  (38 KB). Ships in v0 with JSON-file storage; SQLite session
  storage is a v1 follow-up.
- **New subcommand** `sin-code grill` (5 subcommands):
  - `grill start <topic>` — begin a grilling session, print the id
  - `grill next <id>` — ask the next adversarial question
  - `grill answer <id> <d-id> <text>` — record the response
    (use "done" to resolve a decision)
  - `grill status <id>` — show resolved + open decisions
  - `grill synthesize <id> [--json]` — produce a structured
    summary of decisions, assumptions, and open questions
- **Question catalog** — 8 seed anti-patterns (Hidden
  Assumptions, Rollback Plan, Failure Modes, Operator Cost,
  Premature Optimization, Scope Creep, Single Point of Failure,
  Verification Gap). Each has 2-3 example questions. The CLI
  picks one per `grill next` call (hash-seeded for determinism).
- **Storage** — `$SIN_CODE_HOME/grill/<id>.json`, atomic writes
  (temp + rename). v1 will migrate to SQLite via the existing
  `internal/session/store`.
- **14 race-clean tests** covering the full flow (Start → Next →
  Answer → Synthesize → round-trip across restarts).

### Added — `sin-code goal` fusion (issue #140, v0.5)
- **Four new subcommands** under `sin-code goal` (issue #140 fusion
  with the external `SIN-Code-Goal-Mode-Skill` Python MCP server):
  - `goal status <id>` — show one goal with subtasks (children)
  - `goal complete <id>` — mark a goal as verified/done
  - `goal subtask <parent-id> <prompt>` — add a subtask
  - `goal report [--format md|json]` — progress report
- **Mapping to the 8 external tools** (issue body):
  - `goal_start` → existing `goal add`
  - `goal_status` → NEW `goal status`
  - `goal_list` → existing `goal list`
  - `goal_complete` → NEW `goal complete`
  - `goal_subtask` → NEW `goal subtask`
  - `goal_report` → NEW `goal report`
  - `goal_checkpoint` / `goal_rollback` — **deferred to v1** (no
    storage yet; the Queue has no `Checkpoint` table)
- **`parseGoalID` helper** — accepts both `42` and `#42` (with
  optional whitespace); used by all four new subcommands
- **11 tests** (1 unit test for `parseGoalID`, all autonomy
  tests pass under `go test -race -count=1`)

### Added — Skill lifecycle markers (issue #139)
- **`scripts/lifecycle_map.yaml`** — single source of truth for the
  lifecycle of every bundled skill. Maps each of the 34 skills to
  one of `native | external | deprecated` with a `canonical:` field
  pointing to the upstream implementation.
- **`scripts/sync_lifecycle.py`** — stdlib-only Python script. Three
  modes: `--check` (CI: exit 1 if any drift), `--apply` (write
  changes), `--diff` (show what would change). Hand-rolled YAML
  parser for the map file (no PyYAML dep).
- **`scripts/validate_skill.py` strict mode** — now requires the
  `lifecycle` frontmatter key in `--strict` mode and validates the
  value. Non-strict mode remains backward-compatible.
- **`sin-code skill list`** now prints a `LIFECYCLE` column with
  `[native]`, `[external]`, `[deprecated]`, or `[unknown]` markers.
  A new `parseLifecycleFromFrontmatter` helper extracts the field
  from the embedded SKILL.md without a yaml dep.
- **`docs/SKILLS.md`** — design doc with the value table, the
  workflow, and the migration path.
- **All 34 SKILL.md files migrated** — 28 skills received the
  `lifecycle:` field; 6 were already in sync. Total: 34/34.

### Added — `internal/testutil/` (issue #161, race-flake hardening v2)
- **Five reusable test helpers** in a stdlib-only package:
  - `IsolatedSQLite(t)` — fresh `t.TempDir()`-backed `*sql.DB`, auto-closed
  - `CleanEnv(t, kv)` — set + restore env vars via `t.Cleanup`, handles empty-prev
  - `WithTimeout(t, d, fn)` — context-bounded test fn with 50 ms post-deadline grace
  - `GoroutineLeakCheck(t, fn)` — stack-snapshot diff, best-effort leak detector
  - `MustGo(t, fn)` — synchronous `go func()` that captures panics as `t.Errorf`
- **21 race-clean tests** (13 for the helpers themselves + 6 example tests
  showing the four-pattern composition, all green under
  `go test -race -count=1`).
- **`testutil.doc.md`** — design doc with the helper table, the
  acceptance-criteria checkboxes, and the caveats around
  `GoroutineLeakCheck` (best-effort, not a sound leak checker).
- **Diagnosis pass** (informational): ran
  `go test -count=1 -v ./...` across `internal/{notifications,orchestrator,
  loopbuilder,todo}/` to find slow tests. The slowest is
  `TestGenerateIDUniqueness` at 3.36 s (todo), which is below the
  5-minute acceptance threshold; no per-test fixup is needed for
  this issue. The diagnosis methodology is in the runbook below.

### Added — `sin-code triage` (issue #162)
- **`cmd/sin-code/triage_cmd.go`** — new 41st subcommand `sin-code triage`
  (and `triage --format=md|json --repo owner/repo --limit N`). Reads the
  open issue backlog via `gh issue list` through the `ghbridge` wrapper,
  scores each issue with a deterministic heuristic (epic +10, blocks +5
  per ref, acceptance +3, not-in-v0 +5, loop-system +4, fusion +2,
  fresh -2, stale +1, good-first-issue -3), groups by label bucket, and
  renders. The markdown output is the canonical `BACKLOG.md` generator.
- **`cmd/sin-code/internal/triage/`** — new package, four files
  (`types.go`, `score.go`, `render.go`, `loader.go`) plus
  `triage_test.go` (15 tests, all green under `-race -count=1`). The
  loader is a `var` so tests inject fixtures without spawning `gh`. No
  new third-party deps (M2).
- **`triage.doc.md`** — design doc with the scoring table, the
  per-bucket ordering rule, and the deferred-items list.

### Added — `sin-code catalog` (issue #163, hub-assets merge)
- **`cmd/sin-code/catalog_cmd.go`** — new subcommand `sin-code catalog`
  (`list | search | info`) with `--kind=agent|command|skill|hub` and
  `--format=text|json`. The unified tool catalog that operators have
  been asking for: "do I have a tool for this?" — not "do I want the
  hub or the assets?".
- **`cmd/sin-code/internal/catalog/`** — new package, 4 source files
  (`catalog.go`, `source_hub.go`, `source_assets.go`, `catalog_test.go`)
  + 21 race-clean unit tests. The `Source` interface (Name + List +
  Get) is the abstraction that lets the catalog walk both backends.
  Adding a new source (e.g. a remote registry) is one file.
- **Merge de-duplication rule** — first source to provide a
  `(kind, name)` pair wins; subsequent duplicates are dropped. The
  source name is intentionally not part of the dedup key, so a
  hub.Tool and an assets.Asset with the same name are merged into
  one catalog entry (the SOTA choice for the operator's mental model).
- **Search ranking** — name +4, short +2, description +1, tag +1;
  ties break by name ascending. Transparent, auditable, deterministic.
- **`catalog.doc.md`** — design doc with the de-dup table, the
  scoring heuristic, the deprecation plan for `sin-code hub`, and
  the known build issue (Chromedp API mismatch in PR #201, not
  in this PR).

### Added — `internal/rag/` (issue #160, RAG over instinct store)
- **New package** `cmd/sin-code/internal/rag/` with 4 source files
  (`embedder.go`, `embedder_hash.go`, `embedder_onnx.go`,
  `index.go`, `worker.go`, `retriever.go`) + 24 race-clean tests.
  - `Embedder` interface + `HashEmbedder` (default, deterministic,
    dependency-free, 384-dim L2-normalized) + ONNXRuntimeEmbedder
    and HTTPEmbedder as documented stubs.
  - `Index` with optional `Persister` interface; the instinct
    subsystem uses a `jsonPersister` writing to
    `$SIN_CODE_HOME/instinct-embeddings.json`.
  - `WorkerPool` (bounded-concurrency, M7) for async embedding
    so the agent loop never blocks.
  - `Retriever` (high-level: Embedder + Index → top-N IDs).
- **`sin instinct search "<query>"`** — top-5 cosine-similarity
  search over the active instincts. Reindexes on every call
  (cheap at <100 active) and persists to disk. Renders the hits
  as `id — trigger` lines with the action underneath.
- **8 race-clean tests** in `internal/instinct/search_test.go`
  for the JSON persister round-trip, the path-overriding
  env var, the atomic-write behavior, and the trim helper.
- **`rag.doc.md`** — design doc with the mandate-compliance
  analysis, the Embedder interface, the acceptance-criteria
  checkboxes, and the deferred-items list (GOAP Planner,
  Federation, real ONNX implementation).

### Added — `sin-code compile-spec` (issue #164, v0 spike)
- **`cmd/sin-code/internal/spec/compiler/`** — new package, 4 source
  files (`schema.go`, `parse.go`, `validate.go`, `emit.go`) + 24
  race-clean unit tests. Round-trip test (`TestRoundTrip`) is the
  load-bearing guarantee: parse → emit → parse → emit must produce
  identical bytes.
- **`cmd/sin-code/compile_spec_cmd.go`** — new subcommand
  `sin-code compile-spec` with `--init`, `--check`, `--out <dir>`,
  `--dry-run` flags. Atomic writes (temp + rename) so a crash
  mid-write never leaves a half-written file behind.
- **Four derived JSON outputs** (contract defined; engines not
  yet wired — that is v1.1):
  - `.sin/hooks.json`
  - `internal/verify/config.json`
  - `internal/permission/policies.json`
  - `.sin/loop.json` (parsed but not consumed — migration path
    for issue #155)
- **`SPEC-COMPILER.md`** — design doc with the schema, the
  mandate-compliance analysis, the deferred-items list (engine
  wiring, remote spec inheritance, spec testing), and the
  relationship to issue #155.

### Added — `sin-code install` + one-line curl|bash installer (issue #170)
- **`cmd/sin-code/install_cmd.go`** — new 40th subcommand `sin-code install`
  (and `install --auto`). Downloads the latest GitHub release asset,
  SHA256-verifies against the goreleaser-style `checksums.txt`,
  extracts the single binary, atomically places it into
  `$SIN_CODE_BIN_DIR` or `$HOME/.local/bin`, and prints the canonical
  PATH hint. Flags: `--dir`, `--release <tag>` (pin a version),
  `--channel stable|dev`, `--verify-only` (health-check, no write),
  `--no-verify` (offline / sanctioned CI), `--dry-run`.
- **`cmd/sin-code/internal/install/`** — new pure-stdlib package.
  Four tiny files (`release.go`, `github.go`, `verify.go`, `composer.go`)
  plus race-safe `install_test.go` (19 tests, all green under
  `-race -count=1`). The bootstrap never depends on `gh` or `jq`,
  making the install cmd safe to run on a freshly imaged host.
- **Root `install.sh`** — rewritten from 1031 lines to **27 lines**
  shell-only shim. `curl -fsSL ... | bash` compatible. Settles the
  downloaded archive, extracts the `sin-code` binary via three
  tolerant glob shapes (works across goreleaser versions), then
  `exec`s `sin-code install --auto` so the Go entrypoint owns the
  verify-and-place flow. Legacy 12-step logic permanently retired —
  post-v3.0 the unified `sin-code` binary already subsumes the 7
  Go tool subcommands the old installer built.
- **Root `install.ps1`** — new 35-line Windows equivalent
  (`irm https://raw.githubusercontent.com/.../install.ps1 | iex`).
  Uses `Invoke-WebRequest` + `System.IO.Compression.ZipFile`, then
  re-execs `sin-code.exe install --auto`.
- **`permission_defaults.go`** — three new MCP rules under the
  `install__*` prefix (mirror of the `gh_execute` precedent in §3
  M4): `install__verify_only` allow, `install__dry_run` allow,
  `install__run` ask. The headless daemon therefore CANNOT
  self-install silently, satisfying M4's "always headless" clause.
- **`AGENTS.md` §6** — repo layout now lists `install.sh`,
  `install.ps1`, `cmd/sin-code/install_cmd.go`, and the new
  `internal/install/` package alongside the existing 39 subcommands.

### Added — Verbosity / compression mode (issue #167)
- **`internal/style/`** — first-class verbosity mode system-prompt
  renderer. Five canonical modes (`default`, `verbose`, `normal`,
  `terse`, `ultra`) with byte-stable output per `(mode, skillBody)`
  pair (prerequisite for the system-prompt hash metric, issue #2).
  `default` and `verbose` pass through skill bodies unchanged;
  `normal` drops pleasantries + tool-call narration, `terse` drops
  articles/hedging, `ultra` is the tightest valid compression.
  Every non-default ruleset carries the **auto-clarity** clause that
  forces normal prose around destructive, security-relevant, or
  order-sensitive actions — the verification gate (mandate M3) must
  never be skipped because the output mode is terse. API surface:
  `ParseMode(s)`, `RenderRules(mode, body)`, `RenderSystemBlock(level)`,
  `AppendVerbosity(existing, mode)`, `WithVerbosity(mode)` (functional
  option).
- **`instinct.RenderSystemBlockWithVerbosity`** (issue #167): the
  instinct renderer now accepts a verbosity-mode string and appends
  the matching ruleset after the learned-instinct list (stable order:
  instincts → style, separated by exactly one blank line). Backward-
  compatible — the legacy `RenderSystemBlock(active, max)` still
  returns the bare instinct block.
- **`learning.Learner` style hook**: `BeforeTurn` now honors
  `Options.Style` and routes it through the new renderer. New
  `SetStyle(level)` method lets mid-session callers toggle verbosity
  safely under a per-instance `sync.RWMutex` (mandate M7, race-free).
  `wiring.Deps.Style` passes through.
- **`llm.style` config key** (`internal/config.go`): user-facing knob
  with full get/set/list/validate/TOML/JSON coverage. Validated
  against the canonical mode set. `sin-code config set llm.style terse`
  works end-to-end.
- **Reference docs**: `internal/style/style.doc.md` (developer),
  `cmd/sin-code/internal/config.doc.md` updated (table + example),
  `cmd/sin-code/internal/learning/learner.doc.md` updated (Style
  field), `AGENTS.md` §6 + §7 cross-references.
### Added — `sin-code compress` (issue #172, deterministic + LLM compaction)
- **`internal/compress/`** — first-party package implementing the
  caveman-compress pattern (`JuliusBrussee/caveman`) for SIN-Code's
  long-lived stores. Public API: `BuildPlan`, `Apply`, `Rollback`;
  three strategies (`deterministic` default, `llm`, `hybrid`)
  targeting four surfaces (`lessons`, `instincts`, `summaries`,
  `memory`, `agents_md`) plus an aggregate `all`. Plan is read-only
  and content-addressed (`PlanHash` covers Entries+Drops+Merges);
  Apply is atomic (snapshot written to `.partial` then renamed before
  any source rewrite) and lossless (dropped entries are preserved
  verbatim under `~/.local/share/sin-code/compress-snapshots/<plan-id>.json`).
- **Deterministic pass** (`compressor.go` + `deterministic.go`):
  SHA-256 dedupe + utility-sorted (recency × inverse-size) keep-recent
  with byte-budget cap. Algorithm pins `time.Now()` for tests via
  `PlanOptions.UseStableTime` so two plans built from identical inputs
  agree byte-for-byte regardless of wall clock. Stable-time pins are
  verified by `TestPlanDeterministicIdempotent`.
- **LLM summarization pass** (`llm.go`): caveman-style compress prompt
  that preserves code fences, URLs, file paths, commands, and headings
  byte-for-byte. Validates the response with a line-based check;
  retries up to 2 with a targeted patch prompt on validation failure
  (`MaxRetries`, configurable). Falls back to deterministic on
  exhausted retries or when no provider is configured.
- **Atomic snapshot+rollback**: Apply writes a `.partial` snapshot
  first; `Rollback(<plan-id>)` reads the snapshot, refuses to consume
  any in-flight `.partial`, and restores the originals via per-target
  re-apply. `TestApplyIsAtomicAndLossless` covers one round-trip.
- **CLI surface** (`compress_cmd.go`, 41st subcommand = 40th
  registered cobra verb because `internal.InstinctCmd` etc. are
  in-package additions):
  - `sin-code compress plan [--target all] [--strategy deterministic]
     [--keep-bytes 4096] [--keep N] [--recent-days N] [--json]`
  - `sin-code compress apply [--dry-run] [--no-llm] [--target ...]
     [--strategy ...] [--keep-bytes ...] [--json]`
  - `sin-code compress rollback <snapshot-id>`
- **Permission policy** (`permission_defaults.go`):
  `{Tool: "compress__plan", Policy: "allow"}`,
  `{Tool: "compress__apply", Policy: "ask"}` (M4 — destructive),
  `{Tool: "compress__rollback", Policy: "allow"}` (restorative only).
  Wired so any future agent-loop surface that exposes the compressor
  via MCP is gated correctly.
- **Regression tests** (`compress_test.go` + `testhelpers_test.go`):
  hash determinism, plan idempotence, byte-budget enforcement, dedupe
  invariance, atomic-write contract, dry-run no-touching-nothing,
  partial-marker rollback refusal, preservation line scoping (heading,
  code fence, URL, file path, command line), snapshot JSON round-trip.
  Passes `go test -race -count=1 ./cmd/sin-code/internal/compress/`.
- **Snapshot dir**: `~/.local/share/sin-code/compress-snapshots/`
  (overridable via `SIN_CODE_SNAPSHOT_DIR`). Same form factor as
  lessons.db / ledger.db per AGENTS.md §7.
### Added — Per-agent profile renderer (issue #175)
- **Single source of truth** at `docs/agent-profiles/sin-profile.md`
  (≤80 lines, KISS, hard mandates + working style + subagent contracts +
  per-agent notes, edits roll out everywhere).
- **`internal/profile`** package: targets map mirroring AGENTS.md §10
  (`claude-code`, `opencode`, `gemini`, `codex`, `cursor`, `windsurf`,
  `cline`, `copilot`); per-format writers (`dir`, `rule`, `marker`); a
  byte-stable `Render(tgt, body)`; a `Verify(base, body)` SHA-256
  drift gate; idempotent marker-fence envelopes byte-identical to
  `internal/skilldist` (issue #169 covenants preserved).
- **`sin-code profile` subcommand** with four verbs:
  - `profile show`               — print the source markdown
  - `profile list`               — print the supported target table (text + `--json`)
  - `profile render <target|all>` — write one or all mirrors (idempotent;
    supports `--dry-run` for byte/audit preview without touching disk)
  - `profile verify`             — CI gate: refuse on missing/drift;
    surfaces a 12-char-row table or a JSON envelope for `--json`
- **Permission engine**: `profile__show` / `list` / `verify` are
  registered as `allow`; `profile__render` is `ask` because it
  touches per-agent dotdirs (mandate M4).
- **CI sync**: `.github/workflows/ceo-audit.yml` grew a parallel
  `profile-verify` job that builds `sin-code`, runs
  `profile render all && profile verify`, uploads the rendered
  mirrors as artifacts, and fails the build if any drift surfaces.
  Mirrors AGENTS.md §6 + §10 — single source of truth in
  `internal/profile/target.go`.
- **Test coverage**: 22 race-tested Go tests pinning the byte-stable
  contract (golden render, marker-fence idempotency, marker-Fence
  covenant, `Verify` pass / missing / drift, write-after-write SHA
  equality, replace-not-append for stale mirrors).
### Added — sin-debt marker convention (issue #177)
SIN-Code adopts ponytail v4.7.0's `ponytail:` marker convention as a
first-class, parseable `// sin-debt: <ceiling>, upgrade: <trigger>`
convention. Every intentional shortcut now carries a marker naming its
ceiling and the trigger to revisit; the scanner reads them; `debt stats`
reports them; `debt check` gates them.

- **`internal/sindept/`** — scanner, aggregator, byte-stable report
  renderer, and policy gate. Single-package surface:
  - `parser.go`  — `Marker{File, Line, Column, Reason, Upgrade, HasUpg,
    Raw, Language, Symbol}`, regex over five comment families
    (`//`, `#`, `--`, `/*…*/`, `<!--…-->`), `ParseFile` + `ParseDir`.
    Trims trailing block-comment closers (`*/`, `-->`) and post-processes
    captured clauses for byte-determinism.
  - `stats.go`   — `Stats{Total, WithUpgrade, WithoutUpgrade, ByFile,
    ByReason, ByLanguage, BySymbol, RotRisk, Oldest, MarkersPerFile}`.
    Every map is materialized as a lex-sorted `[]KV` so `Render*`
    output is stable.
  - `report.go`  — `RenderStats` / `RenderStatsString` /
    `RenderListString` markdown renders with `FormatVersion =
    "sin-debt/v1"`. Two scans of the same tree emit the same bytes.
  - `policy.go`  — `Policy{DefaultReasons, UpgradeTriggers,
    MaxNoUpgrade, RequireUpgrade, Source}`. TOML overlay via
    `.sin-code/debt-policy.toml`; `LoadPolicyForRoot` walks upward to
    the closest file.
- **`debt_cmd.go`** — 41st subcommand. `sin-code debt list | stats |
  check | policy | fix | export`. Common flags: `--path`, `--format`
  (`table|json`), `--no-trigger`. Stats sub-commands take `--by
  file|reason|language|symbol|age|summary`. The `check` sub-command
  is the CI gate; it exits non-zero when `Missing > MaxNoUpgrade` or
  `RequireUpgrade=true && Missing > 0`.
- **`docs/sin-debt-convention.md`** — author-facing reference:
  format, examples, default reasons catalogue, default upgrade
  triggers.
- **Permitted `sindept__*` tools**: `sindept__list`, `sindept__stats`,
  `sindept__policy` are read-only `allow`; `sindept__check` exiting
  non-zero is `ask`; `sindept__fix` and `sindept__export` are
  `ask` because they instruct humans to edit code or write a file.
- **10 hard-coded fixture markers** under
  `cmd/sin-code/internal/sindept/testdata/` (5 languages) and 4 *real*
  markers placed in production code (`cmd/sin-code/internal/lessons/
  store.go` × 2, `…/ledger/store.go`, `…/orchestrator/dispatcher.go`).
- **Tests**: 23 tests in `cmd/sin-code/internal/sindept/sindept_test.go`
  cover family coverage, trailing-closer stripping, byte-stability,
  vendor / hidden-dir walk, age/rot grouping, and the
  policy-gate semantics above vs. below threshold. Race-clean.

### Notes
- `sin-code debt stats` is the precondition for the four-arm
  comparator snapshot (issue #171); byte-stable today, golden file is
  expected in the next PR cycle.
- The marker syntax deliberately does NOT include `\Q…\E` quoting —
  RE2 has no such construct, and the literal token `sin-debt:` is
  plain inside the regex.
- `internal/sindept` is the upstream of issue #179 (complexity auditor)
  and issue #180 (audit-engine): both are expected to call
  `sindept.ParseFile` / `sindept.AggregateStats` so a marker reads
  through the same shape regardless of consumer.

---

### Added — Loop Engineering (decoupled completion authority)
### Added — MCP tool-manifest compression (issue #173, v3.19.0)
- **`internal/mcpcompress/`** — ponytail-tag compressor for
  `sin-code serve --compress-tools` (issue #173). Five canonical tags
  drive five byte-stable Rules:
  - `delete` → `DeleteHedges` drops pleasantries / hedge adverbs
    ("safely", "carefully", …).
  - `stdlib` → `StdlibPatterns` drops redundant stdlib
    parentheticals ("(via stdlib)", `Go stdlib …`).
  - `native` → `DropTrimEncouragement` drops M6 tail clauses
    `Always prefer over native X` / `Prefer sin_X over native Y`
    (the M6 mandate is internal-only, not for the model).
  - `yagni` → `YagniPatterns` drops speculative
    `(experimental)` / `(TBD)` / `(reserved)` parentheticals.
  - `shrink` → `ShrinkExamples` drops redundant
    `(e.g. …)` / `(such as …)` parenthetical examples.
- **Three new `serve` flags**:
  - `--compress-tools` — apply the full default ponytail tag set
    (`delete|stdlib|native|yagni|shrink`) before registration.
  - `--compress-tags <csv>` — override the active tag set
    (e.g. `--compress-tags "delete,yagni,shrink"`). Unknown tags
    are silently dropped; the active set is logged via `--print-stats`.
  - `--print-stats` — emit a left-aligned text table to stderr
    (tool / orig / comp / saved / ratio + TOTAL row + active rules).
- **Tool names unchanged.** The compressor mutates only `Description`;
  the 47 MCP tool `Name` fields (public API per AGENTS.md §10)
  are never modified. `TestCompressSpec_NameMutable` guards this.
- **Byte-stable per `(tool_spec, ruleset)`** — every Rule, the
  Pipeline, and the post-pipeline `Normalize` are deterministic and
  idempotent. The `compressor_test.go` golden suite is the single
  source of truth — any Rule regex / declaration-order change must
  update the gold expectations in the same PR. Prerequisite for the
  system-prompt hash metric (issue #2).
- **Real-world savings.** Smoke test against the 47-tool registry:
  `sin_execute` 80→73 bytes (-7, 8.8%); `sin_read` 200→167 bytes
  (-33, 16.5%); `sin_write` 194→160 bytes (-34, 17.5%);
  `sin_edit` 474→441 bytes (-33, 7.0%). TOTAL 3977→3870 bytes
  saved across 4 affected tools (-107 bytes / 2.7%); the other 43
  tools' descriptions don't match any of the conservative patterns
  and stay byte-identical. Per-tool `--print-stats` output is
  deterministic across runs (no time, no random).

### Added — Loop Engineering (decoupled completion authority)### Added — Loop Engineering (decoupled completion authority)
- **Stop-gate harness** (`internal/stopgate`): an independent completion
  authority consulted after the verify-gate passes. Hybrid mode runs
  deterministic checks first (fail-closed) then a strong/equal LLM judge
  (`SIN_EVALUATOR_MODEL`) for non-mechanical criteria; a green judge can never
  override a red deterministic check. Rejection forces the loop to keep working
  with the open criteria injected back in. 92.5% test coverage.
- **Goal contracts / Definition-of-Done** (`internal/goalcontract`): machine-
  checkable acceptance criteria per goal, layered resolution (explicit file +
  inline `--criteria` + `--done-when`, auto-detected Go checks incl. a
  `no-new-todos` diff guard, verify-cmd fallback). Persisted in the queue's
  `contract` column. `goal add --criteria/--contract-file`. 94.5% coverage.
- **Continuation instead of hard abort** (`agentloop`): with `AllowContinuation`,
  hitting `max-turns` now checkpoints and returns a resumable `Result`
  (`Continuation=true`) instead of erroring. The daemon re-enqueues via
  `queue.Continue` (refunds the attempt, bumps a `continuations` counter)
  bounded by `--max-continuations`. Long tasks never need a human restart.
- **Recursive goal decomposition** (`autonomy.Queue`): `parent_id`/`depth`
  columns, `AddSub`, depth-first draining (a parent only finalizes once every
  child verifies, via `Complete`→`blocked`→`TryFinalize`/`bubbleUp`). The daemon
  exposes a `spawn_subgoal` tool bounded by `--max-depth`.
- **Autonomous backlog discovery** (`autonomy/discover.go`): scans TODO/FIXME/
  XXX/HACK markers and unchecked `MASTER_TODO.md` items into deduplicated goals
  (`AddDiscovered` with a `dedup_key`). Exposed as a `discover` trigger type and
  the `goal discover [--dry-run]` command — the agent finds its own work.

### Added — Learning Subsystem (continuous learning in Go)
- **`internal/instinct/`** — continuous-learning subsystem (port of the
  `continuous-learning-v2` homunculus model in a clean-room Go
  reimplementation). Project-scoped + global Markdown-with-frontmatter
  store, confidence 0.3–0.9, Reinforce/Contradict/Decay math with
  `atomic.Value`-backed env-overridable tuning, heuristic + LLM-backed
  extractors (with graceful fallback), cross-project promotion, cluster
  evolution into Skill/Command/Agent proposals, and a system-prompt
  block renderer that closes the learning loop. CLI: `sin instinct
  status|projects|evolve|promote|prune|export|import|show|forget|history`.
  Storage: `$SIN_INSTINCT_DIR | $XDG_DATA_HOME/sin-code/instinct |
  ~/.local/share/sin-code/instinct`.
- **`internal/hooklife/`** — native Go lifecycle-hook system (no Node
  dependency). Phases: `PreToolUse`, `PostToolUse`, `Stop`,
  `SessionStart`, `SessionEnd`, `PreCompact`, `UserPrompt`. `PreToolUse`
  may `Block` (ECC exit-code-2 equivalent); other phases aggregate
  warnings. Per-hook timeout, panic recovery. Built-in hooks:
  `block-no-verify`, `config-protection`, `post-edit-format`,
  `quality-gate` (against the real `verify.Gate`), `cost-tracker`,
  `suggest-compact`. CLI: `sin hooks list|test`.
- **`internal/assets/`** — harvested agent/command/skill loader with
  schema validation (port of ECC CI validators, including unsafe-unicode
  and duplicate detection), `Selector` for domain+keyword-based
  ranking, and an `import` subcommand that harvests skills from a
  vendored source repo with origin/license attribution. CLI:
  `sin assets list|validate|show|import`.
- **`internal/evalharness/`** — eval-driven development. `EvalSet` /
  `Run` / `Result` types, pluggable `Scorer`s (exact, contains-all,
  success-flag, LLM-judge, composite), per-case timeout, JSONL run
  history, and `Compare` for case-by-case regression detection with
  `--fail-on-regress` as a CI gate. CLI: `sin eval run|list|compare`.
- **`internal/dispatch/`** — turns loaded command and agent assets
  into executable actions. ECC-style placeholder substitution
  (`$ARGUMENTS`, `$1..$9`, `$@`, `${flag}`), `Dispatcher` routes
  slash-commands to `PromptSink` and agent requests to
  `SubagentRunner`. Closes the load → select → dispatch → run
  pipeline.
- **`internal/prp/`** — Product Requirement Prompt workflow. Persistent
  reviewable plans under `.sin/prp/<id>.md` driven through phases
  (draft → planned → implementing → verifying → ready → shipped).
  Each step persists, so a run is interruptible and resumable.
  Verification failure kicks the PRP back to `implementing`. CLI:
  `sin prp new|run|status|plan|implement|verify|pr`.
- **`internal/adapters/`** — concrete adapters that implement the
  abstract `hooklife.Verifier`, `instinct.MemorySink`, and
  `instinct.Completer` interfaces against the real SIN-Code
  subsystems (`verify.Gate`, `memory.Store`, `llm.Client`). Fail-soft:
  missing subsystems degrade to no-ops, never block startup.
- **`internal/learning/`** — bridge package between
  `agentloop.Loop` and the new subsystems. `Learner` exposes
  `BeforeTurn` (prepends active-instinct system block), `BeforeTool`
  (PreToolUse dispatch, may veto), `AfterTool` (PostToolUse + observer
  feed), `EndTurn` (observer flush), `PreCompact` (flush + hook
  dispatch). Built once at startup via `learning.New(Options)`.
- **`internal/wiring/`** — `Build(Deps)` assembles the full
  `Bundle{Learner, Dispatch, Eval factory, PRP deps}` in one call.
- **`examples/eval-sets/`** — `go-quality.json` (build, vet, test,
  secrets scan) and `instinct-behavior.json` (end-to-end learning
  loop validation).
- **Five new top-level subcommands** wired into `cmd/sin-code/main.go`:
  `sin instinct`, `sin hooks`, `sin assets`, `sin evalset`, `sin prp`.
  (The existing `sin eval` — Golden-Dataset runner from issue #75 — is
  preserved unchanged; the new harness lives at `sin evalset` to avoid
  a cobra `Use:` collision.)

### Notes
- All loop-engineering features are opt-in and fail-safe: a nil stop-gate /
  empty contract / `AllowContinuation=false` preserves exact legacy behavior.
- The learning subsystem is additive — it does not modify the existing
  `internal/agentloop` package. The chat command can opt into the learner by
  calling `learning.New(...)` and invoking the lifecycle methods around its
  loop run; the default is "no learning wired" so the chat behavior is
  unchanged for existing users.
- `go test -race` clean across the new learning packages. No new third-party
  dependencies (`gopkg.in/yaml.v3` was already transitively present in
  `go.sum`).

### Added — Spec-Layer (issue #157)
The Spec-Layer is the bridge between human intent and machine-checkable
verification. A `*.spec.md` file captures a change's contract as
`Requirements` + `Acceptance Criteria` (each with an optional `verify:`
shell command) + `Invariants`. The agent and CI can then run those
checks, and the drift checker verifies the code still matches the
spec's signatures.

- **`internal/spec/`** — the Spec-Layer core (issue #122, hardened by
  #157). Parses `*.spec.md` files; `Spec.Marshal` writes them back in
  canonical form. `Spec.Check(ctx, timeout)` runs every criterion's
  `verify:` shell command and aggregates per-criterion results into a
  `CheckReport` with `HasFailures()` for the CI gate. `Spec.Author(ctx,
  desc, opts)` runs the LLM Planner → Implementer → Drift-check loop
  with up to 3 retries on drift. `Spec.DetectSignatureDrift(root)`
  walks the source tree and compares backtick-wrapped Go/Python
  function signatures and JSON object shapes against the spec. ~3700
  LOC, 22+ tests, race-clean.
- **Python signature matching** via subprocess to `python3` + `ast`
  (`internal/spec/python.go`). Embedded extractor script as a const
  string; no separate `.py` file to ship. Top-level functions only in
  v0; method-on-receiver deferred to PR 4.
- **JSON shape matching** (`internal/spec/json.go`). Structural type
  check: every spec key must exist in a JSON file with a compatible
  type (`string`/`int`/`bool`/`array`/`object`/`null`, with `[]T`
  and `{}` as sugar). No new deps (M2).
- **LLM wiring** (`internal/wiring/spec.go`). `NewSpecCompleter` adapts
  `llm.Client` to `spec.Completer` so `sin spec author` can drive the
  end-to-end loop. Env var `SIN_SPEC_LLM_BASEURL` is the v0 hook for a
  local model; `--dry-run` is the no-LLM path that returns a stub
  spec for end-to-end testing.
- **CLI**: `sin spec validate|show|check|author`. New flags:
  `check --drift` runs the spec↔code drift; `check --root <dir>`
  scopes the walk; `author --dry-run --out <file>` writes a stub spec;
  `author --apply` opens a PR via `gh` (scaffolded, wired in PR 4).
- **Pre-commit hook** (`scripts/spec-drift-check.sh`): runs
  `sin spec check --all` on every commit. Override path is
  `git commit --no-verify` per M3.
- **CI workflow** (`.github/workflows/spec-ci.yml`): runs the spec
  check on every PR and push to main. A must-priority failure
  blocks the merge. M1-compliant (n8n-delegated).
- **Spec format change**: the `verify:` annotation now requires
  backtick-wrapping (`` `verify: cmd` ``) so the parser doesn't
  misread plain prose as a verify command. Existing pre-v3.18 spec
  files need a one-time `sed` pass; the migration is documented in
  `docs/spec-layer.md`.
- **Tests**: 22+ tests in `internal/spec/`, race-clean. Cover the
  parser, the verify:-runner, the LLM loop (with a stub
  `Completer`), Go/Python/JSON drift, type compatibility, and
  persistence.
- **Docs**: `docs/spec-layer.md` is the canonical reference; it
  supersedes the older `docs/spec-layer.md` content (the file is
  extended, not replaced). `docs/SPEC-LAYER.md` is the design spec
  for the hardening pass.

### Added — Four-arm Eval Comparator (issue #171)
- **`internal/evalharness/arms.go`** — built-in `Arm` constructors
  for the canonical four-arm harness: `__baseline__` (no system
  prompt), `__terse__` (`"Answer concisely."`), `__lazy_skill__`
  (`skill-code-lazy` body issued from issue #178), and the
  `<user-skill>` arm named by `--skill`. Skill discovery is
  best-effort and falls back to a byte-stable `[skill unavailable]`
  placeholder so snapshots remain diff-clean.
- **`internal/evalharness/comparator.go`** — `Compare(ctx, EvalSet,
  []Arm, CompareOptions)` runner. The outer loop is per-arm, the
  inner loop per-case; per-(case, arm) results are aggregated into
  TotalsByArm per arm. `NoOpSubject` and `SetDefaultSubject` keep
  the harness honest for offline / stub CI runs.
- **`internal/evalharness/prices.go`** — self-pricing price book
  (USD per 1k prompt + completion tokens) keyed by `Arm.PricingName`.
  Known models: `stub`, `gpt-4o`, `gpt-4o-mini`,
  `claude-3.5-sonnet`, `fireworks-qwen2.5-7b`,
  `fireworks-llama-3.1-70b`. Unknown names produce a warning in
  `CompareReport.Warnings` and zero USD (so the harness never
  silently under-reports cost).
- **`internal/evalharness/snapshot.go`** — deterministic snapshot
  round-trip. `BuildSnapshot` sorts rows by `ArmID`, takes medians
  across all per-case values, and emits byte-stable JSON.
  `WriteSnapshotFile`/`LoadSnapshotFile` round-trip disk I/O.
  `DiffSnapshots` produces row-level deltas with the
  `changed-skill-body` signal for SKILL.md drift.
- **Result/Output extensions**: `Result` carries `ArmID`,
  `PromptTokens`, `CompletionTokens`, `TotalTokens`, `LOC`, `USD`
  (all `omitempty` for backward compatibility). `Output` carries
  an optional `USD` for Subject authors that compute cost at the
  source.
- **`cmd/sin-code/eval_cmd.go` extensions** — three new
  subcommands: `eval compare`, `eval snapshot`, `eval diff`.
  `eval run --arm baseline,terse,lazy_skill,<skill>` opts into
  the comparator path; without `--arm` the legacy Golden-Dataset
  path is preserved unchanged. The four-arm matrix output mirrors
  ponytail's `benchmarks/README.md:34-58` columns (LOC, USD,
  latency, correctness).
- **`evals/three-arm-example.json`** — canonical example dataset
  with 3 cases: 2 LOC-countable (gopher explain + reverse Go
  function), 1 LLM-judge (lz4 vs zstd).
- **Comparator test coverage** — 11 new tests pass with
  `go test -race -count=1`. Median aggregation, 4-arm matrix,
  snapshot byte-round-trip, schema-version rejection, late-write
  warnings, parallel-vs-serial equivalence (race-detector safe).
- **`Compare` renamed in `regression.go`**: `Compare(base, cand,
  eps)` → `CompareRuns(base, cand, eps)` to free the bare name
  for the new multi-arm comparator (issue #171). All three call
  sites (`cli.go`, `evalharness_test.go`) updated.
- **`AGENTS.md` §12.1** added documenting the four-arm comparator
  contract: the honest delta = `<user-skill>` − `__terse__`, not
  the inflated `<user-skill>` − `__baseline__`.
### Added — Bundled skills (issue #178)
- **`skill-code-lazy`** (35th bundled skill, in `skills/process-skills/`):
  SIN-Code adaptation of Dietrich Gebert's `ponytail` skill — "ship
  the laziest version that actually works" with the 6-stufige Leiter
  (YAGNI → stdlib → platform → existing dep → one function → minimum
  that works). **Gated by `verify.pass` (mandate M3)**: the skill is
  inert while `verify.result ∈ {pending, pre, fail}` and only arms
  after the verify-gate. Activation keyword `lazy_skill` (issue #176)
  binds to the four intensities `off | lite | full | ultra`.
- **`sin-debt:` marker cookbook** in `templates/debt-markers.md`:
  paired ceiling + upgrade-trigger convention (issue #177); every
  shortcut ships a `// sin-debt: <ceiling>, upgrade: <event>` pair
  so reviewers can audit YAGNI vs hardening pressure.
- **Byte-stable render contract**: the 5 keyword examples in
  `SKILL.md` render to identical octets across runs (prerequisite
  for the issue #2 system-prompt hash metric).
- **Naming-convention exception** recorded in `AGENTS.md` §10: the
  canonical pattern is `skill-<category>-<name>`, but
  `skill-code-lazy` is preserved as the v3.18.0 exception because
  the `lazy_skill` activation keyword binds to the literal
  frontmatter name.

## [v3.17.0] - 2026-06-13

### Added
- **Structured logging** (`internal/logger/`): JSON output with log levels
  (DEBUG/INFO/WARN/ERROR), dynamic stderr for testability.
- **Health check endpoints** in WebUI: `/health`, `/live`, `/ready`, `/info`
  with custom checks for templates and todo_db.

### Fixed
- **MCP server warnings deduplicated** (#66): each server name warned about
  at most once per process lifetime.
- **TUI test flake** (#64): `SkipMCP` flag in loopbuilder/tui configs —
  tests skip MCP connections (48s → 3.3s).
- **Python 3.14 test fix** (#65): marketplace update tests mock `Updater`
  class to avoid real `git pull` calls.

## [v3.16.1] - 2026-06-13

### Fixed
- Mock git pull in marketplace update tests for Python 3.14 compatibility.

## [v3.16.0] - 2026-06-13

### Added
- **Forge integration (#37)**: `sin forge` top-level command (thin wrapper
  around the `forge` binary from SIN-Code-Forge-Tool). `sin status` now
  detects both the `forge` binary and the `sin-forge` MCP server.
  `mcp_config` full mode registers `sin-forge` as the 16th individual tool.
  ECOSYSTEM.md lists SIN-Code-Forge-Tool as ACTIVE.

## [v3.15.0] - 2026-06-13

### Added
- **Go-native SCA Phase 1 (#41)**: new `cmd/sin-code/internal/security/sca/`
  package that parses `go.mod` natively and calls `grype` JSON output via
  subprocess. Wired into `sin security` for Go projects with 91.5% test
  coverage.

### Fixed
- **Race-flake hardening (#59)**: `TestDoctorVaneDownIsNotFatal` is now
  hermetic — it forces an unreachable Vane URL instead of depending on the
  local environment. `go test ./... -race -count=3` passes on one run.

## [v3.14.0] - 2026-06-13

### Added
- **Unified config subsystem (#34)**: `sin-code config` now supports
  `init`, `show`, `validate`, `get`, `set`, `list`, and `path`.
  - User config: `~/.config/sin/sin-code.toml` (defaults).
  - Project config: `./.sin-code/config.toml` (overrides user).
  - Expanded schema: `theme`, `default_timeout`, `default_format`,
    `mcp_server_enabled`, `llm.*`, `agent.*`, `permissions.tools_allow`,
    `permissions.tools_deny`, and `paths.*`.
  - Deep merge: project-level keys override user-level keys; unset keys in
    project config do not zero out user defaults (uses raw key maps).
  - Atomic writes: temp file + rename so readers never see a half-written
    config.
  - Secret masking: `llm.api_key` is masked in `show`/`show --json` unless
    `--plain` is passed.
  - Validation: `sin-code config validate` checks enum values, ranges, and
    positive integers.
- **New tests** in `config_test.go`: show/validate, deep merge, atomic
  writes, secret masking, namespaced keys, expanded roundtrip.
- **CoDocs companion**: `cmd/sin-code/internal/config.doc.md`.

### Fixed
- Updated `cmd/sin-code/testdata/scripts/golden_help.txt` to include
  `hub`, `ledger`, `summary`, `update` and the corrected `serve` tool count
  (15), removing pre-existing help-golden drift from v3.12.0/v3.13.0.

## [v3.13.0] - 2026-06-13

### Added
- **Semantic Session Ledger (#43)**: append-only SQLite store of agent-loop
  events (prompts, tool calls, verification results, completions). New
  internal packages `ledger` and `summary` (CGo-free via `modernc.org/sqlite`).
- **Ledger integration in agent loop**: `loopbuilder.Build` opens the ledger
  and every `Run` records user prompts, tool calls/errors, verification
  pass/fail, and task completion/abortion.
- **New subcommands**:
  - `sin-code ledger list` — list recent sessions with ledger entries.
  - `sin-code ledger show <session-id>` — show ledger entries for a session.
  - `sin-code summary <session-id>` — deterministic markdown summary from
    the ledger.
  - `sin-code summary <session-id> --evidence` — compact one-line evidence
    string suitable for Oracle-style verification.
- Auto-summaries are deterministic and LLM-free: they report verification
  status, tool-call turns, tools used, and the task-completion one-liner.

## [v3.12.0] - 2026-06-13

### Added
- **Tool catalog hub (#35)**: new `sin-code hub` subcommand with `list`,
  `search`, and `info` subcommands. Static, categorized catalog of all 37
  subcommands plus key MCP surfaces. Read-only, no runtime dependencies.
  - `sin-code hub` prints full categorized catalog.
  - `sin-code hub list` prints flat list of all tools.
  - `sin-code hub search <keyword>` searches by name, short, or description.
  - `sin-code hub info <tool>` prints detailed description and example.
  - New internal package `cmd/sin-code/internal/hub/` with `hub.go`,
    `hub_test.go`, and `hub.doc.md`.
  - New CLI binding `cmd/sin-code/hub_cmd.go`.

## [v3.11.0] - 2026-06-13

### Added
- **sin update e2e (#33)**: top-level `sin update` command for full-stack self-update
  (Go + scripts + skills). Replaces 15+ manual steps with a single command.
  Flags: `--python-only`, `--go-only`, `--skills-only`, `--check`, `--dry-run`,
  `--force`, `--rollback`, `--skip-doctor`, `--state-root`, `--keep-snapshots`.
  Snapshot-based rollback via `update_manifest.go`, `update_backup.go`,
  `update_phases.go`, `update_rollback.go`, `update_cmd.go`.
  `sin-code self-update` remains as legacy alias.
  Fixed `githubAPIURL` to point to `OpenSIN-Code/SIN-Code` (was archived `SIN-Code-Bundle` repo).
- **security + sbom MCP tools (#36)**: `sin_security_scan` and `sin_sbom_generate`
  exposed via `sin-code serve`, wrapping the in-tree `security` and `sbom`
  CLI subcommands. Both read-only, permission `allow`.
  `sin_security_scan` runs govulncheck, gosec, go vet, bandit, safety,
  npm audit, secrets grep, and file-permission walker.
  `sin_sbom_generate` generates SPDX 2.3 JSON or CycloneDX 1.5 JSON.
  Timeout ceiling 3600s at MCP layer. Path-escape guard on output param.
  TUI sidebar `security` now marked `Runnable: true`.

### Changed
- Serve help text: 13 → 15 tools. `security` and `sbom` removed from CLI-only exclusion list.

## [v3.10.0] - 2026-06-13

### Fixed
- **`--version` flag on 13 Go-tool subcommands** (#38). Previously
  only `sin-code --version` worked; per-subcommand invocation
  (`sin-code discover --version`, etc.) errored with `unknown flag`.
  Each of discover, execute, map, grasp, scout, harvest, orchestrate,
  ibd, poc, sckg, adw, oracle, efm now prints `<name> <version> (commit <sha>, built
  <date>)` and exits 0. Side-effect: fixed a longstanding ldflag
  injection bug in `.goreleaser.yaml` (lowercase `main.version` did
  nothing) and `install.sh` (no version injection at all) — production
  builds now report the real tag instead of `dev`.

### chore
- **#61** — `.gitignore`: ignore `cmd/sin-code/tui/.sin-code/` runtime
  artifacts produced by the TUI's session/lessons store; add CoDocs
  companion `.gitignore.doc.md`; add regression test
  `tests/test_gitignore_tui_sin_code.py`. No code paths changed.
- **#40** — Cross-repo: standardized AGENTS.md to SIN-Code 8-section template
  in 6 ecosystem tool repos (SCKG, IBD, PoC, ADW, Oracle, EFM).

## [v3.9.0] - 2026-06-13
- **GitHub CLI bridge** (`internal/ghbridge/`): bridged external (NEVER vendored) for the official `gh` CLI. 3-tier verb policy enforced in code: read-only (allow) | mutating (ask) | forbidden (hard-blocked). 3 MCP tools: `gh_query` (allow), `gh_execute` (ask), `gh_health` (allow). Enables the SIN-Code contributing workflow "issue first" to be executed by the agent itself.
- New subcommand: `gh` (setup/doctor/run/surface/serve). 35 → 36.
- Permission-Defaults: `gh_query`/`gh_health` → allow, `gh_execute` → ask.
### Security
- Defense in depth: `gh_query` re-validates with `Classify` and rejects mutations even if caller picked wrong tool.
- Fail-closed: unknown verbs/groups → `TierForbidden`, never reach runner.
- `gh api`, `gh auth`, `gh secret`, `gh config`, `gh alias`, `gh extension`, `gh codespace`, `gh fork`, `gh sync`, `gh archive/unarchive/transfer`, `gh ssh-key`, `gh gpg-key` are hard-blocked.
### Mandate Compliance
- M1 n8n-CI only ✓
- M2 CGo-free, stdlib-only ✓
- M3 Verification-Gate passed: build OK, vet OK, race OK
- M4 3-tier policy matches permission engine ✓
- M5 Module path correct ✓
- M7 Race-clean ✓

## [v3.8.0] - 2026-06-13

- **Vane bridge** (`internal/vane/`): HTTP-Bridge zur ItzCrazyKns/Vane (MIT) self-hosted AI-answering-engine mit zitierten Quellen. stdlib-only, stdio MCP server (2 tools: `vane_research`, `vane_health`), graceful degradation → websearch fallback. Closes #62.
- **Stack consolidation** (`internal/stack/`): unified `sin-code stack install|doctor` über superpowers + dox + vane. Idempotent, --json output, graceful degradation pro layer. Closes #62.
- New subcommands: `vane` (setup/doctor/search/config/serve), `stack` (install/doctor). 33 → 35.
- New MCP servers in `.sin-code/mcp.json`: `vane` (2 tools), plus pre-existing `superpowers` (3 tools) and `dox` (0 tools, protocol-block based).

### Mandate Compliance
- M1 n8n-CI only ✓
- M2 CGo-free, stdlib-only ✓
- M3 Verification-Gate: PoC + Oracle (commit-time) ✓
- M4 Permission-Defaults updated, ecosystem-sync green ✓
- M5 Module path correct ✓
- M7 Race-clean (tested with -race -count=1) ✓

## [3.7.0] - 2026-06-12

- **`sin-code superpowers`** — integration of obra/superpowers (MIT)
  methodology skills into the SIN-Code agent. Skills (TDD,
  systematic-debugging, subagent-driven-development, verification-before-
  completion, writing-plans, brainstorming, requesting-code-review,
  finishing-a-development-branch, using-git-worktrees) are cloned from
  upstream, pinned to a reviewed commit SHA (supply-chain lock), overlaid
  with SIN-Code tool mappings (M6: sin_* tools over naive builtins), and
  served as MCP tools (`superpowers_list_skills`, `superpowers_find_skill`,
  `superpowers_use_skill`).
- **Review-before-trust update flow:** `sin-code superpowers update`
  shows the upstream skill diff first; applies + re-pins only with
  `--yes` (skill content flows into agent context — must be reviewed
  like a dependency bump).
- **Full YAML frontmatter parser:** handles plain values, quoted strings,
  folded block scalars (>–), literal block scalars (|–), and indented
  continuations — all forms used by upstream superpowers.
- **AGENTS.md auto-injection:** `sin-code superpowers init` adds a
  Superpowers prompt block (bounded by `<!-- SUPERPOWERS:BEGIN/END -->`)
  making skill usage a mandatory agent workflow.
- **Defense-in-depth:** skills are NOT destructive (overlay on top of
  upstream files), idempotent (re-install = no-op), and pinned (no
  automatic `git pull` of new content into agent context).

## [3.6.0] - 2026-06-12

- **Swarm mode** — `sin-code swarm -p <prompt> --agents <n1,n2,n3>`. N agent
  profiles race the same prompt headless; first verified solution wins.
  Per-agent isolated sessions. Cancellation via parent context.
  Mandate M4 holds (headless ask->deny).
- **Self-extending agent** — `sin_bootstrap_skill` tool. Agent writes
  Python MCP servers from natural-language specs, smoke-tests them,
  and registers in `.sin-code/mcp.json`. Defense-in-depth: permission
  policy "ask" + env gate `SIN_ALLOW_BOOTSTRAP=1` for headless use.
- **TUI v3.3.1** — `internal/tui/agent_runner.go` (84.6% cov). TUI embeds
  the real agent loop. Skill palette entries execute live instead of
  printing CLI hints. Permission asks render as TUI dialogs (y/N) over
  the AskReply channel.
- **WebUI-v2 backend API** — `internal/apiweb/api.go` (81.5% cov). 6
  HTTP endpoints (sessions CRUD, fork, knowledge, chat-with-SSE) with
  bearer-token auth via `SIN_API_TOKEN` and localhost-only fallback.
  Mounted by `sin-code serve --transport=http`. Chat endpoint streams
  progress as SSE events, final frame is the stable JSON contract
  `{session_id, summary, verified, turns}`.

## [3.5.0] - 2026-06-12

- `internal/lessons` — persistent knowledge base (SQLite, modernc);
  failed verifications and tool errors accumulate with occurrence
  counts. `lessons.Briefing` injects top repeated lessons before the
  first turn (singletons are noise, repetition is signal).
- `internal/loopbuilder` — shared factory eliminates duplication of
  provider/permission/hooks/gate/mcp/lessons setup across chat/swarm/
  serve (DRY refactor).
- agentloop.Loop gained `Lessons` field; on verify.fail / tool.error
  the lesson is recorded. On Run() start, the briefing is injected
  before the first turn.
- `internal/mcpclient` — `server__tool` namespacing, LoadConfigs with
  mcp.json discovery (merge defaults + user + workspace), registry of
  13 ecosystem servers (12 skills + Symfony-Lens).
- `sin-code mcp list|status|call` — live MCP debugging.
- Chat command suite (chat_cmd.go, chat_mcp.go, chat_tools.go):
  interactive REPL + headless one-shot with stable JSON contract.
- `sin-code sessions list|show|rm` — persistent resumable sessions
  over `~/.local/share/sin-code/sessions.db` (modernc, foreign_keys=ON).
- Ecosystem consolidation: ECOSYSTEM.md (24 ACTIVE repos + sync rules),
  requirements-ecosystem.txt (8→24 entries), profiles/*.toml
  (fireworks, qwen-relay), docs/HOOKS.md, docs/WEBUI.md,
  docs/mcp.json.example.
- .github/workflows/ecosystem-sync.yml — CI gate preventing drift
  between registry.go, permission_defaults.go, ECOSYSTEM.md,
  requirements-ecosystem.txt.
- Goal-queue + autonomy: persistent SQLite queue, atomic leases,
  cron + file-watch triggers, skill-lifecycle manager.
- 7 new hook events: goal.enqueued/started/verified/exhausted,
  trigger.fired, skill.installed/failed.
- `sin-code daemon --verify-cmd` — autonomous worker (M3+M4 enforced).
- `sin-code goal add|list` and `sin-code skill install|status`.

## [3.4.0] - 2026-06-12

- Einstein Layer — the agent that learns from mistakes.

## [Unreleased]

- **LSP integration dependencies** — `sin-code lsp` now documents its gopls
  requirement. Install via `brew install gopls` (macOS) or
  `go install golang.org/x/tools/gopls@latest` (Linux/CI). Without gopls on
  `$PATH`, Go-language LSP commands degrade gracefully to a "gopls not
  detected" message (see `sin-code lsp servers`).
- **Live LSP regression testscript** — `cmd/sin-code/testdata/scripts/lsp_live.txt`
  exercises symbols / hover / definition / references / format against this
  repository. Added so the LSP client can be re-validated whenever `client.go`
  changes.
- **Ecosystem cleanup (legacy Python bundle)** — removed the deprecated
  `sin-code-bundle` package from `AllPythonPackages` in `cmd/sin-code/internal/update_phases.go`;
  the `sin update` command no longer attempts to upgrade the superseded Python
  companion. Fixed remaining `SIN-Code-Bundle` repo-name references in
  `go.mod`, `self-update.doc.md`, and `harvest.doc.md` (M5 compliance).

### chore
- **#61** — `.gitignore`: ignore `cmd/sin-code/tui/.sin-code/` runtime
  artifacts produced by the TUI's session/lessons store; add CoDocs
  companion `.gitignore.doc.md`; add regression test
  `tests/test_gitignore_tui_sin_code.py`. No code paths changed. No
  version bump.

### Known Issues
- **LSP framing bug** — `internal/lsp/client.go:Client.Call` reads LSP responses
  one line at a time with `bufio.ReadString('\n')`, but gopls v0.20+ emits
  JSON-RPC notifications (e.g. `window/logMessage`, `$/progress`) on the same
  stdout stream. The header parser only recognises `Content-Length:` lines, so
  notification lines desync the reader, and subsequent `io.ReadFull` returns a
  truncated body. Visible as
  `Error: initialize go: unexpected end of JSON input`
  on every `sin-code lsp {symbols,hover,definition,references,format}` call.
  Workaround: pin gopls to v0.16.x or rewrite `Call` to use
  `bufio.Scanner` with a custom split function that tolerates interleaved
  notifications. Tracked in follow-up issue (see `docs/lsp-known-issues.md`).

## [2.5.0] - 2026-06-11

- **Persistent Incremental Index (Phase 3)** — gob-persisted trigram + symbol
  index at `<root>/.sin-code/index.bin`. Auto-builds on first search,
  stat-based incremental refresh, 8 parallel build workers. New `index`
  subcommand (build/refresh/status/watch/clear) and MCP `sin_index` tool.
  Scout CLI now uses indexed search with 25-37× speedup over full scan.
- **AST tiered structure extraction (Phase 4)** — 3-tier provider (Go go/ast
  exact, structural fallback, tree-sitter opt-in via `-tags treesitter`).
  Default build stays zero-dep. Enables `read --mode outline` with engine
  info, `edit --symbol NAME` for AST-anchored edits, and unified parsing
  across all consumers.
- **Phase 4b — grasp/map/SCKG migrated to parseOutline()** — removed 5
  regex-based per-language extractors in `grasp.go`, replaced with single
  `parseOutline()` call. SCKG `buildGraph` now uses `parseOutline` for all
  languages (no more regex for Python/JS). Map entry-point detection uses
  `isGoEntryPoint()` via AST lookup. Kind normalization helpers
  (`normalizeGraspKind`, `sckgKind`) maintain backward-compatible labels.
- **Phase 5 — Benchmark suite + CI gate** — 18 Go benchmarks across all
  tools with synthetic project trees (`makeTree()`), `benchmark.sh` shell
  runner with pprof profiling (`PPROF=1` mode), `.github/workflows/go-ci.yml`
  with median speedup gate (≥3× indexed vs fullscan on CI runners).
  BenchmarkComparisonTable directly compares fullscan vs indexed sub-bench.

### Changed
- **Go upgraded to 1.25.11** — was 1.24.3 (ADR-008, st-gvc4). go.mod
  updated, CI workflows updated, govulncheck switched from warn-only to
  blocking (Go 1.25 fixed the stdlib false positives that required the
  carve-out). ADR-008 marked as Superseded.
- **Coverage corrected** — the 93.6% claim in v1.0.9 was for the cmd/sin-code
  package only. Full project coverage (including internal/ and all
  sub-packages: plugins, lsp, memory, todo, notifications, orchestrator,
  webui, llm, attachments, tui, tui/chat) is 68.2% as of this release.
  Goal for v2.6.0: raise internal/ coverage to ≥80%.

### Fixed
- **st-pwt5** — `testdata/scripts/plugin_wire.txt` manifest was using
  deprecated v2.3.0 minimal format. Updated to current TOML schema
  (description, provider, timeout, capabilities, populated agents/tools)
  so the test exercises the modern manifest shape, not the deprecated
  one. Added descriptive comment at top of the testscript.
- **CI benchmark gate** — was using integer-only bash arithmetic that crashed
  on float ns/op values, and used `sort -n | head -1` (minimum) which biased
  against the indexed path. Now uses float-safe awk with median calculation
  and a 3× threshold (was 5× — too aggressive for 2-4 core CI runners).
- **Legacy Python CI** — `ci.yml` was red on every Go commit because the
  deprecated Python stack still ran ruff + pytest. Added path filters so
  it only triggers on `**.py` / `pyproject.toml` / etc.

### Closed Issues
- st-gvc4 (govulncheck blocking) — P3
- st-pwt5 (plugin_wire test) — P2
- st-phw1 (plugin hook wiring) — P0 [closed retroactively, fixed in Phase 3/4]
- st-ptm2 (plugin tools → MCP) — P0 [closed retroactively, fixed in Phase 3/4]

## [2.4.0] - 2026-06-08

LSP framing fix, plugin system, multi-agent orchestrator, TUI chat LLM, NIM
model aliases. See commit `63b33f5` for the full list of changes.

## [1.1.0] - 2026-06-07

- **TUI 2.0** — complete rewrite of `sin-code tui` as a multi-pane command center
  - Session tab bar (top, up to 6 sessions)
  - Collapsible left sidebar (Ctrl+B) with 5 views + 19 subcommands
  - Custom SIN-Code loading animation (rotating half-block halo + ⚡)
  - Bottom footer with view name, agent (Build/Audit/Stats), token stats, cost
  - Command palette (Ctrl+P), subagents popup (Ctrl+X), view switcher
  - 5 themes: default, Dracula, Nord, Solarized, Monokai
  - Multi-view support: Tools, Sessions, EFM, Config, History
- **EFM OrbStack support** — auto-detect `orb` on macOS, `--runtime orb|docker|auto` flag
- **OrbStack mandate** (PRIORITY -5.0) — added to all 3 AGENTS.md files
- **TUI design doc** — `docs/tui-v2-design.md` (1,319 lines, opencode research)

### Changed
- TUI moved to dedicated `cmd/sin-code/tui/` package (~2,900 LOC, 15 files)
- Old monolithic `tui_test.go` + `tui_interactive_test.go` removed (replaced by 61 new tests)

### Architecture
- Bubbletea v1.3.10 (matches go.mod)
- 5 themes via Lipgloss, multi-pane via lipgloss.JoinHorizontal/Vertical

## [1.0.9] - 2026-06-07

- 448 new tests bringing coverage from 82.7% to 93.6%
- serve_handlers_test.go: all 13 MCP handleXxx functions + runSubcommand (1136 lines)
- execute_extended_test.go: 55+ tests for runCommand, checkSafety, redactSecrets, signal handling
- main_subprocess_test.go: 11 tests for main() symlink routing + checkUpdate
- efm_test.go: expanded from 14 → 44 tests with Docker skip logic
- sbom_test.go: expanded from 16 → 45 tests, CycloneDX + edge cases
- All 12 core/advanced files pushed to 95%+ coverage

### Changed
- sbom.go: fix parseGoModFallback single-require parsing bug
- Coverage increased from 82.7% to 93.6% (+10.9%)
- Total tests: 415 → 863
- Files at 95%+ coverage: 0/20 → 17/20

## [1.0.8] - 2026-06-07

- 84 new tests bringing coverage from 73.6% to 82.7%
- self_update_test.go: 30 tests with httptest mocks for GitHub API, tar.gz/zip extraction, downloadFile
- security_extended_test.go: 28 tests for tool runners (govulncheck, gosec, bandit, safety, npm audit, secrets-grep, file-permissions)
- main_extended_test.go: 11 tests for checkUpdate stamp logic + symlink routing
- common_test.go: 7 tests for PrintError, lookupStandalone, capitalize
- config_test.go: +12 tests for get/set roundtrip, list, path, init, persist/reload

### Changed
- self-update.go: extract githubAPIURL var for testability (was hardcoded URL)
- Test coverage increased from 73.6% to 82.7% (+9.1%)
- Total tests: 331 → 415

## [1.0.7] - 2026-06-07

- 200+ new tests (unit + E2E + MCP integration)
- 7 new dedicated test files (ibd, poc, sckg, efm, grasp, map, scout)
- testscript E2E framework (9 CLI tests)
- MCP server stdio integration tests (10 stdio + 9 integration)
- Dependency: rogpeppe/go-internal v1.15.0 for testscript

### Changed
- Test coverage increased from 48.4% to 72.2%
- Documentation: corrected tool counts across AGENTS.md, main.go, serve.go (19 subcommands = 13 MCP + 6 CLI-only)

## [1.0.4] - 2026-06-07

- `security` subcommand — auto-detects project type (Go/Python/Node/Generic) and runs available security tools
- `config` subcommand — manages sin-code configuration (get, set, list, path, init)
- `self-update` subcommand — checks GitHub releases and installs latest binary with backup/restore
- TUI themes — 5 built-in color schemes (default, Dracula, Nord, Solarized, Monokai)
- TUI arg-input mode — press 'r' and enter arguments for commands that need them
- Daily update availability check — non-blocking, runs once per day when --version is used
- Windows zip extraction in self-update (archive/zip support)

### Changed
- Pipeline: govulncheck non-blocking (Go 1.24.3 stdlib CVEs fixed in Go 1.25)
- TUI status bar shows dynamic hints per command (Enter: --help, r: run, t: theme, q: quit)
- Homebrew formula updated for v1.0.4 with SHA-256 checksums

### Fixed
- Go version compatibility: downgraded to Go 1.24.3 with compatible dependencies
- Release pipeline: multiple hotfixes for Go toolchain, artifact upload, cross-compilation
- GitNexus index rebuilt with 9,997 symbols and 17,832 relationships
- AGENTS.md synced across all 3 repos (SIN-Code-Bundle, Infra-SIN-OpenCode-Stack, ~/.config/opencode)

## [1.0.3] - 2026-06-07

- `tui` subcommand — interactive Bubbletea menu for all subcommands with fallback

### Fixed
- Pipeline hardened: go vet blocking, govulncheck non-blocking with artifact upload

## [1.0.2] - 2026-06-07

- 13 core tools in unified Go binary: discover, execute, map, grasp, scout, harvest, orchestrate, ibd, poc, sckg, adw, oracle, efm
- MCP server mode (`serve`) exposing all 13 tools via JSON-RPC 2.0 stdio
- Symlink backwards compatibility (`discover`, `execute`, etc. → `sin-code`)
- 5-platform release pipeline (darwin/linux × amd64/arm64 + windows-amd64)
- Homebrew formula and tap repo (`OpenSIN-Code/homebrew-sin`)

## [1.0.0] - 2026-06-04

- Initial release of 7 standalone Python tools (discover, execute, map, grasp, scout, harvest, orchestrate)
- CEOAudit grade A+ (100.0/100)
