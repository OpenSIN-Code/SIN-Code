# Purpose: Test package init and version (marketplace)
# Docs: test_init.py.doc.md
"""Tests for sin_code_bundle.tools.marketplace.__init__."""

from sin_code_bundle.tools.marketplace import __version__, Catalog, Installer, Registry, Updater


def test_version() -> None:
    assert __version__ == "0.1.0"


def test_exports() -> None:
    assert Catalog is not None
    assert Installer is not None
    assert Registry is not None
    assert Updater is not None
