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
### Added — Auto-Activation Hook (issue #176, v3.19.0)
- **`internal/hooklife/autoactivate/`** — per-session rule injection subpackage.
  Two Phase hooks (`autoactivate-session-start` / `autoactivate-user-prompt`)
  register against any `*hooklife.Registry` via `Activator.Register(reg)`.
  Privacy-first: off by default; activated by `sin-code chat --activate <rule>`
  or a project-local `.sin-code/autoactivate.toml` file.
- **`AutoActivate.Activator`** tracks per-session state under a single
  `sync.RWMutex` (mandate M7 — race-safe under `go test -race -count=1`).
  `OnSessionStart(sid, opts)` is idempotent; `OnUserPrompt(sid, prompt)`
  returns the rule set re-emittable for this turn, with trigger-phrase
  substring matching for natural-language activation. `EndSession(sid)`
  drops state on exit.
- **`RuleSet.Render()`** is byte-stable: any two RuleSets with the same
  name+body+trigger tuples produce identical bytes regardless of insertion
  order (prerequisite for the system-prompt hash metric, issue #2).
- **`sin-code chat --activate terse,skill-x`** comma-separated rule list;
  **`--no-trigger`** suppresses per-prompt phrase matching; reads
  `.sin-code/autoactivate.toml` silently when present.
- Tests: 35 race-safe unit tests + 8 chat-wiring integration tests, 91.5%
  statement coverage on the autoactivate package. New package follows
  the existing `hooklife` Phase contract; no new external deps.
### Added — Orchestrator output contracts (issue #174)
- **Caveman-style output contract** for the four orchestrator sub-agents
  (`internal/orchestrator/output_contract.go`). Every Finding renders to ONE
  byte-stable line: `<path>:<line> — <symbol> — <tag> — <hint> # c=<confidence>`.
  Five closed tags (parallel to `JuliusBrussee/ponytail`): `delete | simplify |
  rebuild | risk | verify`. Em-dash U+2014 separator; no prose, no pleasantries,
  no hedging (`you might`, `perhaps`, `could consider`, `maybe`, `i think`,
  `sort of`, `should probably`, … — closed set of 12 phrases, case-folded).
- **`Finding` struct** (`internal/orchestrator`) and `ParseFinding` /
  `ParseFindings` regex parsers with strict byte-stability. `Render()` is
  fully deterministic — `Finding{...}` → `Render()` → `ParseFinding()` →
  equal struct, every byte counted (verified by `TestParseFinding_RoundTrip`).
- **`VerifyFindings`** runs the full contract: structural (`Path != ""`,
  `Tag ∈ {delete, simplify, rebuild, risk, verify}`), lexical (zero hedging,
  hint ≤ 240 chars, no trailing punctuation), and emits per-Finding error
  strings — never silent drops.
- **Wired into the four sub-agents**:
  - `Critic.Drive` parses the LAST attempt's prose into `CriticResult.Findings`
    and surfaces `CriticResult.ParseErrors` for the orchestrator to re-inject
    as retry feedback (mirrors the `verify.fail` flow).
  - `Adversary.Review` derives Findings from the structured `Attack` slice
    (landed → `risk`, cleared → `verify`); the `CounterexampleBrief` free-
    form prose is preserved as the audit trail.
  - `Governor.Execute` derives one `risk` Finding per `Escalation` (Path =
    `task://<ID>`, Symbol = `<from>-><to>`); the prose `Reason` stays on
    Escalation for the audit log.
  - `Cartographer.Findings(k)` exposes the PageRank-sorted top-k as
    `verify`-tagged Findings (opt-in: k ≤ 0 yields the empty slice).
- **Byte-stable golden tests** (`output_contract_test.go` and
  `output_contract_integration_test.go`) pin one fixture per agent;
  rendering drift breaks the build (the prerequisite for issue #168's
  ledger-level token-cost hashing).

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
  success-flag, LLM-judge, composite, **CompileAndRun**), per-case
  timeout, JSONL run history, and `Compare` for case-by-case regression
  detection with `--fail-on-regress` as a CI gate. CLI:
  `sin eval run|list|compare`.
- **`CompileAndRun` scorer** (issue #181) — ponytail `correctness.js`
  analog for SIN-Code. Extracts fenced code from model output, compiles
  it (`go`/`python`/`javascript`/`bash`), and runs a sandboxed self-check.
  Returns 1.0 only when compile + run pass; `skip_test` mode accepts
  trivial one-liners after compile-only (YAGNI for tests). Wired into
  `sin-code eval run` via `--scorer compile-and-run --language <lang>`
  and into Golden Datasets via `test_cases[].scorer`.
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

### Added — Complexity Audit (issue #180)
- **`sin-code audit complexity`** — repo-wide ponytail-audit analog. Five tags
  (`delete`, `stdlib`, `native`, `yagni`, `shrink`), deterministic static pass
  (single-impl interfaces, single-product factories, wrapper functions,
  one-export files, dead flags/config, hand-rolled stdlib), optional LLM judge
  for top-N findings. Output: one-liner per finding ending with
  `net: -<N> lines, -<M> deps possible.` or `Lean already. Ship.`.
- **`cmd/sin-code/internal/audit/`** — new `Auditor`, `Finding`, `Result` types;
  `// sin-debt:` markers approve findings and exclude them from the net total.
- **`sin-code ceo-audit`** — new 48-gate CEO-grade audit. The 48th gate is the
  complexity audit; score contribution is `+1` per 100 removable lines.
- **Docs** `docs/complexity-audit.md` and **tests** `complexity_test.go` +
  `audit_cmd_test.go` with race-free coverage.

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
### Added — Complexity Review (issue #179)
- **`cmd/sin-code/internal/complexity/`** — static, AST-based complexity analyzer
  implementing ponytail's 5-tag format: `delete`, `stdlib`, `native`, `yagni`,
  `shrink`. Detects single-implementation interfaces, one-product factories,
  wrapper-only functions, hand-rolled `min`/`max`, dead flag-like variables,
  repeat-append loops, and imports that duplicate stdlib/platform features.
  Respects `// sin-debt:` and `# sin-debt:` markers (issue #177).
- **`cmd/sin-code/review_cmd.go`** — new top-level `sin-code review` command with
  `sin-code review --complexity [--path] [--since <ref>] [--tags] [--format text|json|markdown]`.
- **Output format**: one line per finding
  (`<tag>: <what>. <replacement>. [path:line]`), ranked by line count and removed
  dependencies, ending with `net: -<N> lines, -<M> deps possible.` or
  `Lean already. Ship.`. `net_lines` and `net_deps` are included in JSON output.
- **Tests**: `cmd/sin-code/internal/complexity/complexity_test.go` + golden file,
  race-clean.