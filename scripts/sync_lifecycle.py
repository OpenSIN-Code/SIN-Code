#!/usr/bin/env python3
"""Sync skill lifecycle frontmatter from the mapping table.

Reads `scripts/lifecycle_map.yaml` and ensures every bundled
SKILL.md has a `lifecycle: <value>` line in its YAML frontmatter.

Usage:
    python3 sync_lifecycle.py --check      # exit 1 if any drift
    python3 sync_lifecycle.py --apply      # write changes
    python3 sync_lifecycle.py --diff       # show what would change

The script is intentionally stdlib-only (yaml is via a small hand-
rolled parser — we use it for one structure with string leaves and a
list of dicts; PyYAML is not a dependency for the bundled scripts).

Docs: scripts/sync_lifecycle.py.doc.md
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

# Repo root is two levels up from this script.
REPO_ROOT = Path(__file__).resolve().parent.parent
SKILLS_DIR = REPO_ROOT / "skills"
MAP_PATH = Path(__file__).resolve().parent / "lifecycle_map.yaml"

# Lifecycle values, exactly as required by issue #139.
VALID_LIFECYCLES = {"native", "external", "deprecated"}

# Frontmatter delimiters in a SKILL.md file.
FRONTMATTER_RE = re.compile(
    r"^---\s*\n(.*?)\n---\s*\n(.*)$",
    re.DOTALL,
)


def parse_map(text: str) -> dict[str, str]:
    """Parse lifecycle_map.yaml into {name: lifecycle} dict.

    The format is intentionally simple (no nested keys, no anchors,
    no flow style). Lines starting with `#` are comments. Each
    non-comment line is either a key (`key: value`) or a list item
    under the `skills:` key.
    """
    out: dict[str, str] = {}
    in_skills = False
    current: dict[str, str] = {}
    for raw in text.splitlines():
        line = raw.rstrip()
        if not line or line.lstrip().startswith("#"):
            continue
        if line.startswith("skills:"):
            in_skills = True
            continue
        if not in_skills:
            continue
        if line.startswith("  - "):
            # New list item. Flush previous.
            if current:
                if "name" in current and "lifecycle" in current:
                    out[current["name"]] = current["lifecycle"]
                current = {}
            # Strip the leading "  - "
            item = line[4:].strip()
            if ":" in item:
                k, _, v = item.partition(":")
                current[k.strip()] = v.strip()
        elif line.startswith("    "):
            # Continuation of current item.
            item = line.strip()
            if ":" in item:
                k, _, v = item.partition(":")
                current[k.strip()] = v.strip()
    if current:
        if "name" in current and "lifecycle" in current:
            out[current["name"]] = current["lifecycle"]
    return out


def read_frontmatter(path: Path) -> tuple[dict[str, str], str, str] | None:
    """Return (frontmatter_dict, body_before_close, full_body) or None.

    The frontmatter dict is the parsed keys/values. The body is the
    rest of the file (markdown content). full_body is the original
    text (for the re-emit path).
    """
    text = path.read_text()
    m = FRONTMATTER_RE.match(text)
    if not m:
        return None
    fm_text = m.group(1)
    body = m.group(2)
    fm: dict[str, str] = {}
    for line in fm_text.splitlines():
        if ":" in line and not line.startswith(" "):
            k, _, v = line.partition(":")
            fm[k.strip()] = v.strip()
    return fm, body, text


def write_frontmatter(path: Path, fm: dict[str, str], body: str) -> None:
    """Re-emit the SKILL.md with the updated frontmatter.

    We preserve key order by re-issuing the frontmatter dict in the
    order it was parsed, then appending any new keys at the end. This
    is a stable round-trip: the same dict produces the same bytes.
    """
    lines = ["---"]
    for k, v in fm.items():
        # Quote values that contain colons or special characters.
        v = v.strip()
        if any(c in v for c in [":", "#", "{", "}", "[", "]"]):
            v = f'"{v}"'
        lines.append(f"{k}: {v}")
    lines.append("---")
    lines.append("")  # blank line after frontmatter
    lines.append(body.lstrip("\n"))
    new_text = "\n".join(lines)
    path.write_text(new_text)


def check_one(skill_md: Path, expected_lifecycle: str) -> str | None:
    """Return None if the file is in sync, or a human-readable diff line."""
    parsed = read_frontmatter(skill_md)
    if parsed is None:
        return f"{skill_md}: no frontmatter"
    fm, _, _ = parsed
    actual = fm.get("lifecycle", "").strip()
    if actual != expected_lifecycle:
        return f"{skill_md}: lifecycle={actual!r}, expected {expected_lifecycle!r}"
    return None


def apply_one(skill_md: Path, expected_lifecycle: str) -> bool:
    """Update the lifecycle field. Return True if the file changed."""
    parsed = read_frontmatter(skill_md)
    if parsed is None:
        return False
    fm, body, _ = parsed
    if fm.get("lifecycle", "").strip() == expected_lifecycle:
        return False
    fm["lifecycle"] = expected_lifecycle
    write_frontmatter(skill_md, fm, body)
    return True


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    g = ap.add_mutually_exclusive_group(required=True)
    g.add_argument("--check", action="store_true", help="exit 1 if any drift")
    g.add_argument("--apply", action="store_true", help="write changes")
    g.add_argument("--diff", action="store_true", help="show what would change")
    args = ap.parse_args()

    text = MAP_PATH.read_text()
    mapping = parse_map(text)

    # Sanity: every lifecycle value is valid.
    for name, lc in mapping.items():
        if lc not in VALID_LIFECYCLES:
            print(f"lifecycle_map.yaml: {name} has invalid lifecycle {lc!r}", file=sys.stderr)
            return 2

    drift: list[str] = []
    missing: list[str] = []

    # Find each skill by walking the category directories. The
    # map's `name` is the basename (e.g. "skill-process-goal"),
    # not the full path.
    def find_skill_md(name: str) -> Path | None:
        for category_dir in SKILLS_DIR.iterdir():
            if not category_dir.is_dir():
                continue
            candidate = category_dir / name / "SKILL.md"
            if candidate.exists():
                return candidate
        return None

    for name, expected in sorted(mapping.items()):
        path = find_skill_md(name)
        if path is None:
            missing.append(f"{name}: not found under skills/*/")
            continue
        d = check_one(path, expected)
        if d is not None:
            drift.append(d)

    # Detect bundled skills not in the map (orphans).
    for category_dir in sorted(SKILLS_DIR.iterdir()):
        if not category_dir.is_dir():
            continue
        for skill_dir in sorted(category_dir.iterdir()):
            if not skill_dir.is_dir():
                continue
            sm = skill_dir / "SKILL.md"
            if not sm.exists():
                continue
            if skill_dir.name not in mapping:
                missing.append(f"{sm}: not in lifecycle_map.yaml")

    if args.check:
        if drift or missing:
            if drift:
                print(f"Drift detected ({len(drift)} files):")
                for d in drift:
                    print(f"  {d}")
            if missing:
                print(f"Orphans ({len(missing)} entries):")
                for m in missing:
                    print(f"  {m}")
            return 1
        print(f"all {len(mapping)} skills in sync with lifecycle_map.yaml")
        return 0

    if args.diff:
        for d in drift:
            print(d)
        for m in missing:
            print(m)
        return 0

    if args.apply:
        changed = 0
        for name, expected in sorted(mapping.items()):
            path = find_skill_md(name)
            if path is None:
                continue
            if apply_one(path, expected):
                changed += 1
                print(f"updated {path}")
        print(f"{changed} files updated, {len(missing)} orphans, {len(drift)} drift")
        return 0

    return 0


if __name__ == "__main__":
    sys.exit(main())
