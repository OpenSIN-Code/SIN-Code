#!/usr/bin/env python3
"""Add filemode import to a Go file (avoiding duplicates) and convert the
0o644 literals to filemode.Default() literals.

Operates file-by-file; safe to re-run (idempotent on already-converted
files because the literal 0o644 will be gone).
"""
import re
import sys
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
IMPORT_PATH = "github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
IMPORT_ALIAS = "filemode"  # package name = filemode, no alias needed


def split_imports_block(content: str) -> tuple[str, str, str]:
    """Return (pre, block, post) covering the entire `import (...)` group.

    The block captured is inclusive of the trailing `)`.
    """
    m = re.search(r"^import\s*\(\s*\n", content, flags=re.MULTILINE)
    if not m:
        return "", "", ""
    start = m.start()
    depth = 1
    i = m.end()
    while i < len(content) and depth > 0:
        if content[i] == "(":
            depth += 1
        elif content[i] == ")":
            depth -= 1
        i += 1
    return content[:start], content[start:i], content[i:]


def split_single_import(content: str) -> tuple[str, str, str]:
    """Return (pre, stmt, post) for the first `import "x"` single-line.
    Only used when there is NO block import.
    """
    m = re.search(r"^import\s+\"[^\"]+\"\s*\n", content, flags=re.MULTILINE)
    if not m:
        return "", "", ""
    return content[: m.start()], content[m.start(): m.end()], content[m.end():]


def collect_imports(block: str) -> list[str]:
    """Yield import lines (each strip()-ed, no comments deleted) inside
    `import ( ... )`. Comments and blank lines are preserved separately.
    """
    lines = block.splitlines(keepends=False)
    inner = lines[1:-1]  # drop `import (` and `)`
    return [ln.strip() for ln in inner]


def insert_import_line(block: str, import_line: str) -> str:
    """Insert `import_line` into the block alphabetically among quoted
    imports. We only consider lines that start with `"` for placement.
    Blank/comment/paren-only lines are skipped over.
    """
    if import_line in block:
        return block
    head = "import (\n"
    foot = ")\n"
    inner = block[len(head):-len(foot)]

    # split into lines preserving blank lines and comments
    raw_lines = inner.splitlines(keepends=False)
    quoted = [ln for ln in raw_lines if ln.lstrip().startswith('"')]
    if not quoted or import_line < min(quoted):
        # insert at the top of the inner block
        inner_lines = [import_line, ""] + raw_lines
        return head + "\n".join(inner_lines) + "\n" + foot
    if import_line > max(quoted):
        inner_lines = raw_lines + ["", import_line]
        return head + "\n".join(inner_lines) + "\n" + foot
    # find the right alphabetical slot among quoted-only lines (re-emit
    # all raw lines but insert in the right position)
    inserted = False
    new_inner = []
    for ln in raw_lines:
        if (
            not inserted
            and ln.lstrip().startswith('"')
            and ln.strip() > import_line
        ):
            new_inner.append(import_line)
            inserted = True
        new_inner.append(ln)
    if not inserted:
        new_inner.append(import_line)
    return head + "\n".join(new_inner) + "\n" + foot


def ensure_import(content: str) -> str:
    """Add the filemode import to a Go file. Returns the new content."""
    if IMPORT_PATH in content or "internal/filemode\"" in content:
        return content
    if "\nimport (\n" in content:
        pre, block, post = split_imports_block(content)
        new_block = insert_import_line(block, f'\t"{IMPORT_PATH}"')
        return pre + new_block + post
    # No block — look for a single-line import and convert to block
    pre, stmt, post = split_single_import(content)
    if not stmt:
        # No imports at all — insert a fresh import block right after the
        # package line.
        pkg_m = re.search(r"^package\s+\w+\s*\n", content, flags=re.MULTILINE)
        if not pkg_m:
            return content
        end_of_pkg = pkg_m.end()
        new_block = "import (\n\t\"" + IMPORT_PATH + "\"\n)\n\n"
        return content[:end_of_pkg] + new_block + content[end_of_pkg:]
    # Append the new import to the single import and turn it into a block.
    new_block = (
        "import (\n"
        + "\t" + stmt.replace("import ", "").strip() + "\n"
        + "\t\"" + IMPORT_PATH + "\"\n"
        + ")\n"
    )
    return pre + new_block + post


def transform_file(path: Path) -> bool:
    """Apply the substitution. Returns True if the file was modified."""
    src = path.read_text()
    if "0o644" not in src:
        return False
    new = src.replace("0o644", "filemode.Default()")
    new = ensure_import(new)
    if new != src:
        path.write_text(new)
        return True
    return False


def main() -> int:
    targets = [REPO / line.strip() for line in Path("/tmp/filemode_targets.txt").read_text().splitlines() if line.strip()]
    # Skip the filemode package itself — its 0o644 IS the policy fallback.
    targets = [t for t in targets if "/internal/filemode/" not in str(t)]
    changed: list[Path] = []
    for t in targets:
        if not t.exists():
            print(f"missing: {t}", file=sys.stderr)
            continue
        if transform_file(t):
            changed.append(t)
    print(f"modified {len(changed)} files")
    for c in changed:
        print(f"  {c.relative_to(REPO)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
