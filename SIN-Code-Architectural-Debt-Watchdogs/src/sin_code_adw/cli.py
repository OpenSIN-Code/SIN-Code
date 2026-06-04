"""CLI fuer den Architectural Debt Watchdog."""
from __future__ import annotations

import json
from typing import Optional

import typer

from .complexity import ComplexityAnalyzer
from .cost_tracker import CostTracker
from .daemon import WatchdogDaemon

app = typer.Typer(help="SIN-Code Architectural Debt Watchdog CLI")

_EXCLUDE = {"venv", ".venv", "node_modules", ".git", "__pycache__"}


@app.command()
def scan(root: str = "."):
    """Scan repository for architectural debt."""
    analyzer = ComplexityAnalyzer()
    reports = analyzer.analyze(root, exclude=_EXCLUDE)
    debt = analyzer.debt_score(reports)
    hotspots: list[dict] = []
    for r in reports:
        for h in r.hotspots:
            hotspots.append({"file": r.path, **h})
    typer.echo(
        json.dumps(
            {
                "debt": debt,
                "files_scanned": len(reports),
                "top_hotspots": sorted(
                    hotspots, key=lambda x: -x["complexity"]
                )[:10],
            },
            indent=2,
        )
    )


@app.command()
def costs(agent: Optional[str] = None, task: Optional[str] = None):
    """Show tracked costs."""
    tracker = CostTracker()
    typer.echo(json.dumps(tracker.total_for(agent, task), indent=2))


@app.command()
def record(
    model: str,
    prompt_tokens: int,
    completion_tokens: int,
    agent: str = "default",
    task: str = "",
):
    """Record a cost entry."""
    tracker = CostTracker()
    entry = tracker.record(model, prompt_tokens, completion_tokens, agent, task)
    typer.echo(json.dumps(entry.__dict__, indent=2))


@app.command()
def watch(root: str = ".", interval: int = 30):
    """Run watchdog daemon."""
    wd = WatchdogDaemon(root, interval)
    wd.start()
    typer.echo(f"[ADW] Watchdog started on {root}. Press Ctrl+C to stop.")
    try:
        import time

        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        wd.stop()


if __name__ == "__main__":
    app()
