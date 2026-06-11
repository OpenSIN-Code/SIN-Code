# orchestrator/strategy_router.go

Adaptive Tool-Strategy selection via Thompson sampling. Per
`(task-class, strategy)` Beta posterior learned from verified outcomes.
No epsilon, no decay — exploration and exploitation in one mechanism.

## Public surface

- `Strategy` (ast-edit, hashline-patch, full-rewrite, shell-codegen)
- `TaskClass` (rename, refactor, bugfix, greenfield, config, unknown)
- `ClassifyTask(task) TaskClass` — simple keyword-based classification
- `StrategyRouter{arms, db, rng}`
  - `NewStrategyRouter(db, seed) *StrategyRouter, error`
  - `Pick(class, candidates) Strategy` — Thompson draw
  - `Report(ctx, class, strategy, success) error` — feed back outcome
  - `Posterior(class, strategy) (mean, n)` — for TUI display

## Behavior

- Default arms: `α=β=1` (uniform prior).
- A `*sql.DB` is optional — with `nil` DB the router operates in
  in-memory mode (useful for tests and ephemeral sessions); the
  `router_arms` schema is only created when a DB is provided.
- `Posterior` returns `α / (α+β)` and the effective sample size
  (`α+β-2`).

## Why Thompson sampling?

- Simple Bayesian regret bound.
- No hyperparameters (no `ε`, no decay schedule).
- Naturally balances exploration of under-sampled arms and exploitation
  of well-performing arms.

## Caveats

- The `math/rand` RNG is non-cryptographic. Use a seeded `*rand.Rand`
  for reproducible agent runs.
- Classification is deliberately simple — misclassification only slows
  learning (the wrong arm is updated less), never breaks it.
