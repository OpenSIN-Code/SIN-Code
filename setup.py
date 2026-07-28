"""Setuptools build hooks for generated package resources.

The CEO Audit skill remains canonical under ``skills/code-skills``. During a
Python build, this hook copies that source tree into the wheel's
``sin_code_bundle/resources`` directory. No generated mirror is committed.
"""

from __future__ import annotations

import shutil
from pathlib import Path

from setuptools import setup
from setuptools.command.build_py import build_py as _build_py

ROOT = Path(__file__).resolve().parent
CEO_AUDIT_SOURCE = ROOT / "skills" / "code-skills" / "skill-code-ceo-audit"


def _ignore_skill_artifacts(_directory: str, names: list[str]) -> set[str]:
    ignored = {name for name in names if name in {"__pycache__", ".pytest_cache", "tests"}}
    ignored.update(name for name in names if name.endswith((".pyc", ".pyo")))
    return ignored


class build_py(_build_py):
    """Copy canonical non-Python resources into the wheel build tree."""

    def run(self) -> None:
        super().run()
        if not CEO_AUDIT_SOURCE.is_dir():
            raise RuntimeError(f"canonical CEO Audit skill missing: {CEO_AUDIT_SOURCE}")
        destination = Path(self.build_lib) / "sin_code_bundle" / "resources" / "ceo-audit"
        shutil.rmtree(destination, ignore_errors=True)
        shutil.copytree(
            CEO_AUDIT_SOURCE,
            destination,
            ignore=_ignore_skill_artifacts,
            copy_function=shutil.copy2,
        )

    def get_outputs(self, include_bytecode: bool = True) -> list[str]:
        outputs = list(super().get_outputs(include_bytecode=include_bytecode))
        destination = Path(self.build_lib) / "sin_code_bundle" / "resources" / "ceo-audit"
        if destination.exists():
            outputs.extend(str(path) for path in destination.rglob("*") if path.is_file())
        return outputs


setup(cmdclass={"build_py": build_py})
