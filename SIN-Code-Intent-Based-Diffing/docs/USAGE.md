# Usage — IBD

The package installs the `ibd` command.

## `ibd diff <file_a> <file_b>`

Semantic diff between two files.

```bash
ibd diff before.py after.py
ibd diff before.py after.py --json
```

The JSON output contains three sections:

```json
{
  "changes": [{"change_type": "signature_changed", "symbol": "function:foo", "severity": "high", ...}],
  "intents": [{"headline": "...", "category": "api_change", "risk": "high", ...}],
  "risk": {"score": 6.0, "risk": "high", "change_count": 2, "breakdown": {...}}
}
```

## `ibd review-git`

Analyze all uncommitted Python changes in the current Git repository
(working tree vs. `HEAD`).

```bash
ibd review-git
```

## Python API

```python
from sin_code_ibd import ASTDiff, IntentSummarizer, RiskScorer

changes = ASTDiff().diff_files("before.py", "after.py")
intents = IntentSummarizer().summarize(changes)
risk = RiskScorer().score(changes)
```

## Change types and severities

| change_type | severity | meaning |
|-------------|----------|---------|
| `signature_changed` | high | parameter list changed — may break callers |
| `decorators_changed` | medium | auth/caching/tracing may be affected |
| `removed` | medium | symbol deleted — verify no callers remain |
| `added` | info | new symbol introduced |
| `body_changed` | low | internal logic changed, public surface intact |
| `docstring_changed` | info | documentation only |
