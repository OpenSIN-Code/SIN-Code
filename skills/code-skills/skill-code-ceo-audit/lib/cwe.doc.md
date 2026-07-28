# lib/cwe.py

CWE Top 25 lookup table (2023 edition) used by the CEO Audit axes and
the scoring engine to enrich findings with rank, category, and
"is-in-Top-25" metadata.

## Dependencies

- stdlib only (no third-party deps)

## Touched by

- `lib/owasp_asvs.py` — sibling mapping, same purpose
- `scripts/score.py` — uses `CWE_TOP25_IMPACT` (its own copy) to bump
  the risk multiplier for Top-25 entries
- Every axis script that wants to print a CWE category

## What it does

Exposes:

- **`CWE_TOP25_2023`** — ordered list of the 25 CWEs, ranked by
  prevalence + impact (per the 2023 MITRE update).
- **`_CWE_INDEX`** — reverse-lookup `{cwe_id: rank}` for O(1) `rank()`
  and `is_top25()`.
- **`is_top25(cwe) -> bool`** — membership test.
- **`rank(cwe) -> int`** — 1-25 if Top-25, else 0.
- **`category(cwe) -> str`** — high-level bucket
  (`"Memory Safety"`, `"Web/XSS"`, `"Web/Injection"`,
  `"Input Validation"`, `"Path/File"`, `"Web/CSRF"`,
  `"Auth/Authz"`, `"Null Pointer"`, `"Integer/Math"`,
  `"Credentials"`, `"SSRF"`, `"Concurrency"`, `"Other"`).

## Important config

- The 2023 list is **frozen at module load**; the 2024 update
  (when MITRE publishes it) requires a manual edit + version bump.
- `_CWE_INDEX` is recomputed at import; do not mutate it at runtime.

## Usage

```python
from lib.cwe import is_top25, rank, category, CWE_TOP25_2023

print(is_top25("CWE-89"))        # True
print(rank("CWE-89"))            # 3
print(category("CWE-22"))        # "Path/File"
print(CWE_TOP25_2023[0])         # "CWE-787"
```

## Known caveats

- Categories are coarse-grained on purpose (12 buckets). For finer
  granularity, cross-reference the CWE entry in the MITRE database.
- The list tracks the **2023** Top 25; new top entries (e.g. supply
  chain) are not auto-included.
- `category()` returns `"Other"` for any unknown CWE — there is no
  fuzzy matching.
