from __future__ import annotations

from pathlib import Path

import pytest


@pytest.fixture(autouse=True)
def allow_temporary_test_workspace(
    tmp_path: Path,
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    """Permit each test's isolated temporary directory as a workspace root."""
    monkeypatch.setenv("SIMONE_WORKSPACE_ROOTS", str(tmp_path))
