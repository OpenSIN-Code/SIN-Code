"""Purpose: CWE Top 25 lookup for CEO Audit.

Reference: https://cwe.mitre.org/top25/
Updated for 2023 list.

Docs: cwe.doc.md
"""

from __future__ import annotations

# ── Module overview ──────────────────────────────────────────────────
#
# Provides three helpers used by score.py + report.py:
#
#   - is_top25(cwe)   — True if CWE is in the 2023 Top 25
#   - rank(cwe)       — 1-25 rank, or 0 if not in the list
#   - category(cwe)   — group label for executive-summary charts
#                       (Memory Safety / Web/XSS / etc.)
#
# Top-25 status feeds into score.py's CWE_TOP25_IMPACT multiplier
# (1.5x for findings tagged with a Top-25 CWE). The category() function
# powers report.py's compliance breakdown chart.
#
# Maintenance: refresh CWE_TOP25_2023 annually when MITRE publishes
# the new Top 25 list. Keep the order — index = 1-based rank.
# ─────────────────────────────────────────────────────────────────────


# ── 2023 CWE Top 25 (ranked by prevalence + impact) ──────────────────
# Ordering matters: index 0 = CWE rank 1 (most-exploited 2023). Source
# is MITRE's annual analysis of NVD entries; the list shifts ~3-5
# positions per year, so refresh annually.
CWE_TOP25_2023 = [
    "CWE-787",  # 1. Out-of-bounds Write
    "CWE-79",  # 2. Improper Neutralization of Input During Web Page Generation (XSS)
    "CWE-89",  # 3. Improper Neutralization of SQL Commands
    "CWE-20",  # 4. Improper Input Validation
    "CWE-125",  # 5. Out-of-bounds Read
    "CWE-78",  # 6. OS Command Injection
    "CWE-416",  # 7. Use After Free
    "CWE-22",  # 8. Path Traversal
    "CWE-352",  # 9. CSRF
    "CWE-434",  # 10. Unrestricted Upload of File with Dangerous Type
    "CWE-862",  # 11. Missing Authorization
    "CWE-476",  # 12. NULL Pointer Dereference
    "CWE-287",  # 13. Improper Authentication
    "CWE-190",  # 14. Integer Overflow
    "CWE-502",  # 15. Insecure Deserialization
    "CWE-77",  # 16. Command Injection (parent of 78)
    "CWE-119",  # 17. Buffer Errors (parent of 125, 787)
    "CWE-798",  # 18. Hardcoded Credentials
    "CWE-918",  # 19. SSRF
    "CWE-306",  # 20. Missing Authentication for Critical Function
    "CWE-362",  # 21. Concurrent Execution using Shared Resource without Proper Synchronization
    "CWE-269",  # 22. Improper Privilege Management
    "CWE-94",  # 23. Code Injection
    "CWE-863",  # 24. Incorrect Authorization
    "CWE-276",  # 25. Incorrect Default Permissions
]

# ── Quick lookup index (CWE → 1-based rank) ──────────────────────────
# Built once at import-time so is_top25/rank are O(1).
_CWE_INDEX = {cwe: i + 1 for i, cwe in enumerate(CWE_TOP25_2023)}


def is_top25(cwe: str) -> bool:
    """Return True if `cwe` is in the 2023 CWE Top 25 list."""
    # Top-25 status raises a finding's impact score in score.py.
    return cwe in _CWE_INDEX


def rank(cwe: str) -> int:
    """Return the 1-25 rank of `cwe` in the 2023 Top 25, or 0 if not in the list."""
    return _CWE_INDEX.get(cwe, 0)  # 0 = not in Top 25


def category(cwe: str) -> str:
    """High-level category for a CWE."""
    # Groups related CWEs for executive-summary charts (report.py).
    # Categories chosen to align with OWASP Top 10 narrative.
    # When a new CWE is added, classify under the closest existing bucket;
    # only invent a new category if NO existing bucket fits.
    categories = {
        # Memory Safety: out-of-bounds reads/writes, use-after-free, buffer errors.
        "CWE-787": "Memory Safety",
        "CWE-125": "Memory Safety",
        "CWE-416": "Memory Safety",
        "CWE-119": "Memory Safety",
        # Web/XSS: cross-site scripting (output encoding).
        "CWE-79": "Web/XSS",
        # Web/Injection: SQL, command, code, deserialisation.
        "CWE-89": "Web/Injection",
        "CWE-78": "Web/Injection",
        "CWE-77": "Web/Injection",
        "CWE-94": "Web/Injection",
        "CWE-502": "Web/Injection",
        # Input Validation: missing/insufficient input checks.
        "CWE-20": "Input Validation",
        # Path/File: traversal + unrestricted upload.
        "CWE-22": "Path/File",
        "CWE-434": "Path/File",
        # Web/CSRF: cross-site request forgery.
        "CWE-352": "Web/CSRF",
        # Auth/Authz: missing/incorrect authentication or authorisation.
        "CWE-862": "Auth/Authz",
        "CWE-863": "Auth/Authz",
        "CWE-287": "Auth/Authz",
        "CWE-306": "Auth/Authz",
        "CWE-269": "Auth/Authz",
        "CWE-276": "Auth/Authz",
        # Null Pointer / Integer math: classic memory/numeric bugs.
        "CWE-476": "Null Pointer",
        "CWE-190": "Integer/Math",
        # Credentials / SSRF / Concurrency: specialised single-issue buckets.
        "CWE-798": "Credentials",
        "CWE-918": "SSRF",
        "CWE-362": "Concurrency",
    }
    # "Other" is a deliberate catch-all so report.py never has a missing label.
    return categories.get(cwe, "Other")


if __name__ == "__main__":
    # CLI: prints the Top 25 with category — `python3 cwe.py`.
    if __name__ == "__main__":
        for i, cwe in enumerate(CWE_TOP25_2023, 1):
            print(f"  {i:2d}. {cwe} ({category(cwe)})")
