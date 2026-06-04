"""CLI for Proof of Correctness."""
from __future__ import annotations

import importlib.util
import json
from pathlib import Path

import typer

from .property_generator import PropertyGenerator
from .spec_compiler import SpecCompiler
from .runtime_verifier import RuntimeVerifier

app = typer.Typer(help="SIN-Code Proof of Correctness CLI")


def _load_function(module_path: Path, function_name: str):
    spec = importlib.util.spec_from_file_location("_poc_target", module_path)
    if spec is None or spec.loader is None:
        raise typer.BadParameter(f"Cannot load module {module_path}")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    if not hasattr(mod, function_name):
        raise typer.BadParameter(f"{function_name} not found in {module_path}")
    return getattr(mod, function_name)


@app.command()
def suggest(module_path: Path, function_name: str):
    """Suggest properties for a function."""
    fn = _load_function(module_path, function_name)
    sig = RuntimeVerifier.extract_signature(fn)
    pg = PropertyGenerator()
    props = pg.suggest(sig)
    typer.echo(pg.render_hypothesis_test(function_name, props))


@app.command()
def verify(module_path: Path, function_name: str, max_examples: int = 50):
    """Run runtime verification on a function."""
    fn = _load_function(module_path, function_name)
    rv = RuntimeVerifier(max_examples=max_examples)
    results = rv.verify_function(fn)
    typer.echo(json.dumps(results, indent=2))


@app.command()
def from_intent(intent: str, function_name: str):
    """Compile a natural-language intent into a spec test."""
    sc = SpecCompiler()
    spec = sc.from_intent(intent, function_name)
    typer.echo(sc.compile(spec))


if __name__ == "__main__":
    app()
