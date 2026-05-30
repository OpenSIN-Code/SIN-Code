# Plan: SOTA Roadmap

Status: proposed
Owner: unassigned
Scope: cross-repo, medium-term

## Motivation

The stack currently leans heavily on static analysis. The highest-leverage
improvements for real-world agent coding quality lie in execution feedback,
verification oracles, budget-aware context selection, and security. This plan
captures those larger items as discrete, independently shippable workstreams so
they can be prioritized and assigned over time.

Each workstream is intentionally coarse; it will be broken into smaller issues
when picked up.

## Workstreams

### WS1 — Compiler/LSP as a primary correctness oracle (SCKG + Oracle)
Today the Verification-Oracle shells out to linters and runs tests. The
strongest, cheapest correctness signal — the compiler/type checker — is only
partially used, and SCKG re-implements navigation that Language Servers already
provide.
- Integrate a Language Server (pyright/tsserver/gopls) behind a stable adapter.
- Use LSP diagnostics as a first-class oracle signal.
- Let SCKG consume LSP `definition`/`references` instead of hand-rolled call
  resolution where available.

Value: high. Risk: medium (LSP lifecycle management).

### WS2 — Budget-aware context selection (SCKG)
SCKG can build the graph and answer `impact`/`downstream`, but cannot answer
"what is the minimal context this task needs under this token budget?".
- Add a relevance ranking (graph centrality + recency + task affinity).
- Add a `select-context --budget N` command returning a ranked, truncated set.

Value: high. Risk: medium.

### WS3 — Behavioral trace diffing (IBD + Oracle)
IBD diffs ASTs; it does not show what *behavior* changed. The Oracle already
has a trace-diff primitive — connect them.
- Capture execution traces (or structured logs) before/after a change.
- Produce a behavioral diff alongside the AST/intent diff.

Value: high. Risk: medium-high (trace capture is environment-specific).

### WS4 — Security scanning (ADW or new module)
No SAST, secret detection, dependency-vuln (SCA), or license checks exist.
- Integrate secret scanning, dependency vulnerability scanning, and a SAST pass.
- Surface findings through ADW's debt/score reporting and the Bundle CLI.

Value: high (production blocker). Risk: low-medium.

### WS5 — Eval harness expansion (Oracle)
The Oracle has a SWE-bench-style harness skeleton. Make it the meta-tool that
proves the rest of the stack helps.
- Add a curated task suite + scoring.
- Add regression evals runnable in CI.
- Report pass-rate deltas between versions.

Value: high (without this, "SOTA" is unmeasurable). Risk: medium.

### WS6 — Incremental graph updates (SCKG)
The graph is rebuilt from scratch each time, which will not scale.
- Add file-watcher / changed-files incremental updates.
- Persist and invalidate per-file subgraphs.

Value: medium-high. Risk: medium.

### WS7 — Polyglot parity (SCKG + IBD)
Parsing/diffing is Python-centric. Real repos are polyglot.
- Bring JS/TS (and one more language) to parity in both SCKG and IBD.
- Share a language-capability matrix in docs.

Value: medium. Risk: medium.

### WS8 — Persistent cross-task learning (Bundle)
Nothing remembers which solutions/patterns worked or failed.
- Add a lightweight store of task outcomes keyed by repo + task signature.
- Expose retrieval as an MCP tool the agent can consult.

Value: medium. Risk: medium-high (design-heavy).

### WS9 — Semantic merge for parallel agents (new)
When multiple agents work in parallel, naive merges conflict.
- Prototype conflict-aware merging at the symbol level (reuse IBD's AST diff).

Value: medium. Risk: high (research-y).

## Prioritization (suggested)

1. WS4 Security (production blocker, low risk)
2. WS5 Eval harness (makes everything else measurable)
3. WS1 LSP/compiler oracle (highest correctness leverage)
4. WS2 Budget-aware context
5. WS3 Behavioral trace diff
6. WS6 / WS7 (scaling & breadth)
7. WS8 / WS9 (research-heavy)

## Out of scope

- Operational/CI work — tracked in `docs/plans/operational-hardening.md`.
