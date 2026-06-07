# SPDX-License-Identifier: MIT
"""CLI for the SBOM generator.

Docs: cli.doc.md
"""

import json
import sys
from pathlib import Path
from typing import Optional

import click
from rich.console import Console
from rich.table import Table
from rich.panel import Panel
from rich.text import Text

from .generator import SBOMGenerator
from .models import SBOM, SBOMPackage, SBOMMetadata


console = Console()


@click.group()
@click.version_option(version="1.0.0", prog_name="sin-sbom")
@click.pass_context
def cli(ctx):
    """SIN-Code SBOM Generator — Generate SPDX and CycloneDX SBOMs.
    """
    ctx.ensure_object(dict)
    ctx.obj["generator"] = SBOMGenerator()


@cli.command()
@click.argument("input_path", type=click.Path(exists=True, dir_okay=False, path_type=Path))
@click.option("--format", "fmt", type=click.Choice(["spdx", "cyclonedx", "both"]), default="both", show_default=True, help="SBOM output format")
@click.option("--output", "-o", type=click.Path(path_type=Path), help="Output directory (default: current directory)")
@click.option("--name", default="sbom", show_default=True, help="SBOM document name")
@click.option("--summary", is_flag=True, help="Print human-readable summary")
@click.pass_context
def generate(ctx, input_path: Path, fmt: str, output: Optional[Path], name: str, summary: bool):
    """Generate SBOM from SCA scan results (JSON file)."""
    generator: SBOMGenerator = ctx.obj["generator"]

    try:
        data = json.loads(input_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        console.print(f"[red]Error: Invalid JSON in {input_path}: {e}[/red]")
        sys.exit(1)

    sbom = generator.generate_from_sca_results(data, document_name=name)

    output_dir = output or Path(".")
    output_dir.mkdir(parents=True, exist_ok=True)

    results = []
    if fmt in ("spdx", "both"):
        spdx_path = output_dir / f"{name}.spdx.json"
        spdx_json = generator.export_spdx(sbom, str(spdx_path))
        results.append(("SPDX", spdx_path, len(spdx_json)))
        console.print(f"[green]✅ SPDX SBOM written to {spdx_path}[/green]")

    if fmt in ("cyclonedx", "both"):
        cdx_path = output_dir / f"{name}.cyclonedx.json"
        cdx_json = generator.export_cyclonedx(sbom, str(cdx_path))
        results.append(("CycloneDX", cdx_path, len(cdx_json)))
        console.print(f"[green]✅ CycloneDX SBOM written to {cdx_path}[/green]")

    if summary:
        console.print(Panel(generator.export_summary(sbom), title="SBOM Summary", border_style="blue"))

    # Print stats table
    table = Table(title="SBOM Statistics", show_header=True, header_style="bold magenta")
    table.add_column("Metric", style="cyan")
    table.add_column("Value", style="green")
    table.add_row("Packages", str(sbom.total_packages))
    table.add_row("Dependencies", str(sbom.total_dependencies))
    table.add_row("Unique Licenses", str(len(sbom.unique_licenses)))
    table.add_row("Source Type", sbom.source_type or "unknown")
    console.print(table)


@cli.command()
@click.argument("name", default="sbom")
@click.option("--format", "fmt", type=click.Choice(["spdx", "cyclonedx", "both"]), default="both", show_default=True)
@click.option("--output", "-o", type=click.Path(path_type=Path), default=Path("."), help="Output directory")
@click.option("--packages", type=click.STRING, help='JSON array of packages, e.g., \'[{"name":"lodash","version":"4.17.21"}]\'')
@click.pass_context
def from_deps(ctx, name: str, fmt: str, output: Path, packages: str):
    """Generate SBOM from a raw list of dependencies (JSON string)."""
    generator: SBOMGenerator = ctx.obj["generator"]

    try:
        deps = json.loads(packages) if packages else []
    except json.JSONDecodeError as e:
        console.print(f"[red]Error: Invalid JSON in --packages: {e}[/red]")
        sys.exit(1)

    sbom = generator.generate_from_raw_dependencies(deps, document_name=name)

    output.mkdir(parents=True, exist_ok=True)

    if fmt in ("spdx", "both"):
        path = output / f"{name}.spdx.json"
        generator.export_spdx(sbom, str(path))
        console.print(f"[green]✅ SPDX SBOM written to {path}[/green]")

    if fmt in ("cyclonedx", "both"):
        path = output / f"{name}.cyclonedx.json"
        generator.export_cyclonedx(sbom, str(path))
        console.print(f"[green]✅ CycloneDX SBOM written to {path}[/green]")


@cli.command()
@click.argument("input_path", type=click.Path(exists=True, dir_okay=False, path_type=Path))
@click.pass_context
def summary(ctx, input_path: Path):
    """Print a human-readable summary of an existing SBOM JSON file."""
    try:
        data = json.loads(input_path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as e:
        console.print(f"[red]Error: Invalid JSON: {e}[/red]")
        sys.exit(1)

    # Heuristic: detect format
    if data.get("bomFormat") == "CycloneDX":
        console.print(f"[cyan]CycloneDX {data.get('specVersion', 'unknown')} SBOM[/cyan]")
        components = data.get("components", [])
        console.print(f"Components: {len(components)}")
    elif data.get("spdxVersion"):
        console.print(f"[cyan]SPDX {data.get('spdxVersion', 'unknown')} SBOM[/cyan]")
        packages = data.get("packages", [])
        console.print(f"Packages: {len(packages)}")
    else:
        console.print("[yellow]Unknown SBOM format[/yellow]")


def main():
    cli()


if __name__ == "__main__":
    main()
