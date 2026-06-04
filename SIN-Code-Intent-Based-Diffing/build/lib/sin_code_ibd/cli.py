"""CLI for Intent-Based Diffing."""
from __future__ import annotations

import json
from pathlib import Path

import typer
from rich.console import Console
from rich.panel import Panel

from .ast_diff import ASTDiff
from .intent_summarizer import IntentSummarizer
from .risk_scorer import RiskScorer

app = typer.Typer(help="SIN-Code Intent-Based Diffing CLI")
console = Console()


@app.command()
def diff(file_a: Path, file_b: Path, json_output: bool = typer.Option(False, "--json")):
    """Diff two files semantically."""
    ad = ASTDiff()
    changes = ad.diff_files(str(file_a), str(file_b))
    intents = IntentSummarizer().summarize(changes)
    risk = RiskScorer().score(changes)

    if json_output:
        console.print_json(json.dumps({
            "changes": [c.__dict__ for c in changes],
            "intents": [i.__dict__ for i in intents],
            "risk": risk,
        }))
        return

    console.print(Panel(f"[bold]Risk: {risk['risk']} ({risk['score']})[/bold]", title="SIN-Code IBD"))
    for intent in intents:
        color = {"high": "red", "medium": "yellow", "low": "green"}.get(intent.risk, "white")
        console.print(f"[{color}][{intent.risk.upper()}][/{color}] {intent.headline}")
        console.print(f"    {intent.rationale}")


@app.command()
def review_git():
    """Review current uncommitted changes."""
    try:
        import git
    except ImportError:
        console.print("[red]gitpython required[/red]")
        raise typer.Exit(1)
    repo = git.Repo(".", search_parent_directories=True)
    diffs = repo.index.diff(None)
    ad = ASTDiff()
    all_changes = []
    for d in diffs:
        if not (d.a_path or "").endswith(".py"):
            continue
        try:
            old = repo.git.show(f"HEAD:{d.a_path}").encode()
        except Exception:
            old = b""
        try:
            with open(d.a_path, "rb") as f:
                new = f.read()
        except Exception:
            new = b""
        all_changes.extend(ad.diff_strings(old, new, d.a_path))
    intents = IntentSummarizer().summarize(all_changes)
    risk = RiskScorer().score(all_changes)
    console.print(Panel(f"[bold]Overall Risk: {risk['risk']} ({risk['score']})[/bold]"))
    for i in intents:
        console.print(f" - {i.headline}")


if __name__ == "__main__":
    app()
