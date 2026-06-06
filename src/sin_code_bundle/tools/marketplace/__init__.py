# Purpose: Package init for sin_marketplace
# Docs: __init__.py.doc.md
"""SIN Marketplace — manage OpenSIN-Code skills via MCP and CLI."""

__version__ = "0.1.0"

from .catalog import Catalog
from .installer import Installer
from .registry import Registry
from .updater import Updater

__all__ = ["Catalog", "Installer", "Registry", "Updater"]
