# Changelog

## 0.1.0

Initial release of sin-code-delegate.

### Core
- Event-sourced run ledger (SQLite WAL): crash-safe, resumable, auditable
- DAG scheduler: critical-path-first, readiness-driven (no wave blocking),
  circuit breaker, cooperative cancellation
- Content-addressed task ids: identical plans resume instead of redo
- Git worktree isolation per task; merge-back as saga
  (snapshot tag -> rebase -> ff-merge | compensate)
- Verification gates: diff screen (secrets/eval), project-aware tests
  (pytest/go/npm), ADW architecture check — graceful degradation, honest
  verdicts about skipped gates
- Pluggable runner backends: opencode, claude, codex, generic command;
  secret redaction on all sub-agent output

### Intelligence
- LLM planner: deterministic repo recon -> decompose -> self-critique pass;
  robust JSON extraction from noisy agent output
- Cross-run analytics: Wilson-score-ranked backend performance per task
  class, EMA runtime estimates
- Policy layer: learned backend routing, epsilon-greedy exploration
  (LOW risk only, HIGH risk never), pinned specs always respected
- Adaptive budget governor: informed seeding, surplus recycling,
  critical-path-weighted extensions, deadline pressure
- sin-brain memory loop: pitfalls and decisions persisted and recalled

### Operations
- Escalation protocol: typed options with explicit consequences,
  idempotent resolution, deterministic resume application
- Multi-repo delegation: two-phase commit across repositories
  (all-or-nothing with global snapshot rollback), cross-repo contracts
- Live status board, Markdown run reports, full audit history
- `sin-delegate doctor` preflight check
- `python -m sin_delegate` standalone MCP server
- Bundle integration ready: `pyproject.toml` entry-points for
  `sin_code.subcommands` and `sin_code.mcp_tools` (fail-open plugin
  isolation)

### Quality
- 44 tests across 4 suites
  - `test_delegate.py` (10) — core invariants
  - `test_intelligence_multirepo.py` (20) — analytics, policy, governor,
    escalation, resolution, multirepo, doctor
  - `test_scheduler_properties.py` (6) — property-based scheduler tests
    over randomized DAGs (dependency order, completeness, failure
    propagation, parallelism bound, resume idempotence, critical-path
    dominance)
  - `test_integration_e2e.py` (7) — end-to-end with real git repos,
    crash recovery, resume semantics
- Zero external dependencies (optional: `sin-brain`, `mcp`)
- Python 3.11+ stdlib only
