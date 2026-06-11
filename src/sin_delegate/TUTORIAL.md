# sin-code-delegate — The complete tutorial

From the first preflight check to the multi-repo release flow.
Each section is a complete, copy-pasteable workflow.

---

## 0. Installation & Preflight

```bash
pip install "sin-code-delegate[all]"      # + sin-brain + mcp
sin-delegate doctor --repo . --backends opencode,claude
```

Resolve every `✗` error before starting. Warnings (`⚠`) are advisory:
a dirty working directory is allowed, but risky — the worktrees are
based on the last commit of the base branch, not on your uncommitted
changes.

---

## 1. The fastest path: `auto`

One command, complete cycle — recon, plan, self-critique, execution,
report:

```shellscript
sin-delegate auto "Add input validation to all API route handlers" -y
```

What happens internally:

1. **RECON**: file tree, languages, test setup are deterministically
extracted (no LLM, no hallucination about your structure).
2. **DECOMPOSE**: The LLM produces a plan draft against the strict
schema; pitfalls from past runs are injected.
3. **CRITIQUE**: A second pass attacks the plan (write conflicts
without deps edges? missing deps? wrong risk levels?) and repairs.
4. **RUN**: Tasks run in parallel in isolated worktrees, gates
verify, merge sagas commit — or compensate.
5. **REPORT**: Markdown summary at the end.


---

## 2. The controlled path: plan → review → run

For anything important, you want the plan BEFORE execution:

```shellscript
sin-delegate plan "Migrate session handling from cookies to JWT" --out plan.json
$EDITOR plan.json        # inspect, adjust, sharpen risk levels
sin-delegate run plan.json --parallel 4
```

Tips for plan review:

- **`files` tight** — that is the sub-agent's write boundary.
`["src/auth/"]` is good, `["src/"]` is an invitation to chaos.
- **`risk: high` everywhere it hurts** — migrations, auth, payments,
deletion paths. HIGH tasks escalate on gate failure instead of blindly
retrying, and the policy NEVER explores alternative backends there.
- **deps edges between tasks touching the same paths** — the
critique pass catches most of it, but you know your repo better.


---

## 3. Live monitoring

Second terminal, while the run is active:

```shellscript
sin-delegate watch <plan_id>
#   ✓ done       Add validation schema module
#   ▶ running    Wire validation into user routes
#   ▣ verifying  Wire validation into billing routes
#   · pending    Update API docs
#   2/4 terminal
```

The board reads only the ledger — safe from any process, anytime.

---

## 4. When something escalates

HIGH-risk task tore the gates, or a merge conflict:

```shellscript
sin-delegate escalations <plan_id>
# [a1b2c3d4e5f6] gate_failure  task: Migrate sessions table
#   HIGH-risk task failed gates: failed gates: tests
#   branch: sin/delegate/<plan>/<task>
#     --option retry    Retry with correction hint (requires --input)
#     --option accept   Accept result despite gate failure
#     --option drop     Discard task
#     --option abort    Abort entire plan
```

Decide and continue:

```shellscript
# Variant A: You know what's wrong -> targeted retry
sin-delegate resolve <plan_id> a1b2c3d4e5f6 --option retry \
    --input "The test fails because the backfill must run BEFORE the column drop."

# Variant B: You check the branch yourself and accept
git log sin/delegate/<plan>/<task>
sin-delegate resolve <plan_id> a1b2c3d4e5f6 --option accept

# Then always:
sin-delegate resume <plan_id>
```

The resume applies all resolutions (retry tasks become PENDING with
injected guidance, accepted branches get merged) and continues the
DAG exactly where it stood.

---

## 5. Crash? No problem.

Machine crashed, SSH died, `kill -9` — the ledger has everything:

```shellscript
sin-delegate runs                 # find your plan_id
sin-delegate resume <plan_id>     # DONE tasks are skipped
```

---

## 6. Multi-repo release flow

Feature across API, frontend, and docs — atomic or not at all:

```shellscript
cat > release.json << 'EOF'
{
  "goal": "Ship the new export feature end to end",
  "repos": {
    "api":  { "path": "../export-api" },
    "web":  { "path": "../export-web" },
    "docs": { "path": "../docs" }
  },
  "tasks": [
    { "key": "endpoint", "repo": "api",
      "title": "Implement /v1/exports endpoints",
      "instructions": "POST /v1/exports (create job) and GET /v1/exports/{id}. Export route signatures and types as a contract.",
      "files": ["src/routes/exports/"], "risk": "high" },
    { "key": "ui", "repo": "web", "deps": ["endpoint"],
      "title": "Add export button + status page",
      "instructions": "Use exactly the endpoint signatures from the upstream contract.",
      "files": ["app/exports/"], "risk": "medium" },
    { "key": "ref", "repo": "docs", "deps": ["endpoint"],
      "title": "API reference for exports",
      "instructions": "Document the endpoints based on the upstream contract.",
      "files": ["api-reference/"], "risk": "low", "verify": ["diff"] }
  ]
}
EOF

sin-delegate run release.json --parallel 3
```

Two-Phase-Commit guarantees:

- **Phase 1** (parallel): all tasks implement + verify in worktrees.
The `endpoint` task exports a `<sin-contract>`; `ui` and `ref` get the
signatures injected as FACTS. NOBODY merges.
- **Phase 2** (serial, only if all green): snapshot tags in all
repos, then merges in topo order. If ONE fails, ALL repos are reset
to their snapshots. No half-release, ever.


---

## 7. The system gets smarter: stats

After a few runs:

```shellscript
sin-delegate stats
# task_class               backend   model               n  pass  wilson  ~sec
# high:py:arch+diff+tests  claude    claude-sonnet-4-5  12   92%   0.646   412
# high:py:arch+diff+tests  opencode  (default)           9   67%   0.354   388
# low:ts:diff+tests        opencode  (default)          31   97%   0.891    95
```

The policy uses this data automatically: future tasks of class
`high:py:...` get routed to claude (Wilson-score, not naive pass rate
— 1/1 lucky shots don't beat 47/50). For LOW risk, 10% exploration
probability so the statistics don't ossify. Pin a backend explicitly
in the plan (`"backend": "...", "model": "..."`) and the policy
respects it always.

---

## 8. As an MCP tool in the parent agent

One entry in the agent config:

```json
{ "mcpServers": { "sin-code-delegate": { "command": "sin-delegate", "args": ["serve"] } } }
```

The parent agent can then delegate autonomously:

1. `sin_delegate` — submit and run a plan
2. `sin_delegate_status` / `sin_delegate_history` — inspect progress
3. `sin_delegate_escalations` + `sin_delegate_resolve` — make decisions
4. `sin_delegate_cancel` — cooperative shutdown

---

## 9. Troubleshooting

| Symptom | Diagnosis | Fix
|-----|-----|-----
| `agent exited 127` | Backend CLI missing | `sin-delegate doctor --backends ...`
| Task fails with "changed nothing" | Instructions too vague / files too tight | Sharpen plan, widen `files`
| Everything is SKIPPED | Upstream task failed | `sin-delegate history <id>` for root cause
| `rebase conflict` escalation | Two tasks, same file, no deps edge | Add deps; resolve branch manually (`--option manual`)
| Run aborts early | Circuit breaker (>50% failures) | Check plan quality, read `history`
| `global budget exhausted` | `--budget` too small | Increase budget or split tasks
| `ledger corrupt` | Disk full / interrupted write | `rm ~/.sin-code/delegate/ledger.db` (loses history)
