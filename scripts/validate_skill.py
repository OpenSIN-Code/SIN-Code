#!/usr/bin/env python3
"""Validate a skill directory against the SIN-Code / OpenCode skill standard.

Docs: ../SKILL.md

Usage:
    python3 validate_skill.py <skill-dir> [--json] [--strict]
    python3 validate_skill.py --all-bundled [--json] [--strict]
    python3 validate_skill.py --all-sin [--json] [--strict]

Exit codes:
    0 = valid
    1 = invalid or error
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path
from typing import Any


# Required directories per the SIN-Code skill standard.
REQUIRED_DIRS = ("context", "frameworks", "tasks", "templates")

# Optional but recommended directories for SIN-Code ecosystem skills.
RECOMMENDED_DIRS = ("scripts", "tests", "lib")

# Frontmatter keys required by OpenCode / SIN-Code.
REQUIRED_FRONTMATTER = ("name", "description")

# Pattern for valid skill names (OpenCode spec).
NAME_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")


class Finding:
    def __init__(self, path: Path, level: str, message: str) -> None:
        self.path = path
        self.level = level
        self.message = message

    def to_dict(self) -> dict[str, Any]:
        return {
            "path": str(self.path),
            "level": self.level,
            "message": self.message,
        }


class SkillValidator:
    def __init__(self, root: Path, strict: bool = False) -> None:
        self.root = root.resolve()
        self.strict = strict
        self.findings: list[Finding] = []

    def add(self, level: str, message: str, path: Path | None = None) -> None:
        self.findings.append(Finding(path or self.root, level, message))

    def validate(self) -> bool:
        if not self.root.is_dir():
            self.add("error", f"Not a directory: {self.root}")
            return False

        self._check_skill_md()
        self._check_required_dirs()
        self._check_recommended_dirs()
        if self.strict:
            self._check_context_files()
            self._check_framework_files()
            self._check_task_files()
            self._check_template_files()
        return not any(f.level == "error" for f in self.findings)

    def _check_skill_md(self) -> None:
        skill_md = self.root / "SKILL.md"
        if not skill_md.is_file():
            self.add("error", "Missing SKILL.md (must be uppercase)")
            return

        text = skill_md.read_text(encoding="utf-8")
        if not text.startswith("---"):
            self.add("error", "SKILL.md must start with YAML frontmatter delimited by ---")
            return

        # Extract frontmatter block.
        end = text.find("---", 3)
        if end == -1:
            self.add("error", "SKILL.md frontmatter is not closed with ---")
            return

        try:
            import yaml
        except ImportError:
            # Graceful fallback: simple regex extraction for required keys.
            for key in REQUIRED_FRONTMATTER:
                if re.search(rf"^{key}:\s*(\S+)", text[:end], re.MULTILINE) is None:
                    self.add("error", f"Missing frontmatter key: {key}", skill_md)
            return

        try:
            front = yaml.safe_load(text[3:end]) or {}
        except Exception as exc:  # noqa: BLE001
            self.add("error", f"Invalid YAML frontmatter: {exc}", skill_md)
            return

        for key in REQUIRED_FRONTMATTER:
            if key not in front or not front[key]:
                self.add("error", f"Missing or empty frontmatter key: {key}", skill_md)

        name = front.get("name")
        if name and not NAME_RE.match(name):
            self.add("error", f"Invalid skill name {name!r} (must match {NAME_RE.pattern})", skill_md)
        if name and name != self.root.name:
            self.add("warning", f"Frontmatter name {name!r} does not match directory {self.root.name!r}", skill_md)

        desc = front.get("description", "")
        if desc and len(desc) > 1024:
            self.add("error", f"Description too long ({len(desc)} > 1024 chars)", skill_md)

        if "license" not in front:
            self.add("warning", "Missing frontmatter key: license", skill_md)

        compat = front.get("compatibility")
        if compat is not None and not isinstance(compat, list):
            self.add("error", "compatibility must be a YAML list", skill_md)

    def _check_required_dirs(self) -> None:
        for d in REQUIRED_DIRS:
            p = self.root / d
            if not p.is_dir():
                self.add("error", f"Missing required directory: {d}/")
            elif not any(p.iterdir()):
                self.add("warning", f"Required directory {d}/ is empty")

    def _check_recommended_dirs(self) -> None:
        for d in RECOMMENDED_DIRS:
            p = self.root / d
            if not p.is_dir():
                self.add("warning", f"Missing recommended directory: {d}/")

    def _check_dir_has_md(self, d: str, purpose: str) -> None:
        p = self.root / d
        if not p.is_dir():
            return
        md_files = list(p.rglob("*.md"))
        if not md_files:
            self.add("warning", f"{purpose} directory {d}/ contains no .md files")

    def _check_context_files(self) -> None:
        self._check_dir_has_md("context", "Context")

    def _check_framework_files(self) -> None:
        self._check_dir_has_md("frameworks", "Frameworks")

    def _check_task_files(self) -> None:
        self._check_dir_has_md("tasks", "Tasks")

    def _check_template_files(self) -> None:
        self._check_dir_has_md("templates", "Templates")

    def report(self) -> dict[str, Any]:
        return {
            "skill": self.root.name,
            "path": str(self.root),
            "valid": not any(f.level == "error" for f in self.findings),
            "findings": [f.to_dict() for f in self.findings],
        }


def validate_all_sin(json_out: bool, strict: bool) -> int:
    """Validate all SIN-Code skills under ~/.config/opencode/skills/sin-*."""
    skills_root = Path(os.getenv("SIN_SKILLS_DIR", Path.home() / ".config/opencode/skills"))
    skill_dirs = sorted(skills_root.glob("sin-*")) + sorted(skills_root.glob("skill-*"))
    return _validate_skill_dirs(skill_dirs, json_out, strict)


def validate_all_bundled(json_out: bool, strict: bool) -> int:
    """Validate all bundled skills under the repo's skills/ directory."""
    repo_root = Path(__file__).resolve().parent.parent
    skills_root = repo_root / "skills"
    skill_dirs = sorted(d for d in skills_root.iterdir() if d.is_dir())
    return _validate_skill_dirs(skill_dirs, json_out, strict)


def _validate_skill_dirs(skill_dirs: list[Path], json_out: bool, strict: bool) -> int:
    reports: list[dict[str, Any]] = []
    failed = 0
    for d in skill_dirs:
        v = SkillValidator(d, strict=strict)
        v.validate()
        reports.append(v.report())
        if not v.report()["valid"]:
            failed += 1

    if json_out:
        print(json.dumps({"skills": reports, "failed": failed}, indent=2))
    else:
        for r in reports:
            status = "✅" if r["valid"] else "❌"
            print(f"{status} {r['skill']}")
            for f in r["findings"]:
                print(f"   [{f['level']}] {f['message']}")
        print(f"\nTotal: {len(reports)} skills, {failed} failed.")
    return 1 if failed else 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Validate skill directory structure")
    parser.add_argument("path", nargs="?", help="Path to skill directory")
    parser.add_argument("--all-sin", action="store_true", help="Validate all SIN-Code skills in ~/.config/opencode/skills")
    parser.add_argument("--all-bundled", action="store_true", help="Validate all bundled skills in the repo's skills/ directory")
    parser.add_argument("--json", action="store_true", help="Emit JSON report")
    parser.add_argument("--strict", action="store_true", help="Enable extra checks")
    args = parser.parse_args(argv)

    if args.all_bundled:
        return validate_all_bundled(args.json, args.strict)
    if args.all_sin:
        return validate_all_sin(args.json, args.strict)

    if not args.path:
        parser.print_help()
        return 1

    root = Path(args.path)
    v = SkillValidator(root, strict=args.strict)
    v.validate()
    report = v.report()

    if args.json:
        print(json.dumps(report, indent=2))
    else:
        status = "VALID" if report["valid"] else "INVALID"
        print(f"{status}: {report['skill']}")
        for f in report["findings"]:
            print(f"  [{f['level']}] {f['message']}")

    return 0 if report["valid"] else 1


if __name__ == "__main__":
    sys.exit(main())
