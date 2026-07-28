# SPDX-License-Identifier: MIT
"""Security sub-commands — extracted from cli.py."""

from __future__ import annotations

import shutil
import subprocess
import sys

import typer

from sin_code_bundle.cli_app import security_app


def _forward_security_subcommand(subcommand: str) -> None:
    """Forward `subcommand <rest of argv>` to the `sin-security` Go binary.

    Unlike `_forward_to_binary`, this helper is meant to be called from a Typer
    subcommand (e.g. `sin security secrets /path`). It extracts everything in
    sys.argv after the first occurrence of `subcommand` and prepends it to the
    binary invocation, so the binary sees the same shape it would if invoked
    directly (e.g. `sin-security secrets /path`).
    """
    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index(subcommand) + 1 :] if subcommand in sys.argv else []
    result = subprocess.run([binary, subcommand, *args])
    raise typer.Exit(code=result.returncode)


@security_app.command("secrets")
def security_secrets(
    path: str = typer.Argument(".", help="Path to scan for hardcoded secrets"),
):
    """Secret scanning (regex + Shannon entropy)."""
    _forward_security_subcommand("secrets")


@security_app.command("sast")
def security_sast(
    path: str = typer.Argument(".", help="Path to scan with static analysis"),
):
    """Static application security testing (SAST)."""
    _forward_security_subcommand("sast")


@security_app.command("sca")
def security_sca(
    path: str = typer.Argument(".", help="Path to scan for vulnerable dependencies"),
):
    """Software composition analysis (SCA) — deps + CVEs."""
    _forward_security_subcommand("sca")


@security_app.command("sbom")
def security_sbom(
    path: str = typer.Argument(".", help="Path to generate SBOM for"),
):
    """Generate SBOM in SPDX and CycloneDX formats."""
    _forward_security_subcommand("sbom")


@security_app.command("container")
def security_container(
    path: str = typer.Argument(".", help="Path (image name or Dockerfile) to scan"),
):
    """Container image security scan."""
    _forward_security_subcommand("container")


@security_app.command("iac")
def security_iac(
    path: str = typer.Argument(".", help="Path to IaC files (Terraform, etc.)"),
):
    """Infrastructure-as-Code security scan (Terraform, CloudFormation, K8s)."""
    _forward_security_subcommand("iac")


@security_app.command("license")
def security_license(
    path: str = typer.Argument(".", help="Path to scan for license compliance"),
):
    """License compliance check across all dependencies."""
    _forward_security_subcommand("license")


@security_app.command("dast")
def security_dast(
    path: str = typer.Argument(".", help="Path or target URL for DAST scan"),
):
    """Dynamic application security testing (DAST)."""
    _forward_security_subcommand("dast")


@security_app.command("full")
def security_full(
    path: str = typer.Argument(".", help="Path to run all 8 security tools against"),
):
    """Run all 8 security tools in sequence and emit a combined report.

    Delegates to the binary's built-in `scan` subcommand, which orchestrates
    secrets, sast, sca, sbom, container, iac, license, and dast. Use
    `--compliance` to scope to a framework (e.g. cis,nist,soc2,iso27001,gdpr,
    hipaa,pci,owasp) and `--skip-tools` to exclude specific tools.
    """
    binary = shutil.which("sin-security")
    if not binary:
        typer.echo(
            "[SIN-BUNDLE] 'sin-security' binary not found in PATH. "
            "Build it from ~/SIN-Code-Security-Bundle:\n"
            "  go build -o ~/.local/bin/sin-security ./cmd/sin-security",
            err=True,
        )
        raise typer.Exit(code=1)
    args = sys.argv[sys.argv.index("full") + 1 :] if "full" in sys.argv else []
    result = subprocess.run([binary, "scan", *args])
    raise typer.Exit(code=result.returncode)
