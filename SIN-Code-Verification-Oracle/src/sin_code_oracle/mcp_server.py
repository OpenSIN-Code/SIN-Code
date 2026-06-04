"""MCP server exposing the Verification Oracle to coding agents.

The single most valuable tool here is `verify_change`: an agent calls it before
declaring a task done, and gets back a Verdict grounded in reality rather than
its own optimism. `confidence` and `verified` let the agent know how much to
trust the answer.
"""

from __future__ import annotations

import json

try:
    from mcp.server.fastmcp import FastMCP
except ImportError:  # pragma: no cover
    FastMCP = None

from .diagnostics import DiagnosticsOracle
from .oracle import VerificationOracle


def main():
    if FastMCP is None:
        raise RuntimeError(
            "mcp package not installed. Install with: pip install 'sin-code-oracle[mcp]'"
        )

    mcp = FastMCP("sin-code-oracle")

    @mcp.tool()
    def verify_change(
        root: str = ".",
        test_command: str | None = None,
        build_command: str | None = None,
        run_diagnostics: bool = True,
        expected_behavior_change: bool = False,
    ) -> str:
        """Independently verify whether a change actually works.

        Runs compilers/type-checkers, build, and tests as ground truth. Returns
        a Verdict with `passed`, `verified`, `confidence`, and `reasons`. Does
        NOT trust any prior claim of success. Call this before reporting 'done'.
        """
        oracle = VerificationOracle(root=root)
        verdict = oracle.verify(
            test_command=test_command,
            build_command=build_command,
            run_diagnostics=run_diagnostics,
            expected_behavior_change=expected_behavior_change,
        )
        return json.dumps(verdict.as_dict(), indent=2)

    @mcp.tool()
    def run_diagnostics(root: str = ".") -> str:
        """Run all available type-checkers/compilers/linters and return diagnostics."""
        return json.dumps(DiagnosticsOracle().check(root).as_dict(), indent=2)

    mcp.run()


if __name__ == "__main__":
    main()
