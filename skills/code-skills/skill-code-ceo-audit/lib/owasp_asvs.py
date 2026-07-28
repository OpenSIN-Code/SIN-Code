"""Purpose: OWASP ASVS v5.0 mapping for CEO Audit findings.

Maps CWE IDs to ASVS chapters/sections. Use to enrich audit findings
with compliance context.

Docs: owasp_asvs.doc.md
"""

from __future__ import annotations

# ── Module overview ──────────────────────────────────────────────────
#
# Two public helpers:
#
#   - lookup(cwe)              — return ASVS chapter ref (e.g., "V5.3")
#                                or "" if unmapped
#   - all_asvs_requirements()  — return the 14-chapter ASVS v5.0 list
#                                ([{id, name}, ...])
#
# Used by report.py's "Compliance" section to enrich each CWE-tagged
# finding with the matching ASVS chapter. Used by external compliance
# auditors to map ceo-audit output onto ASVS audit requirements.
#
# Maintenance: refresh CWE_TO_ASVS when:
#   - ASVS releases a major (v5.0 → v6.0): re-map all chapters
#   - New CWE entries from MITRE: add rows as needed
# ─────────────────────────────────────────────────────────────────────


# ── CWE → ASVS v5.0 mapping (selected high-impact entries) ────────────
# Each row maps a Common Weakness Enumeration ID to its primary
# ASVS chapter. ASVS chapters are versioned (V5.0 here) and shift
# between releases — refresh this table when ASVS bumps a major.
# Source: https://owasp.org/www-project-application-security-verification-standard/
CWE_TO_ASVS = {
    "CWE-22": "V12.3",  # File handling — path traversal, directory access
    "CWE-78": "V5.3",  # OS command injection — exec/system call sanitisation
    "CWE-79": "V5.3",  # XSS — output encoding
    "CWE-89": "V5.3",  # SQL injection — parameterised queries
    "CWE-94": "V5.3",  # Code injection — eval / template injection
    "CWE-259": "V2.10",  # Hardcoded credentials (password literals)
    "CWE-287": "V2.10",  # Authentication bypass — weak auth logic
    "CWE-306": "V2.10",  # Missing authentication for critical functions
    "CWE-327": "V6.2",  # Weak crypto (MD5, SHA-1, DES, ECB)
    "CWE-338": "V6.2",  # Insecure random (Math.random for crypto)
    "CWE-352": "V4.2",  # CSRF — missing anti-forgery tokens
    "CWE-434": "V4.2",  # Unrestricted file upload
    "CWE-476": "V5.3",  # NULL pointer dereference (input validation)
    "CWE-502": "V5.5",  # Insecure deserialization (pickle, yaml.load)
    "CWE-601": "V7.4",  # Open redirect — unvalidated redirect URLs
    "CWE-798": "V2.10",  # Hardcoded credentials (API keys, secrets)
    "CWE-862": "V4.2",  # Missing authorization checks
    "CWE-863": "V4.2",  # Incorrect authorization (broken access control)
    "CWE-918": "V12.6",  # SSRF — server-side request forgery
    "CWE-1333": "V5.3",  # ReDoS — catastrophic regex backtracking
}


def lookup(cwe: str) -> str:
    """Return ASVS chapter reference for a CWE, or empty string."""
    # Empty string (not None) keeps downstream f-strings clean.
    return CWE_TO_ASVS.get(cwe, "")


def all_asvs_requirements() -> list[dict]:
    """Return the full ASVS v5.0 chapter list (subset for reference)."""
    # Top-level chapter list — `id`/`name` shape consumed by report.py.
    # Subset is the 14 official chapters of ASVS v5.0 — keep in numeric order.
    return [
        # V1 — Application architecture & threat modelling.
        {"id": "V1", "name": "Architecture"},
        # V2 — Password storage, MFA, recovery.
        {"id": "V2", "name": "Authentication"},
        # V3 — Session ID generation, expiry, fixation.
        {"id": "V3", "name": "Session Management"},
        # V4 — Permission checks at the boundary of every action.
        {"id": "V4", "name": "Access Control"},
        # V5 — Input validation, output encoding, injection prevention.
        {"id": "V5", "name": "Validation, Sanitization, Encoding"},
        # V6 — Algorithms, key management, TLS, randomness.
        {"id": "V6", "name": "Cryptography"},
        # V7 — Log content, log destinations, PII redaction.
        {"id": "V7", "name": "Error Handling and Logging"},
        # V8 — PII handling, retention, deletion (GDPR-adjacent).
        {"id": "V8", "name": "Data Protection"},
        # V9 — TLS, certificate pinning, HTTP security headers.
        {"id": "V9", "name": "Communication"},
        # V10 — Code-signing, dependency provenance, build-pipeline integrity.
        {"id": "V10", "name": "Malicious Code"},
        # V11 — Workflow integrity, time-of-check / time-of-use.
        {"id": "V11", "name": "Business Logic"},
        # V12 — Upload validation, path traversal, executable content.
        {"id": "V12", "name": "Files and Resources"},
        # V13 — REST/GraphQL/SOAP API surface security.
        {"id": "V13", "name": "API and Web Service"},
        # V14 — Secrets management, hardening, deployment defaults.
        {"id": "V14", "name": "Configuration"},
    ]


if __name__ == "__main__":
    # CLI: `python3 owasp_asvs.py CWE-89` → "CWE-89 → ASVS V5.3"
    import sys

    if len(sys.argv) > 1:
        print(f"{sys.argv[1]} → ASVS {lookup(sys.argv[1])}")
    else:
        # No arg → dump the full mapping (one row per line).
        for cwe, asvs in CWE_TO_ASVS.items():
            print(f"{cwe} → ASVS {asvs}")
