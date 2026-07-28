#!/usr/bin/env python3
"""Production-focused static security scan for the CEO Audit security axis.

The scanner intentionally excludes tests, fixtures, documentation, vendored code,
and generated assets. Findings always include concrete source locations. It is a
conservative static signal, not a claim of exploitability.
"""

from __future__ import annotations

import json
import re
import subprocess
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable

MAX_FILE_BYTES = 2 * 1024 * 1024
MAX_EXAMPLES = 12
SOURCE_SUFFIXES = {
    ".py",
    ".go",
    ".js",
    ".jsx",
    ".ts",
    ".tsx",
    ".rs",
    ".java",
    ".kt",
    ".sh",
    ".bash",
    ".zsh",
}
EXCLUDED_PARTS = {
    ".git",
    ".venv",
    "venv",
    "env",
    "node_modules",
    "vendor",
    "dist",
    "build",
    "coverage",
    ".pytest_cache",
    "__pycache__",
    "tests",
    "test",
    "testdata",
    "fixtures",
    "fixture",
    "examples",
    "docs",
    "third_party",
    "ceo-audit-output",
}
PATTERN_DEFINITION_FILES = {
    "SIN-Code-SAST-Tool/pkg/rules/rules.go",
    "SIN-Code-Secrets-Scanner/pkg/rules/rules.go",
    "skills/code-skills/skill-code-ceo-audit/scripts/security_scan.py",
    "skills/code-skills/skill-code-ceo-audit/lib/sin_tools.py",
}

PLACEHOLDER_MARKERS = {
    "example",
    "dummy",
    "fake",
    "fixture",
    "placeholder",
    "redacted",
    "changeme",
    "replace_me",
    "your_api",
    "your-api",
    "test_secret",
    "test-secret",
}


@dataclass(frozen=True)
class Gate:
    gate_id: str
    severity: str
    cwe: str
    title: str
    fix: str


GATES = {
    "1.1": Gate(
        "1.1",
        "HIGH",
        "CWE-798",
        "Hardcoded credential-like value",
        "Move credentials to an environment-backed secret store.",
    ),
    "1.2": Gate(
        "1.2",
        "CRITICAL",
        "CWE-89",
        "Potential SQL injection",
        "Use parameterized queries and typed query builders.",
    ),
    "1.3": Gate(
        "1.3",
        "CRITICAL",
        "CWE-78",
        "Potential command injection",
        "Use argv lists with shell disabled and validate allowed executables.",
    ),
    "1.4": Gate(
        "1.4",
        "HIGH",
        "CWE-22",
        "Potential path traversal",
        "Resolve paths and enforce that the result remains inside an allowed root.",
    ),
    "1.5": Gate(
        "1.5",
        "HIGH",
        "CWE-918",
        "Potential server-side request forgery",
        "Allowlist schemes and hosts; block loopback, link-local, and private ranges.",
    ),
    "1.6": Gate(
        "1.6",
        "CRITICAL",
        "CWE-502",
        "Unsafe deserialization primitive",
        "Use safe loaders and authenticated, schema-validated payloads.",
    ),
    "1.7": Gate(
        "1.7",
        "HIGH",
        "CWE-327",
        "Weak cryptography in a security-sensitive context",
        "Use SHA-256+ for integrity and modern authenticated encryption.",
    ),
    "1.8": Gate(
        "1.8",
        "CRITICAL",
        "CWE-259",
        "Hardcoded password",
        "Load passwords from a secret store and rotate exposed values.",
    ),
    "1.9": Gate(
        "1.9",
        "MEDIUM",
        "CWE-338",
        "Non-cryptographic randomness in a security context",
        "Use secrets/crypto-rand for tokens, keys, nonces, and passwords.",
    ),
    "1.10": Gate(
        "1.10",
        "HIGH",
        "CWE-1333",
        "Potential catastrophic regular expression",
        "Bound input length or use a linear-time regex engine.",
    ),
    "1.11": Gate(
        "1.11",
        "INFO",
        "ASVS-V3.5",
        "Mutating endpoint requires authorization review",
        "Require authentication and authorization on every mutating route.",
    ),
    "1.12": Gate(
        "1.12",
        "MEDIUM",
        "CWE-601",
        "Potential open redirect",
        "Allowlist redirect targets and reject user-controlled absolute URLs.",
    ),
}

SKIPPED_GATES = {
    "1.4": "path traversal requires origin-to-sink data-flow analysis",
    "1.10": "ReDoS requires parsing regex literals and engine-specific complexity",
    "1.11": "authorization coverage requires framework route/dependency analysis",
}


def is_production_source(root: Path, path: Path) -> bool:
    try:
        rel = path.relative_to(root)
    except ValueError:
        return False
    parts = {part.lower() for part in rel.parts[:-1]}
    if parts & EXCLUDED_PARTS:
        return False
    name = path.name.lower()
    if path.suffix.lower() not in SOURCE_SUFFIXES:
        return False
    if (
        name.startswith("test_")
        or name.endswith("_test.py")
        or name.endswith("_test.go")
        or ".test." in name
        or ".spec." in name
        or name.endswith(".min.js")
        or name.endswith(".generated.go")
    ):
        return False
    return True


def is_pattern_definition(root: Path, path: Path) -> bool:
    try:
        return path.relative_to(root).as_posix() in PATTERN_DEFINITION_FILES
    except ValueError:
        return False


def iter_sources(root: Path) -> Iterable[tuple[Path, str]]:
    for path in root.rglob("*"):
        if not path.is_file() or not is_production_source(root, path):
            continue
        try:
            if path.stat().st_size > MAX_FILE_BYTES:
                continue
            yield path, path.read_text(encoding="utf-8", errors="replace")
        except OSError:
            continue


def location(root: Path, path: Path, text: str, start: int) -> str:
    line = text.count("\n", 0, start) + 1
    return f"{path.relative_to(root).as_posix()}:{line}"


def is_suppressed(text: str, start: int, marker: str | None) -> bool:
    if not marker:
        return False
    line_index = text.count("\n", 0, start)
    lines = text.splitlines()
    window = "\n".join(lines[max(0, line_index - 5) : line_index + 2]).lower()
    return f"ceo-audit: {marker}" in window


def regex_hits(
    root: Path,
    sources: list[tuple[Path, str]],
    pattern: str,
    flags: int = 0,
    suppression_marker: str | None = None,
) -> list[str]:
    compiled = re.compile(pattern, flags)
    hits: list[str] = []
    for path, text in sources:
        if is_pattern_definition(root, path):
            continue
        for match in compiled.finditer(text):
            if is_suppressed(text, match.start(), suppression_marker):
                continue
            hits.append(location(root, path, text, match.start()))
            if len(hits) >= MAX_EXAMPLES:
                return hits
    return hits


def secret_hits(root: Path, sources: list[tuple[Path, str]]) -> list[str]:
    pattern = re.compile(
        r"(?i)\b(api[_-]?key|secret(?:[_-]?key)?|password|access[_-]?token|auth[_-]?token)\b"
        r"\s*[:=]\s*[\"']([A-Za-z0-9_./+=-]{16,})[\"']"
    )
    hits: list[str] = []
    for path, text in sources:
        if is_pattern_definition(root, path):
            continue
        for match in pattern.finditer(text):
            value = match.group(2).strip().lower()
            if any(marker in value for marker in PLACEHOLDER_MARKERS):
                continue
            if value.startswith(("${", "{{", "<")) or set(value) <= {"x", "*", "-", "_"}:
                continue
            hits.append(location(root, path, text, match.start()))
            if len(hits) >= MAX_EXAMPLES:
                return hits

    # Gitleaks adds entropy/signature coverage. Only production paths survive.
    try:
        proc = subprocess.run(
            [
                "gitleaks",
                "detect",
                "--no-git",
                "-s",
                str(root),
                "-f",
                "json",
                "-r",
                "-",
                "--redact",
                "--no-banner",
                "--no-color",
            ],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
        raw = proc.stdout.strip() or "[]"
        findings = json.loads(raw)
        for finding in findings if isinstance(findings, list) else []:
            file_value = finding.get("File") or finding.get("file")
            if not file_value:
                continue
            candidate = Path(file_value)
            if not candidate.is_absolute():
                candidate = root / candidate
            if not is_production_source(root, candidate) or is_pattern_definition(root, candidate):
                continue
            line = finding.get("StartLine") or finding.get("startLine") or 1
            loc = f"{candidate.relative_to(root).as_posix()}:{line}"
            if loc not in hits:
                hits.append(loc)
            if len(hits) >= MAX_EXAMPLES:
                break
    except (
        FileNotFoundError,
        subprocess.TimeoutExpired,
        json.JSONDecodeError,
        OSError,
        ValueError,
    ):
        pass
    return hits


def ssrf_hits(root: Path, sources: list[tuple[Path, str]]) -> list[str]:
    """Return high-confidence untrusted outbound-network sinks.

    Configured provider endpoints and installer URLs are operator trust
    boundaries, not SSRF by themselves. This gate therefore only flags URL
    variables whose names explicitly signal request/user/agent input, plus
    browser-navigation APIs. Data-flow-complete coverage remains the job of a
    future AST/SSA gate rather than a broad textual guess.
    """
    untrusted_name = (
        r"(?:user[_-]?url|target[_-]?url|request[_-]?url|input[_-]?url|"
        r"agent[_-]?url|remote[_-]?url|destination[_-]?url)"
    )
    sink_patterns = (
        re.compile(
            rf"\b(?:requests|httpx)\.(?:get|post|put|delete|request)\s*\(\s*{untrusted_name}\b",
            re.IGNORECASE,
        ),
        re.compile(
            rf"(?<![\w.])fetch\s*\(\s*{untrusted_name}\b",
            re.IGNORECASE,
        ),
        re.compile(
            rf"\baxios\.(?:get|post|put|delete)\s*\(\s*{untrusted_name}\b",
            re.IGNORECASE,
        ),
        re.compile(
            rf"\bhttp\.(?:Get|Post)\s*\(\s*{untrusted_name}\b",
            re.IGNORECASE,
        ),
        re.compile(r"\bchromedp\.Navigate\s*\(\s*[a-zA-Z_]\w*\s*\)"),
    )
    mitigation = re.compile(
        r"(?:egress\.(?:Check|ValidateURL)|validate(?:Outbound)?URL|checkEgress)",
        re.IGNORECASE,
    )
    hits: list[str] = []
    for path, text in sources:
        if is_pattern_definition(root, path):
            continue
        lines = text.splitlines()
        for line_no, line in enumerate(lines, start=1):
            if line.lstrip().startswith(("//", "#", "/*", "*")):
                continue
            if not any(pattern.search(line) for pattern in sink_patterns):
                continue
            start_line = max(0, line_no - 81)
            context = "\n".join(lines[start_line:line_no])
            if mitigation.search(context):
                continue
            hits.append(f"{path.relative_to(root).as_posix()}:{line_no}")
            if len(hits) >= MAX_EXAMPLES:
                return hits
    return hits


def scan(root: Path) -> dict:
    sources = list(iter_sources(root))
    findings: list[dict] = []
    gate_results: list[dict] = []

    patterns: dict[str, tuple[str, int]] = {
        "1.2": (r"(?i)\b(?:execute|query|raw)\s*\([^\n)]*(?:\+|f[\"']|\.format\()", 0),
        "1.3": (
            r"(?i)(?:subprocess\.(?:run|call|Popen|check_output)\s*\([^\n)]*shell\s*=\s*True"
            r"|\bos\.(?:system|popen)\s*\(|child_process\.exec\s*\("
            r"|exec\.Command\s*\(\s*[\"'](?:sh|bash)[\"']\s*,\s*[\"']-c[\"'])",
            0,
        ),
        "1.4": (
            r"(?i)(?:\bopen|Path|os\.path\.join|filepath\.Join)\s*\([^\n)]*"
            r"(?:request|req\.|params|input|user[_-]?path|filename)",
            0,
        ),
        "1.5": (
            r"(?i)(?:requests\.(?:get|post|put|delete)|httpx\.(?:get|post|put|delete)"
            r"|urllib\.request\.urlopen|fetch|http\.Get)\s*\([^\n)]*"
            r"(?:request|req\.|params|input|user[_-]?url|target[_-]?url)",
            0,
        ),
        "1.6": (r"\b(?:pickle\.(?:load|loads)|marshal\.loads|yaml\.load)\s*\(", 0),
        "1.9": (
            r"(?i)(?:random\.(?:random|randint|choice|randrange)|Math\.random)\s*\([^\n]*"
            r"(?:token|secret|password|nonce|key)",
            0,
        ),
        "1.10": (
            r"(?:re\.compile|regexp\.MustCompile|new RegExp)\s*\([^\n]*(?:\([^\n)]*[+*][^\n)]*\))[+*]",
            0,
        ),
        "1.11": (r"(?m)^\s*@(?:app|router)\.(?:post|put|delete|patch)\s*\(", 0),
        "1.12": (
            r"(?i)(?:redirect|HttpResponseRedirect)\s*\([^\n)]*(?:request|req\.|params|input)",
            0,
        ),
    }

    for skipped_gate in SKIPPED_GATES:
        patterns.pop(skipped_gate, None)
    patterns.pop("1.5", None)

    hits_by_gate: dict[str, list[str]] = {
        "1.1": secret_hits(root, sources),
        "1.5": ssrf_hits(root, sources),
    }
    for gate_id, (pattern, flags) in patterns.items():
        marker = "allow-shell" if gate_id == "1.3" else None
        hits_by_gate[gate_id] = regex_hits(
            root,
            sources,
            pattern,
            flags,
            suppression_marker=marker,
        )

    # Weak hashes are only security findings when nearby code signals an
    # authentication/integrity use. Content-addressing fingerprints are valid.
    weak_context = re.compile(
        r"(?is)(?:password|secret|auth|signature|credential|token|integrity).{0,160}"
        r"(?:hashlib\.(?:md5|sha1)\s*\(|crypto/(?:md5|sha1)|DES\.new|MODE_ECB)"
        r"|(?:hashlib\.(?:md5|sha1)\s*\(|crypto/(?:md5|sha1)|DES\.new|MODE_ECB).{0,160}"
        r"(?:password|secret|auth|signature|credential|token|integrity)"
    )
    hits_by_gate["1.7"] = []
    for path, text in sources:
        if is_pattern_definition(root, path):
            continue
        for match in weak_context.finditer(text):
            hits_by_gate["1.7"].append(location(root, path, text, match.start()))
            if len(hits_by_gate["1.7"]) >= MAX_EXAMPLES:
                break
        if len(hits_by_gate["1.7"]) >= MAX_EXAMPLES:
            break

    # Gate 1.8 is the password-specific subset of 1.1.
    hits_by_gate["1.8"] = regex_hits(
        root,
        sources,
        r"(?i)\bpassword\b\s*[:=]\s*[\"'](?![^\"']*(?:example|dummy|fake|placeholder|changeme))[^\"'\n]{12,}[\"']",
    )

    for gate_id, gate in GATES.items():
        if gate_id in SKIPPED_GATES:
            gate_results.append(
                {
                    "id": gate_id,
                    "severity": gate.severity,
                    "status": "skipped",
                    "reason": SKIPPED_GATES[gate_id],
                }
            )
            continue
        hits = sorted(set(hits_by_gate.get(gate_id, [])))
        status = "finding" if hits else "pass"
        gate_results.append({"id": gate_id, "severity": gate.severity, "status": status})
        if not hits:
            continue
        examples = ", ".join(hits[:MAX_EXAMPLES])
        findings.append(
            {
                "gate": gate_id,
                "severity": gate.severity,
                "cwe": gate.cwe,
                "title": gate.title,
                "description": f"{len(hits)} production-source match(es): {examples}",
                "fix": gate.fix,
                "locations": hits,
                "occurrence_count": len(hits),
            }
        )

    return {
        "axis": "security",
        "gates": gate_results,
        "findings": findings,
        "scanned_files": len(sources),
        "scope": "production source only; tests/docs/fixtures/vendor/generated excluded",
    }


def main() -> int:
    if len(sys.argv) != 3:
        print("Usage: security_scan.py <repo> <output.json>", file=sys.stderr)
        return 2
    root = Path(sys.argv[1]).resolve()
    output = Path(sys.argv[2])
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_text(json.dumps(scan(root), indent=2))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
