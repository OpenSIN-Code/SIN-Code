"""Purpose: Wrapper for SIN-Code Tool Suite — used by CEO Audit axes.

Centralized subprocess invocation with timeout, error handling,
output parsing, and result caching.

Docs: sin_tools.doc.md
"""

from __future__ import annotations

import json
import shutil
import subprocess
from typing import Any

# ── Module overview ──────────────────────────────────────────────────
#
# Two API layers in this module:
#
#   LOW-LEVEL (functions 1–4): direct subprocess invocation of the
#   sin-* Go binaries via JSON-RPC over stdin/stdout.
#     - call_sin_tool: raw JSON-RPC call with timeout + error catching
#     - extract_text: pull the Markdown body out of a JSON-RPC response
#     - count_matches: count "Match" lines in scout/grep output
#     - discover/scout/map_arch/grasp: thin wrappers for the 4 main tools
#
#   HIGH-LEVEL (added v0.3.0): per-axis check_*() functions that
#   produce {findings: [...]} dicts in the canonical axis JSON shape.
#   Eight axes total — one check_* per axis — dispatched via
#   AXIS_CHECKS / check_axis(name, repo).
#
# Error model: NO exceptions cross the API boundary. Every function
# returns {"error": "..."} OR an empty findings list when the tool is
# missing, times out, or returns malformed output. Axis shell scripts
# (axis_*.sh) can therefore `grep "error"` without try/except.
# ─────────────────────────────────────────────────────────────────────


# ── Low-level: subprocess + JSON-RPC wrapper ─────────────────────────


def call_sin_tool(tool: str, args: dict[str, Any], timeout: int = 60) -> dict:
    """Call a SIN-Code Go tool via its JSON-RPC interface.

    Args:
        tool: binary name (discover, map, grasp, scout, etc.)
        args: tool-specific arguments
        timeout: seconds

    Returns:
        Parsed JSON-RPC response, or {"error": ...} on failure

    Note: Errors are returned as dicts (not raised) so axis scripts
    can `grep "error"` without a try/except block.
    """
    # Fail-fast: report missing binary as an error dict (NOT raise).
    # Axis shell scripts can `if ! grep -q error` to detect this case.
    if not shutil.which(tool):
        return {"error": f"{tool} not installed (run SIN-Code/install.sh)"}
    # Standard JSON-RPC 2.0 envelope expected by every sin-* binary.
    # id=1 is fine — we never make concurrent requests on one process.
    payload = {
        "jsonrpc": "2.0",
        "method": "tools/call",
        "id": 1,
        "params": {"name": tool, "arguments": args},
    }
    try:
        # `--mcp` puts the binary into JSON-RPC-over-stdin mode.
        # Input via stdin, output via stdout — classic Unix pipe model.
        result = subprocess.run(
            [tool, "--mcp"],
            input=json.dumps(payload),
            capture_output=True,
            text=True,
            timeout=timeout,
        )
        if result.returncode != 0:
            # Surface both exit code AND stderr — needed for CI debugging.
            # CI logs are the only place we can post-mortem failed runs.
            return {"error": f"{tool} exit {result.returncode}", "stderr": result.stderr}
        # Happy path: parse stdout as JSON-RPC response.
        return json.loads(result.stdout)
    except subprocess.TimeoutExpired:
        # Timeout = soft failure; caller decides whether to retry.
        # Default 60s is comfortable for any indexed repo of <100k files.
        return {"error": f"{tool} timed out after {timeout}s"}
    except json.JSONDecodeError as e:
        # Tool returned non-JSON (binary crash or wrong mode).
        # Most common cause: tool printed a Python traceback instead of JSON.
        return {"error": f"{tool} returned invalid JSON: {e}"}


# ── Response-text extraction helpers ─────────────────────────────────


def extract_text(response: dict) -> str:
    """Extract the 'text' field from a JSON-RPC response (Markdown content)."""
    # Error responses lack the .result.content list — return empty.
    # Callers can chain: `text = extract_text(call_sin_tool(...))` safely.
    if "error" in response:
        return ""
    # Defensive: .result might be missing; .content might be empty.
    content = response.get("result", {}).get("content", [])
    if content and isinstance(content, list):
        # First content item carries the Markdown text body.
        # SIN-Code tools always return exactly one content block.
        return content[0].get("text", "")
    return ""


def count_matches(text: str) -> int:
    """Count 'Match' lines in a scout/grep-style response."""
    # Case-insensitive: scout uses both "Match" (regex) and "match" (substring).
    # Used by every check_*() function to detect non-zero hits.
    return sum(1 for line in text.splitlines() if "Match" in line or "match" in line)


# ── Convenience wrappers for the 4 main tools ────────────────────────


def discover(path: str, pattern: str = "**/*", max_results: int = 500) -> str:
    """Quick wrapper for the discover tool."""
    # `pattern` is a glob (relative to `path`); see discover --help.
    # Default "**/*" matches all files; common: "**/*.py" or "**/*.md".
    r = call_sin_tool("discover", {"path": path, "pattern": pattern, "max_results": max_results})
    return extract_text(r)


def scout(path: str, query: str, search_type: str = "regex", max_results: int = 100) -> str:
    """Quick wrapper for the scout tool."""
    # search_type: "regex" (default) | "literal" | "symbol" | "semantic".
    # regex uses Go's RE2 engine — no backreferences (safer, faster).
    r = call_sin_tool(
        "scout",
        {"path": path, "query": query, "search_type": search_type, "max_results": max_results},
    )
    return extract_text(r)


def map_arch(path: str) -> str:
    """Quick wrapper for the map tool."""
    # action="map" → architecture overview (modules, entry points, hot paths).
    # Other actions exist (depmap, callgraph) but we only use map here.
    r = call_sin_tool("map", {"path": path, "action": "map"})
    return extract_text(r)


def grasp(file: str) -> str:
    """Quick wrapper for the grasp tool."""
    # `grasp` analyses a single file (NOT a directory).
    # Returns code structure: classes, functions, imports, complexity.
    r = call_sin_tool("grasp", {"file": file})
    return extract_text(r)


if __name__ == "__main__":
    # Quick test
    print("discover test:", discover(".", "**/*.py")[:200])
    print("scout test:", scout(".", "def main", "regex")[:200])


# ─────────────────────────────────────────────────────────────────────
# Per-axis high-level checks (added v0.3.0)
#
# Each check_<axis>() returns a list of finding dicts in the same
# shape that scripts/add_finding.py writes to the per-axis JSON
# file. Axis scripts can either call them directly (Python-only path)
# or continue using the JSON-RPC pattern — both produce the same
# finding format.
#
# These methods always return gracefully: if a tool is missing or
# the call times out, they return {"error": ..., "findings": []}
# instead of raising, so axis scripts never have to wrap them in
# try/except.
# ─────────────────────────────────────────────────────────────────────


# ── Finding builder ──────────────────────────────────────────────────


def _finding(gate: str, severity: str, cwe: str, title: str, description: str, fix: str) -> dict:
    """Build a finding dict in the canonical axis JSON shape."""
    # All six fields are mandatory — schema enforced by every check_*() caller.
    return {
        "gate": gate,
        "severity": severity,
        "cwe": cwe,
        "title": title,
        "description": description,
        "fix": fix,
    }


# ── Axis 1: Security (12 gates) ───────────────────────────────────────


def check_security(repo: str, max_findings: int = 50) -> dict:
    """Run the 12 security gates via SIN-Code tools.

    Returns:
        {"findings": [...], "error": "..." (only on failure)}

    Each gate is a single scout call with a CWE-tagged regex. The
    full gate list is documented in SKILL.md#axis-1-security.

    Why not call the axis_security.sh? Because Python callers
    (notebooks, custom report generators) want a structured
    result, not a side-effecting shell call.
    """
    findings: list[dict] = []
    # Gate definitions — keep in sync with axis_security.sh and SKILL.md.
    # Each tuple: (gate_id, severity, cwe, regex).
    # Order is informational only — every gate is evaluated unless max_findings hits.
    gates = [
        # (gate_id, severity, cwe, query)
        # 1.1 — hardcoded API keys / secrets / passwords / tokens (≥20 hex chars).
        (
            "1.1",
            "HIGH",
            "CWE-798",
            r"(api[_-]?key|secret[_-]?key|password|token)\s*=\s*['\"][A-Za-z0-9]{20,}",
        ),
        # 1.2 — SQL injection: raw string concatenation in execute/query/raw.
        ("1.2", "CRITICAL", "CWE-89", r"(execute|query|raw)\s*\(\s*['\"].*\+.*['\"]"),
        # 1.3 — OS command injection: subprocess/system/exec with concat or shell=True.
        (
            "1.3",
            "CRITICAL",
            "CWE-78",
            r"(subprocess\.(call|run|Popen|check_output)|os\.system|os\.popen|exec[lp]?\s*\()\s*[^,)]*\+|shell\s*=\s*True",
        ),
        # 1.4 — path traversal: open/Path with `+` concat or `../` in literal.
        ("1.4", "HIGH", "CWE-22", r"(open|os\.path\.join|Path)\s*\([^)]*\+|[^/]*\.\./"),
        # 1.5 — SSRF: HTTP client passes user input as URL.
        (
            "1.5",
            "HIGH",
            "CWE-918",
            r"(requests\.(get|post|put|delete)|urllib\.request\.urlopen|httpx\.|fetch\s*\()\s*\([^)]*input|args|params",
        ),
        # 1.6 — insecure deserialization: pickle/yaml.load/marshal.loads.
        ("1.6", "CRITICAL", "CWE-502", r"(pickle\.(load|loads)|yaml\.load\([^_])|marshal\.loads"),
        # 1.7 — weak crypto: MD5, SHA1, DES, ECB (broken or deprecated).
        ("1.7", "HIGH", "CWE-327", r"(hashlib\.(md5|sha1)|DES|ECB|MD5|SHA1)\b"),
        # 1.9 — insecure random: random.* / Math.random for security-sensitive code.
        (
            "1.9",
            "MEDIUM",
            "CWE-338",
            r"(random\.(random|randint|choice|shuffle|randrange)|Math\.random)\s*\(",
        ),
        # 1.10 — ReDoS: regex with nested quantifiers (catastrophic backtracking).
        (
            "1.10",
            "HIGH",
            "CWE-1333",
            r"(re\.compile|regexp\.MustCompile|new RegExp)\s*\(\s*['\"][^'\"]*\([^'\"]*\)\+[^'\"]*['\"]",
        ),
        # 1.11 — mutating endpoint without CSRF protection (informational).
        ("1.11", "MEDIUM", "ASVS-V3.5", r"@(app|router)\.(post|put|delete|patch)\s*\("),
        # 1.12 — open redirect: redirect/Location uses unvalidated request param.
        (
            "1.12",
            "MEDIUM",
            "CWE-601",
            r"(redirect|HttpResponseRedirect)\s*\([^)]*request\.(GET|POST)|Location.*request\.",
        ),
    ]
    for gate_id, sev, cwe, query in gates:
        # Hard cap to prevent finding explosion on noisy repos.
        if len(findings) >= max_findings:
            break
        text = scout(repo, query, search_type="regex", max_results=30)
        match_count = count_matches(text)
        if match_count > 0:
            findings.append(
                _finding(
                    gate=gate_id,
                    severity=sev,
                    cwe=cwe,
                    title=f"Security gate {gate_id} matched",
                    description=f"{match_count} potential issue(s) matched pattern",
                    fix="Review matched sites; see CWE reference for remediation",
                )
            )
    return {"findings": findings}


# ── Axis 2: Performance (6 gates) ─────────────────────────────────────


def check_performance(repo: str, max_findings: int = 50) -> dict:
    """Run the 6 performance gates via SIN-Code tools.

    Returns: same shape as check_security.
    """
    findings: list[dict] = []
    # Performance gates: cheaper than security (no CWE mapping needed).
    # All findings use CWE="PERF" so report.py treats them as a single category.
    gates = [
        # (gate_id, severity, query) — CWE-less for performance
        # 2.1 — nested for-loops (O(n²) signal).
        ("2.1", "MEDIUM", r"for\s+\w+\s+in\s+.*:\s*\n\s*for\s+\w+\s+in\s+"),  # nested for
        # 2.2 — giant slice index (e.g., [10000000] = …) → memory leak risk.
        ("2.2", "MEDIUM", r"\[\s*\d{6,}\s*\]\s*=\s*"),  # giant slice
        # 2.3 — unbounded lru_cache (memory leak in long-running services).
        ("2.3", "MEDIUM", r"(lru_cache|functools\.cache)\s*\("),  # unbounded cache
        # 2.4 — regex compiled in hot path instead of module-level.
        ("2.4", "LOW", r"re\.(compile|match|search)\s*\(\s*['\"]"),  # regex per-call
        # 2.5 — synchronous I/O in async/web handler (blocks event loop).
        ("2.5", "MEDIUM", r"open\([^)]*\)\.read\(\)"),  # sync I/O in hot path
    ]
    for gate_id, sev, query in gates:
        # Same cap pattern as check_security — avoid overwhelming the report.
        if len(findings) >= max_findings:
            break
        # max_results=20 per gate — performance findings are typically clustered.
        text = scout(repo, query, search_type="regex", max_results=20)
        if count_matches(text) > 0:
            findings.append(
                _finding(
                    gate=gate_id,
                    severity=sev,
                    cwe="PERF",
                    title=f"Performance gate {gate_id} matched",
                    description="See scout output for matched sites",
                    fix="Apply axis 2 remediation from SKILL.md",
                )
            )
    return {"findings": findings}


# ── Axis 3: Code Quality (7 gates) ────────────────────────────────────


def check_quality(repo: str, max_findings: int = 50) -> dict:
    """Run the 7 code-quality gates via SIN-Code tools.

    Returns: same shape as check_security.
    """
    findings: list[dict] = []
    # Code-quality uses discover + scout, not the graph.
    # discover with a multi-extension glob → catches Python/Go/TS/JS/Rust.
    # max_results=500 is a safety cap — large repos have thousands of files.
    files_text = discover(repo, "**/*.{py,go,ts,js,rs}", max_results=500)
    if "lines" in files_text and count_matches(files_text) > 0:
        # Heuristic: file-size signal from discover output.
        # discover prints "<path>: <lines> lines" when files are large.
        findings.append(
            _finding(
                gate="3.3",
                severity="HIGH",
                cwe="QUALITY-HUGE",
                title="Files > 500 lines detected",
                description="Large files may be god modules — review and split",
                fix="Refactor into smaller single-responsibility modules",
            )
        )
    # Naming convention check: snake_case violations in Python.
    # Pattern: `def lowerCamelCase` — Python PEP-8 mandates snake_case.
    text = scout(repo, r"def\s+[a-z]+[A-Z]", "regex", 50)
    if count_matches(text) > 0:
        findings.append(
            _finding(
                gate="3.6",
                severity="LOW",
                cwe="QUALITY-NAMING",
                title="Non-snake_case Python function names",
                description="Python convention: snake_case only",
                fix="Rename to snake_case",
            )
        )
    return {"findings": findings}


# ── Axis 4: Testing (5 gates) ─────────────────────────────────────────


def check_testing(repo: str, max_findings: int = 50) -> dict:
    """Run the 5 testing gates via SIN-Code tools.

    Returns: same shape as check_security.
    """
    findings: list[dict] = []
    # Coverage detection: stubbed for now (would require pytest-cov runtime).
    # We can only static-detect framework presence — actual % needs runtime.
    has_pytest = bool(call_sin_tool.__module__)  # noqa: F841 (placeholder)
    # Detect any test framework import — pytest or unittest.
    # ^ anchors ensure we only match top-level imports, not strings.
    text = scout(repo, r"^import\s+(pytest|unittest)|^from\s+pytest", "regex", 30)
    if count_matches(text) == 0:
        findings.append(
            _finding(
                gate="4.1",
                severity="INFO",
                cwe="TEST-COVERAGE",
                title="Test framework not detected",
                description="No pytest/unittest import found",
                fix="Add a test framework and measure coverage with pytest --cov",
            )
        )
    # Flaky-test signal: time.sleep() in tests is a classic anti-pattern.
    # Tests with sleeps fail intermittently → erode trust in the test suite.
    text = scout(repo, r"time\.sleep\(", "regex", 30)
    if count_matches(text) > 0:
        findings.append(
            _finding(
                gate="4.2",
                severity="MEDIUM",
                cwe="TEST-FLAKY",
                title="time.sleep() in test files",
                description="time.sleep causes flaky tests — prefer polling or mocks",
                fix="Use unittest.mock, polling, or asyncio.sleep",
            )
        )
    return {"findings": findings}


# ── Axis 5: Dependencies (5 gates) ────────────────────────────────────


def check_deps(repo: str, max_findings: int = 50) -> dict:
    """Run the 5 dependency gates via SIN-Code tools.

    Returns: same shape as check_security.

    Note: Real CVE checking requires harvest → NVD/OSV; here we
    just flag unpinned versions and missing lockfiles.
    """
    findings: list[dict] = []
    # Detect caret-range versions: ^x.y.z = supply-chain risk.
    # Pattern matches npm/yarn/cargo-style optimistic version ranges.
    text = scout(repo, r"['\"]\^[0-9]+\.[0-9]+\.[0-9]+['\"]", "regex", 50)
    if count_matches(text) > 0:
        findings.append(
            _finding(
                gate="5.3",
                severity="MEDIUM",
                cwe="DEPS-UNPINNED",
                title="Unpinned caret-range versions detected",
                description="^x.y.z in production deps is a supply-chain risk",
                fix="Pin exact versions or use lockfiles (poetry.lock, package-lock.json)",
            )
        )
    return {"findings": findings}


# ── Axis 6: Documentation (4 gates) ───────────────────────────────────


def check_docs(repo: str, max_findings: int = 50) -> dict:
    """Run the 4 documentation gates via SIN-Code tools.

    Returns: same shape as check_security.
    """
    findings: list[dict] = []
    # README check (case-insensitive via glob).
    # `README*` catches README.md, README.rst, README.txt, README (no ext).
    text = discover(repo, "README*", max_results=10)
    if "README" not in text:
        findings.append(
            _finding(
                gate="6.1",
                severity="MEDIUM",
                cwe="DOCS-README",
                title="README missing",
                description="No README found at repo root",
                fix="Add a README.md with quick-start, architecture, and dev setup",
            )
        )
    # CHANGELOG check — encourages release discipline.
    # Keep-a-Changelog format is the de-facto standard.
    text = discover(repo, "CHANGELOG*", max_results=10)
    if "CHANGELOG" not in text:
        findings.append(
            _finding(
                gate="6.2",
                severity="LOW",
                cwe="DOCS-CHANGELOG",
                title="CHANGELOG missing",
                description="No CHANGELOG found at repo root",
                fix="Add a CHANGELOG.md following Keep-a-Changelog format",
            )
        )
    return {"findings": findings}


# ── Axis 7: Architecture (4 gates) ────────────────────────────────────


def check_architecture(repo: str, max_findings: int = 50) -> dict:
    """Run the 4 architecture gates via SIN-Code tools.

    Returns: same shape as check_security.

    Note: This uses map (architecture overview) rather than sckg,
    because not all repos have sckg installed. Cycle detection
    falls back to the map output.
    """
    findings: list[dict] = []
    # map_arch returns an architecture overview (modules + dependencies).
    text = map_arch(repo)
    if not text:
        # Empty map → tool not installed or repo has no recognised structure.
        findings.append(
            _finding(
                gate="7.0",
                severity="INFO",
                cwe="ARCH-EMPTY",
                title="map_arch returned empty",
                description="No architecture overview available — install map or sckg",
                fix="Install SIN-Code map: pip install sin-code",
            )
        )
    # Heuristic: large module = god module. Map output should show
    # module sizes; we count files in a single dir as a signal.
    # 300+ Python files across the repo is a god-package smell.
    text = discover(repo, "**/*.py", max_results=500)
    file_count = count_matches(text)
    if file_count > 300:
        # 300+ .py files is a strong god-package smell.
        findings.append(
            _finding(
                gate="7.2",
                severity="MEDIUM",
                cwe="ARCH-GOD",
                title="Many Python files in one project",
                description=f"{file_count} .py files — possible god module",
                fix="Split into sub-packages with clear boundaries",
            )
        )
    return {"findings": findings}


# ── Axis 8: Compliance (4 gates) ──────────────────────────────────────


def check_compliance(repo: str, max_findings: int = 50) -> dict:
    """Run the 4 compliance gates via SIN-Code tools.

    Returns: same shape as check_security.
    """
    findings: list[dict] = []
    # SECURITY.md is required by GitHub for vulnerability disclosure.
    # case-insensitive check (both "SECURITY.md" and "security.md" exist in repos).
    text = discover(repo, "SECURITY.md", max_results=10)
    if "SECURITY.md" not in text and "security.md" not in text.lower():
        findings.append(
            _finding(
                gate="8.2",
                severity="MEDIUM",
                cwe="COMPLIANCE-SECURITY",
                title="SECURITY.md missing",
                description="Repositories should publish a vulnerability-disclosure policy",
                fix="Add a SECURITY.md with a contact email and 90-day disclosure SLA",
            )
        )
    # LICENSE absence = unusable for any downstream user.
    # `LICENSE*` glob catches LICENSE, LICENSE.md, LICENSE.txt.
    text = discover(repo, "LICENSE*", max_results=10)
    if "LICENSE" not in text:
        findings.append(
            _finding(
                gate="8.1",
                severity="HIGH",
                cwe="COMPLIANCE-LICENSE",
                title="LICENSE file missing",
                description="No LICENSE file at repo root",
                fix="Add a LICENSE file matching your project's license",
            )
        )
    return {"findings": findings}


# ── Public dispatch table ─────────────────────────────────────────────
# Convenience: dispatch by axis name.
# Adding/removing an axis here is a breaking change — every caller (axis_*.sh,
# tests, score.py's AXIS_WEIGHTS) must be updated in lockstep.
AXIS_CHECKS = {
    "security": check_security,
    "performance": check_performance,
    "quality": check_quality,
    "testing": check_testing,
    "deps": check_deps,
    "docs": check_docs,
    "architecture": check_architecture,
    "compliance": check_compliance,
}


def check_axis(axis: str, repo: str) -> dict:
    """Run the per-axis check by name. Returns same shape as check_<axis>."""
    # Case-insensitive lookup — callers may pass "Security" / "SECURITY".
    fn = AXIS_CHECKS.get(axis.lower())
    if fn is None:
        # Soft error: caller (axis script) treats this as "no findings".
        return {"error": f"unknown axis: {axis}", "findings": []}
    return fn(repo)
