# SWE-bench Results

## Overview

SWE-bench measures an agent's ability to fix real GitHub issues from popular
Python repositories. SIN-Code's verification-first approach (mandate M3) gives
it a unique advantage: the agent must prove correctness before reporting
success, which maps directly to the SWE-bench evaluation protocol.

## Latest Results

| Benchmark | Pass Rate | Instances | Date | Model |
|---|---|---|---|---|
| SWE-bench Lite | _pending_ | 300 | — | — |
| SWE-bench Verified | _pending_ | 500 | — | — |

## Comparison

| Agent | SWE-bench Lite | SWE-bench Verified |
|---|---|---|
| **SIN-Code** | _pending_ | _pending_ |
| OpenHands (CodeAct 2.1) | 77.6% | — |
| Codex CLI | _pending_ | — |
| Claude Code | _pending_ | — |

## Running

```bash
# Run the full SWE-bench evaluation
sin-code eval swebench \
  --dataset evals/swebench-lite.json \
  --output swebench-results.json \
  --workspace /tmp/swe-workspace \
  --max-turns 200

# Validate the dataset without running agents
sin-code eval swebench \
  --dataset evals/swebench-lite.json \
  --dry-run
```

## CI

A nightly n8n-delegated run is configured in `.github/workflows/swebench-nightly.yml`.
Results are published to GitHub Releases as `swebench-results.json`.

## Methodology

1. Clone the repository at the base commit
2. Run `sin-code -p "<issue description>"` with the agent loop
3. Extract the git diff patch from the agent's changes
4. Apply the patch and run `test_patch` from the SWE-bench dataset
5. Record pass if all FAIL_TO_PASS tests pass and PASS_TO_PASS tests still pass

## References

- [SWE-bench](https://www.swebench.com/)
- [OpenHands SWE-bench Results](https://github.com/All-Hands-AI/OpenHands)
- [Issue #363](https://github.com/OpenSIN-Code/SIN-Code/issues/363)
