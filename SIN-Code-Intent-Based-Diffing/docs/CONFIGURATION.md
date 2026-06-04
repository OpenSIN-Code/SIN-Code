# Configuration — IBD

IBD is **stateless** and needs no configuration file. Behavior is controlled
entirely through CLI flags and the Python API.

## CLI flags

| Command | Flag | Default | Description |
|---------|------|---------|-------------|
| `ibd diff` | `--json` | off | Emit machine-readable JSON instead of rich text. |

## Tuning the risk model (programmatic)

Risk weights live in `RiskScorer.WEIGHT`. To customize, subclass or override:

```python
from sin_code_ibd import RiskScorer

scorer = RiskScorer()
scorer.WEIGHT["signature_changed"] = 5.0   # treat API breaks as more severe
```

Default weights:

| change_type | weight |
|-------------|--------|
| `signature_changed` | 3.0 |
| `removed` | 2.0 |
| `decorators_changed` | 2.0 |
| `body_changed` | 1.0 |
| `added` | 0.5 |
| `docstring_changed` | 0.2 |

Thresholds: score `< 1` → low, `< 5` → medium, otherwise high.

## Language support

The AST diff currently targets **Python**. Other languages fall back to no
detected symbols (empty diff) rather than erroring.
