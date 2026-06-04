# Configuration — POC

POC needs no configuration file. Behavior is controlled by CLI flags and the
Python API.

## CLI flags

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `poc verify` | `--max-examples` | 50 | Max Hypothesis examples per property (capped internally for speed). |

## Property selection heuristics

`PropertyGenerator.suggest()` chooses candidate properties from the function
signature and name:

- numeric return (`int`/`float`) → `non_negative_output`, and `monotonic` if a
  single numeric parameter is present.
- collection return (`list`/`tuple`/`str`) → `length_preserving`.
- name contains `sort` → `sorted_output`.
- name contains `encode`/`decode` → `reversible`.
- name contains `normalize`/`canonical` → `idempotent`.
- purity (`pure`) is always included.

## Customizing strategies

Hypothesis strategies are resolved in `RuntimeVerifier._strategy_for`. To extend
or override the mapping, subclass `RuntimeVerifier` and provide your own
strategy resolution.

## Safety

POC does **not** `eval` generated strings. Each property carries a real Python
`predicate` callable, so verification cannot execute arbitrary code paths beyond
calling the target function.
