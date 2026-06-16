# instinct/config.go — env-overridable thresholds

| Env var | Default | What |
|---|---|---|
| `SIN_INSTINCT_ACTIVATION` | 0.60 | pending → active cutoff |
| `SIN_INSTINCT_EVOLVE` | 0.70 | evolve eligibility |
| `SIN_INSTINCT_REINFORCE` | 0.25 | fraction of gap-to-max on Reinforce |
| `SIN_INSTINCT_CONTRADICT` | 0.40 | fraction of gap-to-floor on Contradict |
| `SIN_INSTINCT_PROMOTE_N` | 2 | min projects for promotion |
| `SIN_INSTINCT_TTL_DAYS` | 30 | pending TTL for prune |

## Why env-only (not file / not flag)

The instinct package is called from the hook dispatcher on every
tool call. Reading a TOML file or parsing flags would be visible
overhead. Env is set once at process start — overhead is amortized
to zero.

## Related files

- `tuning.go` — threads the loaded Config into the math
- `manager.go` — `NewManager` calls `LoadConfig`
