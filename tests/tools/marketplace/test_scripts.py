# SPDX-License-Identifier: MIT
# Purpose: Test bash scripts exist and are executable
# Docs: test_scripts.py.doc.md
"""Tests for bash scripts."""

from pathlib import Path

import pytest


# ── Script tests ──────────────────────────────────────────────────────────────
class TestScripts:
    def test_all_scripts_exist(self) -> None:
        scripts_dir = Path(__file__).parent.parent.parent.parent / "scripts"
        expected = [
            "marketplace-search.sh",
            "marketplace-install.sh",
            "marketplace-list.sh",
            "marketplace-remove.sh",
            "marketplace-update.sh",
            "marketplace-sync.sh",
        ]
        for script in expected:
            path = scripts_dir / script
            assert path.exists(), f"{script} does not exist"
            assert path.stat().st_mode & 0o111, f"{script} is not executable"

    def test_search_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-search.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_install_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-install.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_list_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-list.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_remove_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-remove.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_update_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-update.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_sync_script_has_shebang(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-sync.sh"
        content = path.read_text()
        assert "#!/usr/bin/env bash" in content

    def test_search_script_usage(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-search.sh"
        content = path.read_text()
        assert "Usage:" in content

    def test_install_script_usage(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-install.sh"
        content = path.read_text()
        assert "Usage:" in content

    def test_remove_script_usage(self) -> None:
        path = Path(__file__).parent.parent.parent.parent / "scripts" / "marketplace-remove.sh"
        content = path.read_text()
        assert "Usage:" in content
