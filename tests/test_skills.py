from pathlib import Path

import pytest

from sin_code_bundle.skills import Skill, compile_skills, render_skill

SAMPLE = """---
name: demo
description: A demo skill.
arguments:
  - name: target
    description: thing to act on
    required: true
---
Refactor {{target}} carefully and verify.
"""


@pytest.fixture
def skill(tmp_path: Path) -> Skill:
    p = tmp_path / "demo.md"
    p.write_text(SAMPLE, encoding="utf-8")
    return Skill.parse(p)


def test_parse_frontmatter(skill: Skill):
    assert skill.name == "demo"
    assert skill.description == "A demo skill."
    assert skill.arguments[0]["name"] == "target"
    assert "{{target}}" in skill.body


def test_render_opencode(skill: Skill):
    path, content = render_skill(skill, "opencode")
    assert path == ".opencode/command/demo.md"
    assert "description: A demo skill." in content
    assert "agent: build" in content
    assert "{{target}}" in content


def test_render_codex_maps_positional_args(skill: Skill):
    _, content = render_skill(skill, "codex")
    assert "$1" in content
    assert "{{target}}" not in content


def test_render_claude(skill: Skill):
    path, content = render_skill(skill, "claude")
    assert path == ".claude/skills/demo/SKILL.md"
    assert "name: demo" in content
    assert "{{target}}" in content


def test_compile_writes_files(tmp_path: Path):
    src = tmp_path / "skills"
    src.mkdir()
    (src / "demo.md").write_text(SAMPLE, encoding="utf-8")
    out = tmp_path / "repo"
    written = compile_skills("opencode", source_dir=src, out_root=out)
    assert written
    assert written[0].exists()
    assert "demo" in written[0].read_text()


def test_compile_dry_run_does_not_write(tmp_path: Path):
    src = tmp_path / "skills"
    src.mkdir()
    (src / "demo.md").write_text(SAMPLE, encoding="utf-8")
    out = tmp_path / "repo"
    written = compile_skills("opencode", source_dir=src, out_root=out, dry_run=True)
    assert written
    assert not written[0].exists()


def test_load_skills_empty_dir(tmp_path: Path):
    from sin_code_bundle.skills import load_skills

    result = load_skills(tmp_path / "no-such-dir")
    assert result == []


def test_missing_frontmatter_raises(tmp_path: Path):
    p = tmp_path / "bad.md"
    p.write_text("No frontmatter here.\n", encoding="utf-8")
    with pytest.raises(ValueError, match="frontmatter"):
        Skill.parse(p)
