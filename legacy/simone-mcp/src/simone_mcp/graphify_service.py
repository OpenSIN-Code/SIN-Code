"""Thin wrapper around the external `graphify` CLI.

Looks up the binary in standard install locations, caches the result,
and shells out to it for `update` / `query` / `explain` / `path` /
`install` subcommands. Also exposes `graphify_summary` which reads
`graphify-out/graph.json` directly (no CLI call) for the dashboard.

Docs: graphify_service.doc.md
"""
from __future__ import annotations

import json
import logging
import os
import subprocess
import tempfile
from pathlib import Path
from typing import Any

logger = logging.getLogger(__name__)

_GRAPHIFY_BIN: str | None = None


def _find_graphify() -> str | None:
    """Locate the `graphify` binary on the system.

    Caches the result in a module global. Search order:
      1. Common install locations (Homebrew on Apple Silicon, /usr/local/bin,
         ~/.local/bin).
      2. `which graphify` as a last resort.

    Returns the absolute path or `None` if not found.
    """
    global _GRAPHIFY_BIN
    if _GRAPHIFY_BIN is not None:
        return _GRAPHIFY_BIN
    candidates = [
        "/opt/homebrew/bin/graphify",
        "/usr/local/bin/graphify",
        os.path.expanduser("~/.local/bin/graphify"),
    ]
    for c in candidates:
        if os.path.isfile(c) and os.access(c, os.X_OK):
            _GRAPHIFY_BIN = c
            return c
    which = os.popen("which graphify 2>/dev/null").read().strip()
    if which and os.path.isfile(which):
        _GRAPHIFY_BIN = which
        return which
    _GRAPHIFY_BIN = ""
    return None


def _run_graphify(args: list[str], cwd: str | None = None) -> dict[str, Any]:
    """Run `graphify <args>` in a subprocess and capture stdout/stderr.

    Returns `{"ok": True, "output", "stderr"}` on success or
    `{"ok": False, "error": "..."}` on any failure. 120s timeout.
    """
    gpath = _find_graphify()
    global _GRAPHIFY_BIN
    if _GRAPHIFY_BIN is not None:
        return _GRAPHIFY_BIN
    candidates = [
        "/opt/homebrew/bin/graphify",
        "/usr/local/bin/graphify",
        os.path.expanduser("~/.local/bin/graphify"),
    ]
    for c in candidates:
        if os.path.isfile(c) and os.access(c, os.X_OK):
            _GRAPHIFY_BIN = c
            return c
    which = os.popen("which graphify 2>/dev/null").read().strip()
    if which and os.path.isfile(which):
        _GRAPHIFY_BIN = which
        return which
    _GRAPHIFY_BIN = ""
    return None


def _run_graphify(args: list[str], cwd: str | None = None) -> dict[str, Any]:
    gpath = _find_graphify()
    if not gpath:
        return {"ok": False, "error": "graphify not installed"}
    try:
        result = subprocess.run(
            [gpath] + args,
            cwd=cwd,
            capture_output=True,
            text=True,
            timeout=120,
        )
        output = result.stdout.strip()
        stderr = result.stderr.strip()
        if result.returncode != 0:
            error_msg = stderr or output or f"exit code {result.returncode}"
            return {"ok": False, "error": error_msg}
        return {"ok": True, "output": output, "stderr": stderr}
    except subprocess.TimeoutExpired:
        return {"ok": False, "error": "graphify command timed out (120s)"}
    except FileNotFoundError:
        return {"ok": False, "error": "graphify not found"}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def _graph_path(root: str) -> str:
    """Return the canonical path to the graphify output for `root`."""
    return os.path.join(root, "graphify-out", "graph.json")


def _has_graph(root: str) -> bool:
    """True iff a graph.json exists at the canonical path for `root`."""
    return os.path.isfile(_graph_path(root))


def graphify_update(root: str) -> dict[str, Any]:
    """Run `graphify update <root>` to regenerate the knowledge graph.

    Args:
        root: Workspace root. Must be an existing directory.

    Returns:
        `{"ok": True, "output", "stderr"}` on success,
        `{"ok": False, "error"}` if `root` doesn't exist or the CLI fails.
    """
    root_path = os.path.abspath(os.path.expanduser(root))
    if not os.path.isdir(root_path):
        return {"ok": False, "error": f"directory not found: {root_path}"}
    return _run_graphify(["update", root_path], cwd=root_path)


def graphify_query(question: str, root: str, budget: int = 2000, dfs: bool = False) -> dict[str, Any]:
    """Ask a question about a repo's knowledge graph.

    Args:
        question: Natural-language question.
        root: Workspace root.
        budget: Max output tokens (default 2000).
        dfs: If True, use depth-first traversal instead of the default BFS.

    Returns:
        `{"ok": True, "output"}` or `{"ok": False, "error"}`.
    """
    """Run `graphify query <question>` on a repo's graph."""
    root_path = os.path.abspath(os.path.expanduser(root))
    if not _has_graph(root_path):
        return {"ok": False, "error": f"No graph found at {_graph_path(root_path)}. Run graphify_update first."}
    graph_arg = _graph_path(root_path)

    args = ["query", question, "--graph", graph_arg, "--budget", str(budget)]
    if dfs:
        args.append("--dfs")

    result = _run_graphify(args, cwd=root_path)
    return result


def graphify_explain(node: str, root: str) -> dict[str, Any]:
    """Run `graphify explain <node>` to understand a node's role in the graph.

    Args:
        node: Node label to explain.
        root: Workspace root.

    Returns:
        `{"ok": True, "output"}` or `{"ok": False, "error"}`.
    """
    root_path = os.path.abspath(os.path.expanduser(root))
    if not _has_graph(root_path):
        return {"ok": False, "error": f"No graph found at {_graph_path(root_path)}"}
    graph_arg = _graph_path(root_path)
    return _run_graphify(["explain", node, "--graph", graph_arg], cwd=root_path)


def graphify_path(source: str, target: str, root: str) -> dict[str, Any]:
    """Run `graphify path <source> <target>` to find shortest path.

    Args:
        source: Source node label.
        target: Target node label.
        root: Workspace root.

    Returns:
        `{"ok": True, "output"}` or `{"ok": False, "error"}`.
    """
    root_path = os.path.abspath(os.path.expanduser(root))
    if not _has_graph(root_path):
        return {"ok": False, "error": f"No graph found at {_graph_path(root_path)}"}
    graph_arg = _graph_path(root_path)
    return _run_graphify(["path", source, target, "--graph", graph_arg], cwd=root_path)


def graphify_install(platform: str = "opencode") -> dict[str, Any]:
    """Run `graphify install --platform <platform>`.

    Args:
        platform: Target platform (default "opencode"). Picks the right
                   integration config (MCP, hook, etc.).
    """
    return _run_graphify(["install", "--platform", platform])


def graphify_summary(root: str) -> dict[str, Any]:
    """Read graph.json and return summary stats (no CLI call).

    Used by the dashboard and `get_project_overview` to surface graph
    info without paying the cost of a `graphify` invocation.

    Returns:
        `{"ok": True, "has_graph": True, "node_count", "edge_count",
          "community_count", "top_nodes": [{name, edges}, ...]}` on success,
        or `{"ok": False, "error", "has_graph": False}` if no graph exists.
    """
    root_path = os.path.abspath(os.path.expanduser(root))
    gp = _graph_path(root_path)
    if not os.path.isfile(gp):
        return {"ok": False, "error": f"No graph found at {gp}", "has_graph": False}
    try:
        with open(gp) as f:
            graph = json.load(f)
        nodes = graph.get("nodes", [])
        links = graph.get("links", [])
        hyperedges = graph.get("hyperedges", graph.get("communities", []))
        node_degree: dict[str, int] = {}
        for link in links:
            src = str(link.get("source", ""))
            tgt = str(link.get("target", ""))
            node_degree[src] = node_degree.get(src, 0) + 1
            node_degree[tgt] = node_degree.get(tgt, 0) + 1
        top_nodes = sorted(
            [
                {"name": n.get("label", n.get("id", str(n.get("name", "?")))), "edges": node_degree.get(str(n.get("id", "")), 0)}
                for n in nodes if isinstance(n, dict)
            ],
            key=lambda x: -x["edges"],
        )[:10]
        return {
            "ok": True,
            "has_graph": True,
            "node_count": len(nodes),
            "edge_count": len(links),
            "community_count": len(hyperedges),
            "top_nodes": top_nodes,
        }
    except Exception as e:
        return {"ok": False, "error": str(e), "has_graph": False}


def graphify_available() -> bool:
    """True iff the `graphify` binary is on the system."""
    return _find_graphify() is not None
