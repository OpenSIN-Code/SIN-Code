# orchestrator/confidence.go

Calibrated Confidence — honest agent self-assessment. The agent declares
`P(my change passes)`; we score against reality (Brier) and learn a
per-agent calibration curve from history. Decisions use the CALIBRATED
probability, not the raw claim.

## Public surface

- `ConfidenceClaim{AgentName, TaskClass, Declared, Passed}`
- `Calibrator{db, binCount}`
  - `NewCalibrator(db) *Calibrator, error`
  - `Record(ctx, claim) error`
  - `Calibrate(ctx, agent, declared) (float64, error)` — empirical-Bayes
    shrinkage toward the agent's global pass rate
  - `BrierScore(ctx, agent) (score, n, error)` — calibration quality
- `MergePolicy{AutoMergeThreshold, ReviewThreshold}`
  - `DefaultMergePolicy() MergePolicy` — 0.85 / 0.6
  - `Decide(verified, calibrated) Decision` — auto-merge / review / block
- `Decision` — auto-merge, green-needs-review, block

## Behavior

- With `< 10` claims, the calibrator shrinks the claim halfway toward
  0.5 (maximum uncertainty) — conservative by construction.
- With `>= 10` claims, a local estimate (claims within ± one bin
  width of the declared value) is mixed with the global pass rate
  using `k = 10` pseudo-counts.
- `MergePolicy.Decide`: red always blocks; green + high calibrated
  confidence auto-merges; green + low confidence routes to review.

## Storage

- `confidence_claims` table: ~80 bytes per claim. Indexed on `agent`.
- No FTS — calibration is per-agent, not search-driven.
