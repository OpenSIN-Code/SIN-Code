# AGENTS.md — SIN-Code Engineering Doctrine

> This file is binding for every AI coding agent (opencode, Codex CLI, Hermes,
> Cursor, Amp, Gemini CLI, …) working in this repository. The closest AGENTS.md
> to an edited file wins; an explicit user instruction overrides this file.

## What this project gives you

You have access to the **SIN-Code MCP server** (`sin serve`). It exposes signals
that an LLM cannot infer from source text alone. Use them — do not guess where a
tool exists.

| Tool | Use it to … | Call it … |
|------|-------------|-----------|
| `impact(symbol_fqid)` | See the blast radius of a symbol before touching it | BEFORE editing |
| `semantic_diff(file_a, file_b)` | Understand *intent* + risk of a change | AFTER editing |
| `semantic_review(file_a, file_b)` | Intent + risk + recommendation in one call | AFTER editing |
| `architectural_debt()` | Read current complexity/debt score | BEFORE + AFTER |
| `prove(function_code, properties)` | Generate a proof of correctness | for risky pure logic |
| `verify_tests(code, language)` | Independent security/perf/correctness check | before "done" |
| `mock_env(action, port)` | Spin up an ephemeral full-stack mock | for integration work |

## The non-negotiable loop

For **every** code-changing task, follow this loop. Do not skip steps.

1. **Orient.** Run `sin status` mentally — only call tools whose subsystem is
   available. If `.sin/` does not exist, run `sin bootstrap .` first.
2. **Assess impact.** Before editing a symbol, call `impact(<fqid>)`. If the
   blast radius touches public APIs, tests, or > 5 callers, widen your plan.
3. **Edit minimally.** Make the smallest change that satisfies the task.
4. **Review the change.** Call `semantic_review(before, after)` (or
   `semantic_diff`). If `risk` is not `low`, re-read the diff and justify it.
5. **Guard debt.** Call `architectural_debt()`. If the score regressed past the
   ADW breaker, STOP and refactor instead of piling on.
6. **Verify.** Call `verify_tests` (and `prove` for critical pure functions).
   **You MUST NOT report a task as done while verification is red.**

## Hard rules

- **Never claim "done" without a green verification.** A clean compile or
  "no lint errors" is not verification.
- **Never bypass the ADW cost/complexity breaker.** A tripped breaker means the
  approach is wrong, not that the breaker is wrong.
- **Prefer `impact` over grep** for "what calls this?" — it is structural, not
  textual.
- **One concern per change.** If `semantic_diff` reports multiple unrelated
  intents, split the change.
- **Read the nearest AGENTS.md** in subdirectories; it overrides this one.

## Dev environment

- Python **3.11+**. Install: `pip install -e ".[mcp]"`.
- Per-repo init once: `sin bootstrap .` (writes `.sin/`).
- Start tools for your agent: `sin serve` (stdio MCP).

## Testing instructions

- Run the suite with `pytest -q` and fix every failure before finishing.
- For any code you generate, run `verify_tests` through the MCP server.
- Add or update tests for code you change, even if nobody asked.

## PR instructions

- Title format: `[sin-code-bundle] <Title>`.
- Before committing: `pytest -q` is green AND the MCP `verify_tests` verdict is
  `pass`.
- Summarize the `semantic_review` intents + final `architectural_debt` delta in
  the PR description.
