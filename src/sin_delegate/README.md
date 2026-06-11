# sin-code-delegate

**Verification-gated sub-agent delegation for the SIN-Code stack.**

Most "delegate" tools fire-and-forget sub-agents into your repo and hope.
sin-code-delegate treats every sub-agent as **untrusted** and every run as
a **transaction**:

```
plan (DAG) ──> scheduler ──> per task:
worktree isolation ──> budgeted sub-agent ──> commit
──> verification gates (diff / tests / architecture)
──> merge-back saga (snapshot → rebase → ff-merge | compensate)
all of it event-sourced ──> resumable, auditable, cancellable
```

## Why it is better

| Naive delegation | sin-code-delegate |
| --- | --- |
| Sub-agents edit your branch directly | Isolated worktree per task; base branch is never dirty |
| Failures lose all progress | Event-sourced ledger; re-run resumes, DONE tasks skipped |
| Results trusted blindly | Gates: diff screen (secrets/eval), tests, ADW architecture |
| Retry = same prompt again | Gate verdict injected as feedback into the retry prompt |
| HIGH-risk failures retried forever | HIGH risk + failed gate = ESCALATE to human |
| Sequential or naïve parallel | Critical-path-first DAG scheduling + circuit breaker |
| Forgets everything | Pitfalls/decisions persisted to sin-brain, recalled next run |
| Single repo, race-conditions | Two-phase multi-repo commit (all-or-nothing atomic) |

## Install

```bash
pip install sin-code-delegate               # core (stdlib only)
pip install "sin-code-delegate[memory]"     # + sin-brain feedback loop
pip install "sin-code-delegate[all]"         # everything
```

## CLI

```shellscript
sin-delegate doctor --repo . --backends opencode,claude    # preflight
sin-delegate auto "Add rate limiting to all API routes" -y   # plan + execute
sin-delegate run plan.json --parallel 4                     # execute existing
sin-delegate watch  <plan_id>                                # live board
sin-delegate status <plan_id>                                # latest state
sin-delegate history <plan_id>                               # full event log
sin-delegate escalations <plan_id>                           # open decisions
sin-delegate resolve <plan_id> <esc_id> --option retry \
    --input "fix the migration order"                         # answer
sin-delegate resume <plan_id>                                # apply + continue
sin-delegate report  <plan_id>                               # markdown for PR
sin-delegate stats                                           # Wilson-scored backend performance
sin-delegate runs                                            # list known runs
sin-delegate cancel <plan_id>                                # cooperative cancel
```

## MCP tools (via `sin serve`)

- `sin_delegate` — submit a plan and run it
- `sin_delegate_status` / `sin_delegate_history` / `sin_delegate_cancel`
- `sin_delegate_escalations` / `sin_delegate_resolve`

## Python API

```python
from sin_delegate import Task, delegate

result = delegate(
    goal="Add rate limiting to the API",
    tasks=[
        Task(title="middleware",
             instructions="Add a token-bucket rate limit middleware."),
        Task(title="tests",
             instructions="Add tests for the middleware."),
    ],
    repo=".", max_parallel=2,
)
print(result.ok, result.to_json())
```

## Guarantees

1. **Base branch safety** — merge-back is a saga: snapshot tag before,
   hard restore on failure. Rebase conflicts preserve the task branch
   and escalate.
2. **Exactly-once tasks** — content-addressed task ids + ledger fold.
3. **Secret hygiene** — all sub-agent output is redacted before persistence.
4. **Graceful degradation** — missing pytest/go/sin/sin-brain never crash
   a run; the verdict honestly records what was skipped.
5. **Multi-repo atomicity** — Two-Phase-Commit ensures either every repo
   is merged or none is, even with N parallel branches.

---

MIT — part of the [SIN-Code](https://github.com/OpenSIN-Code) stack.
