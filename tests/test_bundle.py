# SPDX-License-Identifier: MIT
from typer.testing import CliRunner

from sin_code_bundle.cli import app

runner = CliRunner()


def test_status_runs():
    result = runner.invoke(app, ["status"])
    assert result.exit_code == 0
    assert "Oracle" in result.stdout


def test_bootstrap_runs(tmp_path):
    result = runner.invoke(app, ["bootstrap", str(tmp_path)])
    assert result.exit_code == 0
    assert (tmp_path / ".sin").exists()
