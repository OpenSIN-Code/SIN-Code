"""Purpose: ADW interceptor — pre-flight architectural rule enforcement.

Docs: interceptor.doc.md
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import Optional


class InterceptorRule:
    def __init__(self, name: str, pattern: str, message: str, severity: str = "error"):
        self.name = name
        self.pattern = re.compile(pattern, re.IGNORECASE)
        self.message = message
        self.severity = severity

    def matches(self, content: str) -> bool:
        return bool(self.pattern.search(content))


class SINInterceptor:
    """Intercepts tool calls and validates against architectural rules."""

    DEFAULT_RULES = [
        (
            "no_frontend_db_direct",
            r"(import|require).*(database|db|sql).*from.*(frontend|ui|component)",
            "Frontend components must not import database/SQL modules directly. Use an API layer.",
            "error",
        ),
        (
            "no_hardcoded_secrets",
            r"(password|secret|api_key|token)\s*=\s*['\"][^'\"]+['\"]",
            "Hardcoded secrets detected. Use environment variables or a secret manager.",
            "error",
        ),
        (
            "no_eval_exec",
            r"\b(eval|exec|subprocess\.shell=True)\b",
            "Dangerous execution pattern (eval/exec/shell=True) detected. Review for injection risks.",
            "warning",
        ),
    ]

    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = repo_root or Path.cwd()
        self.rules: list[InterceptorRule] = []
        self._load_default_rules()
        self._load_adw_rules()

    def _load_default_rules(self) -> None:
        for name, pattern, message, severity in self.DEFAULT_RULES:
            self.rules.append(InterceptorRule(name, pattern, message, severity))

    def _load_adw_rules(self) -> None:
        try:
            from sin_code_adw import ADW  # type: ignore

            adw = ADW(repo_root=self.repo_root)
            for rule in adw.get_active_rules():
                self.rules.append(
                    InterceptorRule(
                        name=rule.get("name", "adw_rule"),
                        pattern=rule.get("pattern", ".*"),
                        message=rule.get("message", "ADW violation"),
                        severity=rule.get("severity", "warning"),
                    )
                )
        except Exception:
            pass

    def add_rule(self, name: str, pattern: str, message: str, severity: str = "error") -> None:
        self.rules.append(InterceptorRule(name, pattern, message, severity))

    def preflight(self, tool_name: str, tool_input: dict) -> dict:
        content = self._extract_content(tool_name, tool_input)
        if not content:
            return {"allowed": True, "violations": []}
        violations = []
        for rule in self.rules:
            if rule.matches(content):
                violations.append(
                    {
                        "rule": rule.name,
                        "message": rule.message,
                        "severity": rule.severity,
                        "tool": tool_name,
                    }
                )
        if violations:
            has_error = any(v["severity"] == "error" for v in violations)
            return {
                "allowed": not has_error,
                "violations": violations,
                "system_reminder": self._format_reminder(violations) if has_error else None,
            }
        return {"allowed": True, "violations": []}

    @staticmethod
    def _extract_content(tool_name: str, tool_input: dict) -> Optional[str]:
        if tool_name in ("sin_write", "sin_edit", "sin_ast_edit"):
            return tool_input.get("content") or tool_input.get("new_content") or ""
        if tool_name == "sin_bash":
            return tool_input.get("command", "")
        return None

    @staticmethod
    def _format_reminder(violations: list) -> str:
        lines = ["⚠️ **ARCHITECTURAL VIOLATION DETECTED** ⚠️"]
        for v in violations:
            if v["severity"] == "error":
                lines.append(f"- 🚫 [{v['rule'].upper()}] {v['message']}")
        lines.append("\nPlease revise your approach before proceeding.")
        return "\n".join(lines)
