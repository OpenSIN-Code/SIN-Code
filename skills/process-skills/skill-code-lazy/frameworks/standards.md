# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Origin

`skill-code-lazy` is the SIN-Code adaptation of Dietrich Gebert's
**ponytail** skill (`github.com/DietrichGebert/ponytail/skills/ponytail/SKILL.md`).
The upstream skill ships the 6-stufige Leiter (ladder) and the
"never lazy at trust boundaries" rule verbatim.

The SIN-Code port adds:

1. **M3 gating** — the skill may not activate before `verify.pass`.
2. **Activation-keyword contract** — `lazy_skill` binds to the
   intensity through `autoactivate` (issue #176).
3. **`sin-debt:` marker pairing** — every shortcut is tagged with a
   ceiling + upgrade trigger (issue #177).
4. **Byte-stable render** — keyword examples in `SKILL.md` are
   golden fixtures for the system-prompt hash metric (issue #2).

## The 6-stufige Leiter (byte-stable)

Mirrored from `ponytail/SKILL.md:18-30`:

```
1. Does this need to exist at all? (YAGNI)
2. Stdlib does it? Use it.
3. Native platform feature? Use it.
4. Installed dependency? Use it.
5. One line? One line.
6. Only then: the minimum that works.
```

In SIN-Code the ladder is **not** optional. A new dependency that
bypasses rung 2 or 3 is a CI-blocker.

## Intensity Ladder (SIN-Code specific)

| Intensity | Rule | When to use |
|---|---|---|
| `off` | Skill inert | Default until armed |
| `lite` | One-liner answer + one-line caveat | Quick fixes, examples in chat |
| `full` (default) | Apply the 6 stufen strictly | Refactor + review phases |
| `ultra` | YAGNI extremist: deletion > addition, ask the rest | Cleanup of stale code |

`full` is the default when `lazy_skill` is armed without a qualifier.

## Decision Matrix

| Situation | Default intensity | Reason |
|---|---|---|
| Reviewing a PR diff | `full` | Catches over-engineering before merge |
| Refactoring a verified module | `lite` | Quick wins, no rewrites |
| Cleaning a deprecated subtree | `ultra` | Deletion is the answer |
| Writing a doc | `lite` | Prose is cheap |

## Token Budget

The skill's system-prompt block (when activated) is **byte-stable**
per (intensity, skillBody) pair:

| Intensity | Approx. bytes |
|---|---|
| `lite` | ~480 |
| `full` | ~1,260 |
| `ultra` | ~620 |

These budgets were measured on the four-arm benchmark
(`evals/three-arm-example.json`, issue #171) and serve as the upper
bounds in the system-prompt hash metric.

## Constraints

- **M3 is sacred.** Never offer a lazy version that has not passed
  verify.
- **M4 is sacred.** Never offer a lazy version that weakens the
  permission engine default policies.
- **M5 module path.** No `SIN-Code-Bundle` references — even in
  comments.
- **M6 SIN tools first.** Suggest `sin_edit` over `string-replace`,
  `SCKG` over blind `Read`, `EFM` over ad-hoc mocks — when the
  request explicitly calls for them. Don't replace a "make it work"
  request with "let me build an EFM sandbox".
- **Byte-stable render.** Keyword examples must produce identical
  bytes across runs. Use the templates verbatim.

## Quality Gates

- `python3 scripts/validate_skill.py --all-bundled --strict` passes.
- All five byte-stable examples in `SKILL.md` produce identical
  octets across CI runs (snapshot diff = 0).
- `go test -race -count=1 ./cmd/sin-code/internal/evalharness/...`
  passes — the four-arm harness must measure `lazy_skill` honestly
  versus `__terse__`.
- `instinct.Learner.BeforeTurn` logs the gate decision so reviewers
  can audit activation timing.

## Sin-Debt Marker Pair (issue #177)

Every shortcut **must** carry a paired marker:

```
// sin-debt: <ceiling — the simplification I made>,
// upgrade:   <event/concrete trigger — when I'll unsimplify>
```

Examples in `SKILL.md` show the canonical form:

```
// sin-debt: no graceful shutdown, upgrade: add ctx-aware http.Server when SIGTERM is wired
```

A `sin-debt` marker without a paired `upgrade:` is a CI-blocker
(`debt.max_no_trigger` from issue #177).

## Related Documents

- `tasks/workflow.md` — the 5-step activation + apply + verify cycle.
- `templates/output.md` — the "Code, three lines, done" output
  template.
- `templates/prompt.md` — the prompt snippet that arms the skill.
- `templates/intensities.md` — intensity-by-situations matrix.
- `templates/debt-markers.md` — copy/paste-ready `sin-debt` ceiling
  templates.
