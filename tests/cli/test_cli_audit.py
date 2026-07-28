"""CEO Audit CLI source-resolution and installation tests."""

from __future__ import annotations

import os
from pathlib import Path
from types import SimpleNamespace

import pytest
import typer

from sin_code_bundle import cli_audit


def make_skill(root: Path) -> Path:
    skill = root / "skill"
    scripts = skill / "scripts"
    scripts.mkdir(parents=True)
    (scripts / "audit.sh").write_text("#!/usr/bin/env bash\nexit 0\n")
    compat = scripts / "compat-bin"
    compat.mkdir()
    (compat / "scout").write_text("#!/usr/bin/env python3\n")
    return skill


def test_source_checkout_resolves_canonical_skill() -> None:
    source = cli_audit._source_checkout_skill()
    assert source.as_posix().endswith("skills/code-skills/skill-code-ceo-audit")
    assert (source / "scripts" / "audit.sh").is_file()


def test_explicit_skill_override_has_highest_priority(tmp_path: Path, monkeypatch) -> None:
    skill = make_skill(tmp_path)
    monkeypatch.setenv("SIN_CEO_AUDIT_SKILL_PATH", str(skill))
    assert cli_audit._active_skill() == skill
    assert cli_audit._user_skill_path() == skill


def test_bundled_skill_precedes_stale_user_install(tmp_path: Path, monkeypatch) -> None:
    bundled = make_skill(tmp_path / "bundled")
    installed = make_skill(tmp_path / "installed")
    missing_source = tmp_path / "missing-source"

    monkeypatch.delenv("SIN_CEO_AUDIT_SKILL_PATH", raising=False)
    monkeypatch.setattr(cli_audit, "_source_checkout_skill", lambda: missing_source)
    monkeypatch.setattr(cli_audit, "_bundled_skill", lambda: bundled)
    monkeypatch.setattr(cli_audit, "_installed_user_skill", lambda: installed)

    assert cli_audit._active_skill() == bundled


def test_install_copies_distribution_source_and_marks_launchers_executable(
    tmp_path: Path, monkeypatch
) -> None:
    source = make_skill(tmp_path / "source")
    target = tmp_path / "installed"
    monkeypatch.setenv("SIN_CEO_AUDIT_SKILL_PATH", str(target))
    monkeypatch.setattr(cli_audit, "_distribution_skill_source", lambda: source)

    cli_audit.ceo_audit_install(force=True)

    assert (target / "scripts" / "audit.sh").is_file()
    assert os.access(target / "scripts" / "audit.sh", os.X_OK)
    assert os.access(target / "scripts" / "compat-bin" / "scout", os.X_OK)


def test_run_invokes_bash_with_resolved_script(tmp_path: Path, monkeypatch) -> None:
    skill = make_skill(tmp_path)
    calls: list[list[str]] = []
    monkeypatch.setattr(cli_audit, "_active_skill", lambda: skill)
    monkeypatch.setattr(
        cli_audit.shutil, "which", lambda name: "/bin/bash" if name == "bash" else None
    )
    monkeypatch.setattr(
        cli_audit.subprocess,
        "run",
        lambda args, check: calls.append(args) or SimpleNamespace(returncode=0),
    )

    with pytest.raises(typer.Exit) as exc:
        cli_audit.ceo_audit_run(
            repo="repo",
            profile="QUICK",
            grade="B",
            output="out",
            json_out=True,
            no_color=True,
        )

    assert exc.value.exit_code == 0
    assert calls == [
        [
            "/bin/bash",
            str(skill / "scripts" / "audit.sh"),
            "--profile=QUICK",
            "--grade=B",
            "--output=out",
            "--json",
            "--no-color",
            "repo",
        ]
    ]
