# Template: Intensity Decision Matrix

Docs: ../SKILL.md

A pure-stdlib decision aid for picking `lite | full | ultra` at the
moment of arming. Deterministic — picks the lowest intensity that
satisfies the situation.

## Decision Table

| Situation | Default intensity | Reason |
|---|---|---|
| Reviewing a PR diff | `full` | Catches over-engineering before merge |
| Refactoring a verified module | `lite` | Quick wins, no rewrites |
| Cleaning a deprecated subtree | `ultra` | Deletion is the answer |
| Writing a doc | `lite` | Prose is cheap |
| Answering a one-shot question | `lite` | One-liner suffice |
| Auditing a service config | `full` | Tighten defaults, keep shapes |
| Replacing a library outright | `ultra` | Delete the wrapper layer |
| Hotfix under time pressure | `off` | Verify first, lazy later |

## Pure-stdlib selector (Python, no third-party deps)

```python
import os
import json
from pathlib import Path

def resolve_intensity(situation: str, user_pref: str = "") -> str:
    """Resolve intensity in the canonical 3-step order:

    1. SIN_LAZY_DEFAULT env var (highest precedence).
    2. user_pref (caller's explicit choice, e.g. /lazy lite).
    3. Situation table default.
    4. 'off' (fail-safe).
    """
    env = os.environ.get("SIN_LAZY_DEFAULT", "").strip().lower()
    if env in ("off", "lite", "full", "ultra"):
        return env
    if user_pref in ("off", "lite", "full", "ultra"):
        return user_pref
    table = {
        "review": "full",
        "refactor": "lite",
        "cleanup": "ultra",
        "docs": "lite",
        "hotfix": "off",
    }
    return table.get(situation, "off")


def load_user_pref(path: Path) -> str:
    """Read ~/.config/sin-code/active-rules.json → 'lazy' entry.

    File format:
        {"lazy": "lite|full|ultra|off"}
    """
    if not path.is_file():
        return ""
    try:
        data = json.loads(path.read_text())
        v = str(data.get("lazy", "")).strip().lower()
        return v if v in ("off", "lite", "full", "ultra") else ""
    except json.JSONDecodeError:
        return ""


if __name__ == "__main__":  # pragma: no cover
    user_pref = load_user_pref(Path.home() / ".config/sin-code/active-rules.json")
    print(resolve_intensity("review", user_pref))
```

The selector is byte-stable: same inputs ⇒ same output, every run.
This is what feeds the issue #2 hash metric.
