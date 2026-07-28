# SPDX-License-Identifier: MIT
"""Unified CLI fuer den gesamten SIN-Code Stack.

Subsysteme werden lazy und defensiv importiert: fehlt eines, bleibt der Rest
nutzbar und es wird eine klare Meldung statt eines Importfehlers ausgegeben.

Docs: cli.doc.md
"""

from __future__ import annotations

import typer

# Single source of truth for the app instance — cli_app defines all sub-apps
# and wires them to the root ``app`` via ``add_typer``.
from sin_code_bundle.cli_app import app  # noqa: F401
from sin_code_bundle.cli_ast import *  # noqa: F401,F403
from sin_code_bundle.cli_audit import *  # noqa: F401,F403
from sin_code_bundle.cli_browser import *  # noqa: F401,F403
from sin_code_bundle.cli_codocs import *  # noqa: F401,F403
from sin_code_bundle.cli_config import *  # noqa: F401,F403
from sin_code_bundle.cli_docs import *  # noqa: F401,F403
from sin_code_bundle.cli_git import *  # noqa: F401,F403
from sin_code_bundle.cli_gitnexus import *  # noqa: F401,F403
from sin_code_bundle.cli_hashline import *  # noqa: F401,F403
from sin_code_bundle.cli_lint import *  # noqa: F401,F403
from sin_code_bundle.cli_markitdown import *  # noqa: F401,F403
from sin_code_bundle.cli_misc import *  # noqa: F401,F403
from sin_code_bundle.cli_pocock import *  # noqa: F401,F403
from sin_code_bundle.cli_rtk import *  # noqa: F401,F403

# ── Extracted command modules ───────────────────────────────────────────────
# Each module imports its sub-app from cli_app and registers commands via
# decorators. Import order does not matter — all apps are already created.
from sin_code_bundle.cli_sckg import *  # noqa: F401,F403
from sin_code_bundle.cli_security import *  # noqa: F401,F403
from sin_code_bundle.cli_serve import *  # noqa: F401,F403
from sin_code_bundle.cli_sin_code import *  # noqa: F401,F403
from sin_code_bundle.cli_update import *  # noqa: F401,F403
from sin_code_bundle.cli_vfs import *  # noqa: F401,F403

# NOTE: The `sin memory {retain,recall,reflect,stats,forget}` and
# `sin memory {honcho-status,honcho-retain,honcho-chat}` + `sin context query`
# sub-commands were removed in this commit. They referenced `SINMemory` and
# `HonchoBackend` classes that were moved to the external `sin-brain` package
# (see commit af69464, BR-1, Issue #14). The bundle's `memory.py` is now a
# thin pass-through adapter to `sin_brain.mcp_tools` and exposes the five
# memory operations only as MCP tools (`recall`, `remember`, `forget`, `pin`,
# `link_evidence`) registered by `sin serve` — not as CLI sub-commands.
# Honcho integration is intentionally out of scope for this bundle: the
# real memory backend is `sin-brain` (SQLite + FTS5, MIT, 1500+ LOC).
# See `src/sin_code_bundle/memory.doc.md` for the current architecture.

# ── v0.9.3 Consolidated Skill Subcommands (issue #29) ─────────────────────
# Migrated 3 baseline skills into the bundle CLI:
#   - sin-slash           -> sin slash <sub>
#   - sin-mcp-server-builder -> sin mcp-server <sub>
#   - sin-marketplace     -> sin marketplace <sub>
# Source repos are now archived (see DEPRECATED notice in their READMEs).
try:
    from sin_code_bundle.tools.slash.app import app as slash_app

    app.add_typer(slash_app, name="slash")
except ImportError as exc:

    @app.command("slash")
    def slash_missing(exc=exc) -> None:
        """Slash commands (slash module not installed)."""
        typer.echo(f"[SIN-BUNDLE] slash module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)


try:
    from sin_code_bundle.tools.mcp_server_builder.app import app as mcp_server_app

    app.add_typer(mcp_server_app, name="mcp-server")
except ImportError as exc:

    @app.command("mcp-server")
    def mcp_server_missing(exc=exc) -> None:
        """MCP server builder (mcp_server_builder module not installed)."""
        typer.echo(f"[SIN-BUNDLE] mcp-server module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)


try:
    from sin_code_bundle.tools.marketplace.app import app as marketplace_app

    app.add_typer(marketplace_app, name="marketplace")
except ImportError as exc:

    @app.command("marketplace")
    def marketplace_missing(exc=exc) -> None:
        """Marketplace (marketplace module not installed)."""
        typer.echo(f"[SIN-BUNDLE] marketplace module unavailable: {exc}", err=True)
        raise typer.Exit(code=1)


if __name__ == "__main__":
    app()
