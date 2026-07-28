# SPDX-License-Identifier: MIT
"""Config sub-commands — extracted from cli.py."""

from __future__ import annotations

import json

import typer

from sin_code_bundle.cli_app import config_app


@config_app.callback()
def _config_callback() -> None:
    """Manage the layered SIN-Code configuration."""


@config_app.command("show")
def config_show(
    json_out: bool = typer.Option(False, "--json", help="Output JSON instead of text."),
) -> None:
    """Show the resolved config (all sources merged, secrets redacted)."""
    from sin_code_bundle import config as cfg

    payload, origins = cfg.merged()
    if json_out:
        typer.echo(
            json.dumps(
                {
                    "config": payload,
                    "origins": {
                        k: {"label": v.label, "path": str(v.path)} for k, v in origins.items()
                    },
                },
                indent=2,
                default=str,
            )
        )
    else:
        typer.echo(cfg.format_show(payload, origins))


@config_app.command("get")
def config_get(key: str = typer.Argument(..., help="Dotted key, e.g. tui.theme")) -> None:
    """Print the value of a single config key (respects redaction)."""
    from sin_code_bundle import config as cfg

    view = cfg.get(key)
    if view.value is cfg.MISSING:
        typer.echo(f"(unset: {key})", err=True)
        raise typer.Exit(code=1)
    if cfg._is_sensitive(key):
        typer.echo(cfg.REDACTED_PLACEHOLDER)
    else:
        typer.echo(view.value)


@config_app.command("set")
def config_set(
    key: str = typer.Argument(..., help="Dotted key, e.g. tui.theme"),
    value: str = typer.Argument(..., help="Value to store (always as string)"),
) -> None:
    """Set a config value in ./sin.config.toml (project-local)."""
    from sin_code_bundle import config as cfg

    try:
        path = cfg.set_value(key, value)
    except ValueError as exc:
        typer.echo(f"[SIN-BUNDLE] {exc}", err=True)
        raise typer.Exit(code=1)
    typer.echo(f"{key} = {value!r}  ->  {path}")


@config_app.command("unset")
def config_unset(key: str = typer.Argument(..., help="Dotted key to remove")) -> None:
    """Remove a config value from ./sin.config.toml."""
    from sin_code_bundle import config as cfg

    if cfg.unset_value(key):
        typer.echo(f"Removed: {key}")
    else:
        typer.echo(f"(was unset: {key})")
        raise typer.Exit(code=1)


@config_app.command("path")
def config_path() -> None:
    """Print every config file path the resolver checks, with existence markers."""
    from sin_code_bundle import config as cfg

    typer.echo(cfg.format_path(cfg.all_paths()))
