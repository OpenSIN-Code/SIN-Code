# lib/owasp_asvs.py

OWASP ASVS v5.0 chapter/requirement mapping for CWE ids. Used to enrich
audit findings with a compliance pointer so reviewers can show
"`this finding violates ASVS V5.3`" instead of just a CWE number.

## Dependencies

- stdlib only (no third-party deps)

## Touched by

- `lib/cwe.py` — sibling mapping (CWE ↔ CWE rank)
- `scripts/report.py` — emits the `OWASP ASVS v5.0` coverage row in
  the report's Compliance section

## What it does

Exposes:

- **`CWE_TO_ASVS`** — `{cwe_id: "V<n>.<m>"}` for ~20 high-impact
  CWEs (injection, auth, crypto, SSRF, …).
- **`lookup(cwe) -> str`** — returns the ASVS chapter reference
  (e.g. `"V5.3"`) or `""` if unmapped.
- **`all_asvs_requirements() -> list[dict]`** — the 14 ASVS v5.0
  chapter stubs (id + name) for reference.

## Important config

- The mapping is **deliberately short** (20 entries) — it covers the
  CWEs the CEO Audit axes actually emit. New CWEs need new entries
  here.
- ASVS chapter ids follow the `V<n>.<m>` notation from the official
  v5.0 document (https://owasp.org/www-project-application-security-verification-standard/).

## Usage

```python
from lib.owasp_asvs import lookup, all_asvs_requirements

print(lookup("CWE-89"))  # "V5.3"
print(lookup("CWE-999"))  # ""
for ch in all_asvs_requirements():
    print(ch["id"], ch["name"])
```

## Known caveats

- Only 20 CWEs are mapped; the full ASVS v5.0 has hundreds of
  requirements. For a complete mapping, pull from the official JSON
  artifact.
- The mapping is **CWE → chapter**, not CWE → specific requirement.
  A finding mapped to `V5.3` could violate any of `V5.3.1` …
  `V5.3.10`; manual review is still required.
- The CLI main block dumps the whole map; useful for ad-hoc lookups
  but not for production use.
