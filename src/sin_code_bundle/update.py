"""Update the SIN-Code Bundle and its Go toolchain.

Bumps the Python package (pipx upgrade) and rebuilds every Go tool repo
found under ``~/dev/SIN-Code-*-Tool/``. Pure stdlib + subprocess — no
network calls other than ``git pull`` and ``pipx upgrade``.

Docs: update.doc.md
"""

from __future__ import annotations

import os
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

# Result of probing a single Go tool repo
GIT_VERSION_FALLBACK = "unknown"


@dataclass
class GoTool:
    """A Go tool source repo + installed binary location."""

    name: str  # binary name, e.g. "discover"
    repo: Path  # e.g. ~/dev/SIN-Code-Discover-Tool
    binary: Path  # e.g. ~/.local/bin/discover


@dataclass
class UpdateResult:
    """Outcome of a single update step (Python bundle or one Go tool)."""

    target: str  # human label, e.g. "sin-code-bundle" or "discover"
    old_version: str
    new_version: str
    status: str  # "updated" | "up-to-date" | "failed" | "skipped" | "would-update"
    detail: str = ""  # extra context (error, command, ...)


# Default locations — overridable by config (see config.py)
DEFAULT_DEV_DIR = Path.home() / "dev"
DEFAULT_BIN_DIR = Path.home() / ".local" / "bin"
DEFAULT_PIPX_PKG = "sin-code-bundle"


# ── Discovery ────────────────────────────────────────────────────────────
def discover_go_tools(
    dev_dir: Path = DEFAULT_DEV_DIR, bin_dir: Path = DEFAULT_BIN_DIR
) -> list[GoTool]:
    """Find every ``~/dev/SIN-Code-*-Tool/`` repo that exposes a buildable Go binary.

    A repo is considered buildable if it has a ``cmd/<name>/main.go`` whose
    ``<name>`` matches a subdirectory under ``cmd/``. The corresponding
    installed binary is expected at ``bin_dir/<name>`` (matches the layout
    used by ``./install.sh`` in the bundle).
    """
    tools: list[GoTool] = []
    if not dev_dir.exists():
        return tools
    for repo in sorted(dev_dir.glob("SIN-Code-*-Tool")):
        cmd_dir = repo / "cmd"
        if not cmd_dir.is_dir():
            continue
        # pick the first cmd subdir whose name matches an expected binary
        for sub in sorted(cmd_dir.iterdir()):
            if not sub.is_dir():
                continue
            if not (sub / "main.go").exists():
                continue
            name = sub.name
            binary = bin_dir / name
            tools.append(GoTool(name=name, repo=repo, binary=binary))
            break  # one binary per repo is the convention
    return tools


# ── Version probes ───────────────────────────────────────────────────────
def _run(
    cmd: list[str], cwd: Path | None = None, timeout: int = 60
) -> subprocess.CompletedProcess[str]:
    """Run *cmd* and capture stdout/stderr. Never raises — non-zero exits
    surface as ``returncode`` so callers can decide how to react.
    """
    return subprocess.run(
        cmd, cwd=cwd, capture_output=True, text=True, timeout=timeout
    )


def _git_describe(repo: Path) -> str:
    """Return ``git describe --tags --always`` for *repo*, or fallback."""
    if not (repo / ".git").exists():
        return GIT_VERSION_FALLBACK
    res = _run(["git", "describe", "--tags", "--always"], cwd=repo)
    if res.returncode == 0 and res.stdout.strip():
        return res.stdout.strip()
    # No tags yet — fall back to short SHA
    res2 = _run(["git", "rev-parse", "--short", "HEAD"], cwd=repo)
    return res2.stdout.strip() if res2.returncode == 0 else GIT_VERSION_FALLBACK


def _git_branch(repo: Path) -> str:
    """Return the current branch name, or empty string when detached/empty."""
    res = _run(
        ["git", "rev-parse", "--abbrev-ref", "HEAD"], cwd=repo, timeout=10
    )
    if res.returncode != 0:
        return ""
    branch = res.stdout.strip()
    return "" if branch == "HEAD" else branch


def _binary_version(binary: Path) -> str:
    """Best-effort ``<binary> --version`` probe. Returns empty string on failure."""
    if not binary.exists():
        return ""
    res = _run([str(binary), "--version"], timeout=10)
    out = (res.stdout or res.stderr).strip()
    # Take just the first line to keep the table compact
    return out.splitlines()[0] if out else ""


def _pipx_version(pkg: str = DEFAULT_PIPX_PKG) -> str:
    """Return the installed version of *pkg* via ``pipx list`` JSON output."""
    if not shutil.which("pipx"):
        return ""
    res = _run(["pipx", "list", "--json"], timeout=30)
    if res.returncode != 0:
        return ""
    import json

    try:
        data = json.loads(res.stdout)
    except json.JSONDecodeError:
        return ""
    info = data.get("venvs", {}).get(pkg, {})
    # pipx 1.x puts the package version under venvs[].metadata[].version
    metadata = info.get("metadata", []) if isinstance(info, dict) else []
    for entry in metadata:
        if isinstance(entry, dict) and entry.get("package_name") == pkg:
            return entry.get("package_version", "")
    return ""


# ── Update steps ─────────────────────────────────────────────────────────
def update_python(
    pkg: str = DEFAULT_PIPX_PKG, *, check: bool = False
) -> UpdateResult:
    """Run ``pipx upgrade <pkg>`` (or print it in --check mode)."""
    old = _pipx_version(pkg)
    if check:
        return UpdateResult(
            target=pkg,
            old_version=old,
            new_version="(would upgrade)",
            status="would-update",
            detail=f"pipx upgrade {pkg}",
        )
    if not shutil.which("pipx"):
        return UpdateResult(
            target=pkg,
            old_version=old,
            new_version=old,
            status="failed",
            detail="pipx not on PATH",
        )
    res = _run(["pipx", "upgrade", pkg], timeout=180)
    new = _pipx_version(pkg)
    if res.returncode != 0:
        return UpdateResult(
            target=pkg,
            old_version=old,
            new_version=new or old,
            status="failed",
            detail=(res.stderr or res.stdout).strip().splitlines()[-1:]
            or ["unknown error"],
        )
    if new and old and new == old:
        status = "up-to-date"
    elif new and old and new != old:
        status = "updated"
    else:
        # can't determine — treat as updated if pipx reported success
        status = "updated" if not res.stderr else "up-to-date"
    return UpdateResult(
        target=pkg, old_version=old, new_version=new, status=status
    )


def update_go_tool(tool: GoTool, *, check: bool = False) -> UpdateResult:
    """Pull latest commits and rebuild one Go tool binary.

    Skips ``git pull`` when the repo is not on a branch (detached HEAD, or
    shallow clone) so we never accidentally fast-forward into a tag.
    """
    old_version = _git_describe(tool.repo)
    installed_version = _binary_version(tool.binary)
    # If the installed binary already reports the new version, we can
    # short-circuit and skip the rebuild entirely.
    already_current = bool(
        installed_version and old_version and installed_version.endswith(old_version)
    )

    branch = _git_branch(tool.repo)
    if not branch:
        if check:
            return UpdateResult(
                target=tool.name,
                old_version=installed_version,
                new_version="(skip pull — not on branch)",
                status="would-skip",
                detail=f"detached HEAD in {tool.repo}",
            )
        # Even without pulling, try to build what's already on disk.
        return _build_tool(tool, old_version, installed_version)

    if check:
        # --check: just report what we *would* do.
        return UpdateResult(
            target=tool.name,
            old_version=installed_version or old_version,
            new_version="(would pull + rebuild)",
            status="would-update",
            detail=(
                f"git pull --ff-only @ {branch} && "
                f"go build -o {tool.binary} ./cmd/{tool.name}"
            ),
        )

    if not already_current:
        pull = _run(
            ["git", "pull", "--ff-only"], cwd=tool.repo, timeout=120
        )
        if pull.returncode != 0:
            return UpdateResult(
                target=tool.name,
                old_version=installed_version,
                new_version=installed_version,
                status="failed",
                detail=f"git pull failed: {pull.stderr.strip().splitlines()[-1:]}",
            )

    return _build_tool(tool, _git_describe(tool.repo), installed_version)


def _build_tool(
    tool: GoTool, source_version: str, installed_version: str
) -> UpdateResult:
    """Run ``go build`` with a version ldflag and report success/failure."""
    if not shutil.which("go"):
        return UpdateResult(
            target=tool.name,
            old_version=installed_version,
            new_version=installed_version,
            status="failed",
            detail="go toolchain not on PATH",
        )
    # -X main.Version=<git describe> lets every Go tool report its own
    # build SHA through `binary --version`. Fall back to source_version
    # when no tags exist yet.
    ldflags = [f"-X main.Version={source_version}"]
    cmd = [
        "go",
        "build",
        "-ldflags",
        " ".join(ldflags),
        "-o",
        str(tool.binary),
        f"./cmd/{tool.name}",
    ]
    build = _run(cmd, cwd=tool.repo, timeout=300)
    if build.returncode != 0:
        return UpdateResult(
            target=tool.name,
            old_version=installed_version,
            new_version=installed_version,
            status="failed",
            detail=(build.stderr or build.stdout).strip().splitlines()[-1:]
            or ["build error"],
        )
    new_installed = _binary_version(tool.binary)
    status = "up-to-date" if new_installed == installed_version else "updated"
    return UpdateResult(
        target=tool.name,
        old_version=installed_version,
        new_version=new_installed,
        status=status,
    )


# ── Orchestration + rendering ────────────────────────────────────────────
def run_update(
    *,
    core: bool = False,
    go: bool = False,
    check: bool = False,
    dev_dir: Path = DEFAULT_DEV_DIR,
    bin_dir: Path = DEFAULT_BIN_DIR,
    pipx_pkg: str = DEFAULT_PIPX_PKG,
) -> list[UpdateResult]:
    """Top-level driver. Returns one result per step for the CLI to render.

    When neither *core* nor *go* is set, both run. The check flag is
    passed through to every step so no side effects happen.
    """
    do_core = core or not go
    do_go = go or not core
    results: list[UpdateResult] = []
    if do_core:
        results.append(update_python(pipx_pkg, check=check))
    if do_go:
        for tool in discover_go_tools(dev_dir=dev_dir, bin_dir=bin_dir):
            results.append(update_go_tool(tool, check=check))
    return results


def render_table(results: list[UpdateResult]) -> str:
    """Render results as a fixed-width text table for ``typer.echo``."""
    if not results:
        return "No update targets found."
    headers = ("Target", "Old", "New", "Status", "Detail")
    rows = [headers]
    for r in results:
        rows.append(
            (r.target, r.old_version or "-", r.new_version or "-", r.status, r.detail or "")
        )
    widths = [max(len(str(row[i])) for row in rows) for i in range(len(headers))]
    out: list[str] = []
    sep = "  "
    out.append(sep.join(h.ljust(widths[i]) for i, h in enumerate(headers)))
    out.append(sep.join("-" * widths[i] for i in range(len(headers))))
    for row in rows[1:]:
        out.append(sep.join(str(row[i]).ljust(widths[i]) for i in range(len(headers))))
    return "\n".join(out)


def _env_path(name: str, default: Path) -> Path:
    """Read *name* from env and expand ``~``; fall back to *default*."""
    raw = os.environ.get(name)
    if not raw:
        return default
    return Path(os.path.expanduser(raw))
