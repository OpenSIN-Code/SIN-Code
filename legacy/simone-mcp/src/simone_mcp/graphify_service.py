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
    return os.path.join(root, "graphify-out", "graph.json")


def _has_graph(root: str) -> bool:
    return os.path.isfile(_graph_path(root))


def graphify_update(root: str) -> dict[str, Any]:
    """Run `graphify update <root>` to regenerate the knowledge graph."""
    root_path = os.path.abspath(os.path.expanduser(root))
    if not os.path.isdir(root_path):
        return {"ok": False, "error": f"directory not found: {root_path}"}
    return _run_graphify(["update", root_path], cwd=root_path)


def graphify_query(question: str, root: str, budget: int = 2000, dfs: bool = False) -> dict[str, Any]:
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
    """Run `graphify explain <node>` to understand a node."""
    root_path = os.path.abspath(os.path.expanduser(root))
    if not _has_graph(root_path):
        return {"ok": False, "error": f"No graph found at {_graph_path(root_path)}"}
    graph_arg = _graph_path(root_path)
    return _run_graphify(["explain", node, "--graph", graph_arg], cwd=root_path)


def graphify_path(source: str, target: str, root: str) -> dict[str, Any]:
    """Run `graphify path <source> <target>` to find shortest path."""
    root_path = os.path.abspath(os.path.expanduser(root))
    if not _has_graph(root_path):
        return {"ok": False, "error": f"No graph found at {_graph_path(root_path)}"}
    graph_arg = _graph_path(root_path)
    return _run_graphify(["path", source, target, "--graph", graph_arg], cwd=root_path)


def graphify_install(platform: str = "opencode") -> dict[str, Any]:
    """Run `graphify install --platform <platform>`."""
    return _run_graphify(["install", "--platform", platform])


def graphify_summary(root: str) -> dict[str, Any]:
    """Read graph.json and return summary stats (no CLI call)."""
    root_path = os.path.abspath(os.path.expanduser(root))
    gp = _graph_path(root_path)
    if not os.path.isfile(gp):
        return {"ok": False, "error": f"No graph found at {gp}", "has_graph": False}
    try:
        with open(gp) as f:
            graph = json.load(f)
        nodes = graph.get("nodes", [])
        edges = graph.get("edges", [])
        communities = graph.get("communities", [])
        god_nodes = sorted(
            nodes,
            key=lambda n: len(n.get("neighbors", [])),
            reverse=True,
        )[:10]
        return {
            "ok": True,
            "has_graph": True,
            "node_count": len(nodes),
            "edge_count": len(edges),
            "community_count": len(communities),
            "top_nodes": [
                {"name": n.get("label", n.get("id", "?")), "edges": len(n.get("neighbors", []))}
                for n in god_nodes
            ],
        }
    except Exception as e:
        return {"ok": False, "error": str(e), "has_graph": False}


def graphify_available() -> bool:
    return _find_graphify() is not None
