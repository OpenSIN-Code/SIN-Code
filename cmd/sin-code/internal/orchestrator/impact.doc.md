# orchestrator/impact.go

Impact Oracle — predict blast-radius BEFORE editing. Builds a reverse-
dependency graph from `go list -json ./...` and answers "if these files
change, which packages and which tests are affected?".

## Public surface

- `PkgNode{ImportPath, Dir, GoFiles, TestFiles, Imports}`
- `ImpactGraph{repoRoot, nodes, reverse, fileToPkg}`
  - `BuildImpactGraph(ctx, repoRoot) *ImpactGraph, error`
  - `Predict(changedFiles) *Impact` — graph walk, microseconds
- `Impact{ChangedPkgs, AffectedPkgs, AffectedTestPkgs, Radius}`
  - `RiskBrief() string` — planner-readable blast summary

## Behavior

- `BuildImpactGraph` shells out to `go list` once (~100ms-2s).
  Inverted edges are restricted to in-repo packages — vendor/stdlib
  edges are dropped to keep the closure tight.
- `Predict` is a BFS over the reverse graph. Pure read, microseconds.
- `Radius = |AffectedPkgs| / |all packages|`. Above 0.5 the brief
  emits a WARNING suggesting interface-preserving edits.

## Empty-repo mode

- `BuildImpactGraph(ctx, "")` returns an empty graph (no `go list`).
  Useful for tests and for "no go.mod here" workspaces.
- `Predict` on an empty graph returns `Radius = 0`.
