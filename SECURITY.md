# Security Policy

## Supported Versions

| Version | Supported          | Python | Release date | EOL date |
|---------|--------------------|--------|--------------|----------|
| 0.4.x   | :white_check_mark: | 3.11+  | 2026-04+     | active   |
| 0.3.x   | :white_check_mark: | 3.11+  | 2026-02+     | 2026-12  |
| < 0.3   | :x:                | —      | —            | EOL      |

We follow **N+1 supported major versions** (current + previous).

## Reporting a Vulnerability

**Please do NOT report security vulnerabilities via public GitHub issues.**

Instead, use one of these private channels:

| Channel | Response time | For |
|---------|--------------|-----|
| **Email:** `security@opensin.dev` | < 24h ack, < 7d fix | All vulnerabilities |
| **GitHub Security Advisory:** [Report a vulnerability](https://github.com/OpenSIN-Code/SIN-Code-Bundle/security/advisories/new) | < 24h ack, < 7d fix | Coordinated disclosure |
| **Direct contact:** Jeremy (project lead) | < 48h ack | Sensitive disclosures only |

### What to include in your report

A good vulnerability report includes:

1. **Component affected** (e.g., `sin_code_bundle.memory`, `sin_code_bundle.vfs`)
2. **Affected versions** (which versions are vulnerable)
3. **Vulnerability class** (e.g., SQL injection, XSS, SSRF, ReDoS, info leak)
4. **Proof-of-concept** (minimal reproducer, ideally a test case)
5. **Impact assessment** (what an attacker could do)
6. **Environment** (Python version, OS, deployment context)
7. **Optional:** Suggested fix (we appreciate PRs but they are not required)

**Encryption:** For sensitive disclosures, request our PGP key by email.

## Our Response Process

```
Day 0:    Vulnerability reported (private channel)
Day 1:    Acknowledgement + triage
Day 2-7:  Investigation + fix development
Day 7-14: Patch release + CVE assignment (if applicable)
Day 14:   Public disclosure (coordinated with reporter)
```

We aim for **< 7 days from report to fix** for critical (RCE, auth bypass) and
**< 30 days** for high/medium severity issues.

## Supported Severity Levels (CVSS v3.1)

| Severity | CVSS range | Examples | SLA |
|----------|-----------|----------|-----|
| Critical | 9.0-10.0  | RCE, auth bypass, data loss | < 7 days |
| High     | 7.0-8.9   | Privilege escalation, SSRF, SQLi | < 14 days |
| Medium   | 4.0-6.9   | XSS, CSRF, info disclosure | < 30 days |
| Low      | 0.1-3.9   | Best-practice violations | < 90 days |

## Security Architecture (SIN-Code-Bundle)

SIN-Code-Bundle is a **meta-package** that orchestrates 7 Go tools + 11 Python
subsystems. Its security model is layered:

| Layer | Security boundary | Encryption at rest | Auth |
|-------|------------------|--------------------|------|
| **Code execution** | Process isolation (Go subprocess) | N/A | N/A |
| **API keys (v0.4.3+)** | Fernet AES-128-CBC | `SINATOR_ENCRYPTION_KEY` env var | `SINATOR_API_TOKEN` header |
| **Honcho peer memory** | Honcho server (separate process) | Honcho-managed | Honcho workspace ID |
| **SQLite memory** | Local file `.sin_memory.db` | Plaintext (local-only) | N/A (machine-scoped) |
| **CoDocs references** | Code-level only | N/A | N/A |

## Hardening Checklist (for operators)

```bash
# 1. Always set a strong API key for Honcho (32+ bytes)
export HONCHO_API_KEY="$(openssl rand -base64 32)"

# 2. Use Fernet encryption for any SIN-Code keys
python3 -c "from cryptography.fernet import Fernet; print(Fernet.generate_key().decode())"
# → Set as SINATOR_ENCRYPTION_KEY env var

# 3. Run ceo-audit regularly to detect regressions
~/.config/opencode/skills/ceo-audit/scripts/audit.sh . --profile=SECURITY --grade=B

# 4. Enable Dependabot/Renovate for automatic CVE patching
# (configured in .github/dependabot.yml if you forked us)

# 5. Pin Python versions in production
python3 -m pip install 'sin-code-bundle==0.4.4'  # exact pin
```

## Security Tools We Use Internally

| Tool | Purpose | Coverage |
|------|---------|----------|
| **bandit** | Python security linter | Every commit (CI) |
| **gosec** | Go security linter | Every commit (CI) |
| **pip-audit** | Python CVE scanner | Weekly (Dependabot) |
| **govulncheck** | Go CVE scanner | Weekly (Dependabot) |
| **gitleaks** | Secret scanner | Every commit (pre-commit hook) |
| **SOTA CoDocs** | Documentation-as-code | Every commit |
| **CEO Audit** | 47-gate SOTA review | Pre-release |

## Hall of Fame

We thank the following security researchers for responsible disclosure:

*(This section will be populated as vulnerabilities are reported and fixed.)*

## Contact

- **Project lead:** Jeremy
- **Email:** security@opensin.dev
- **PGP:** Available on request
- **Response SLA:** < 24h acknowledgement

---

*This policy is based on the [GitHub Security Lab template](https://github.com/securitylab) and adapted for SIN-Code-Bundle.*
