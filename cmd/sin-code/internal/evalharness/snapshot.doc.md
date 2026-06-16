# evalharness/snapshot.go — byte-stable matrix

`BuildSnapshot`, `WriteSnapshotFile`, `LoadSnapshotFile`, and
`DiffSnapshots` together implement the caveman evals/README.md §3
promise: "snapshot committed to git so CI runs are deterministic
and free".

## Round-trip contract

`WriteSnapshot` produces `bytes A` for `Run X`. `LoadSnapshot(A)`
parses `Run X` back. `WriteSnapshot(parsed(X))` produces
`bytes A` again. We test this directly in
`comparator_test.go:TestSnapshotRoundTrip`.

## Schema

`SnapshotSchemaVersion` is bumped on every column change.
`LoadSnapshot` rejects unknown versions so reviewers see a
clean error rather than garbage data.

## Per-arm row

```json
{
  "arm_id": "skill-code-create",
  "total_cases": 3,
  "passed": 3,
  "median_loc": 12,
  "median_latency_ms": 240,
  "median_usd": 0.000456,
  "median_tokens": 360,
  "median_score": 1.0,
  "pass_rate": 1.0,
  "weighted_score": 1.0,
  "skill_name": "skill-code-create",
  "system_prompt_hash": "d6e8116a"
}
```

`system_prompt_hash` is a 32-bit FNV-1a fingerprint rendered as
8 lower-hex chars — enough for the diff to surface "skill body
changed" without storing the prompt itself.

## Diff

`DiffSnapshots(A, B) → []SnapshotRowDelta` produces a per-arm
delta. `Kind` is one of `added-B`, `removed-A`, `changed`, or
`changed-skill-body`. The last one is the high-signal change:
the SKILL.md moved but the medians didn't.

## Related files

- `comparator.go` — produces `CompareReport` that
  `BuildSnapshot` consumes.
- `prices.go` — `Price` entries that turn tokens into USD on the
  row's `median_usd` field.
