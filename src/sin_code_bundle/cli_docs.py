# SPDX-License-Identifier: MIT
"""Docs sub-commands — extracted from cli.py."""
from __future__ import annotations

import json
from pathlib import Path

import typer

from sin_code_bundle.cli_app import docs_app


@docs_app.command("generate")
def docs_generate(
    path: str = typer.Argument(".", help="Project path."),
    output: str = typer.Option("README.md", help="Output file name."),
    template: str = typer.Option("default", help="Template: default, minimal, full."),
):
    """Generate a README.md from project metadata."""
    proj = Path(path)
    if not proj.exists():
        typer.echo(f"[SIN-BUNDLE] Path not found: {path}", err=True)
        raise typer.Exit(code=1)

    # Gather metadata
    name = proj.resolve().name
    pyproject = proj / "pyproject.toml"
    package_json = proj / "package.json"
    go_mod = proj / "go.mod"

    language = "unknown"
    version = "0.1.0"
    description = f"{name} project"
    dependencies = []

    if pyproject.exists():
        language = "Python"
        content = pyproject.read_text()
        for line in content.splitlines():
            if line.startswith("name"):
                name = line.split("=")[1].strip().strip('"').strip("'")
            if line.startswith("version"):
                version = line.split("=")[1].strip().strip('"').strip("'")
            if line.startswith("description"):
                description = line.split("=")[1].strip().strip('"').strip("'")
    elif package_json.exists():
        language = "JavaScript/TypeScript"
        data = json.loads(package_json.read_text())
        name = data.get("name", name)
        version = data.get("version", "0.1.0")
        description = data.get("description", description)
        dependencies = list(data.get("dependencies", {}).keys())
    elif go_mod.exists():
        language = "Go"
        content = go_mod.read_text()
        for line in content.splitlines():
            if line.startswith("module "):
                name = line.split()[1]

    readme = f"""# {name}

{description}

## Overview

- **Language**: {language}
- **Version**: {version}
- **Path**: `{proj.resolve()}`

## Installation

```bash
# Clone the repository
git clone <repository-url>
cd {name}

# Install dependencies
"""

    if language == "Python":
        readme += "pip install -e .\n"
    elif language == "JavaScript/TypeScript":
        readme += "npm install\n"
    elif language == "Go":
        readme += "go mod tidy\n"
    else:
        readme += "# See project documentation\n"

    readme += """```

## Usage

```bash
# Run the project
# (Add usage examples here)
```

## Testing

```bash
# Run tests
"""

    if language == "Python":
        readme += "pytest\n"
    elif language == "JavaScript/TypeScript":
        readme += "npm test\n"
    elif language == "Go":
        readme += "go test ./...\n"
    else:
        readme += "# See project documentation\n"

    readme += """```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Open a Pull Request

## License

MIT - OpenSIN-Code Project
"""

    if dependencies:
        readme += "\n## Dependencies\n\n"
        for dep in dependencies[:10]:
            readme += f"- {dep}\n"

    output_path = proj / output
    output_path.write_text(readme, encoding="utf-8")
    typer.echo(f"[SIN-BUNDLE] Generated {output_path}")


@docs_app.command("check")
def docs_check(
    path: str = typer.Argument(".", help="Project path."),
):
    """Check documentation coverage (README, docstrings, .doc.md files)."""
    proj = Path(path)
    if not proj.exists():
        typer.echo(f"[SIN-BUNDLE] Path not found: {path}", err=True)
        raise typer.Exit(code=1)

    readme = proj / "README.md"
    has_readme = readme.exists()

    doc_md_files = list(proj.rglob("*.doc.md"))
    py_files = list(proj.rglob("*.py"))
    docstring_count = 0

    for py_file in py_files:
        content = py_file.read_text()
        if '"""' in content or "'''" in content:
            docstring_count += 1

    typer.echo(f"[SIN-BUNDLE] Documentation Coverage Report for {proj.resolve()}")
    typer.echo(f"  README.md: {'✅' if has_readme else '❌'}")
    typer.echo(f"  .doc.md files: {len(doc_md_files)}")
    typer.echo(f"  Python files: {len(py_files)}")
    typer.echo(
        f"  Files with docstrings: {docstring_count}/{len(py_files)} ({100 * docstring_count // max(len(py_files), 1)}%)"
    )

    if not has_readme:
        typer.echo("  ⚠️  Missing README.md — run `sin docs generate` to create one.")
