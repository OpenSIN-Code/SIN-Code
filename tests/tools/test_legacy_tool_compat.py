# SPDX-License-Identifier: MIT
"""Compatibility checks for the three repositories consolidated by issue #29."""


def test_slash_legacy_namespace() -> None:
    from sin_slash.parser import SlashParser
    from sin_slash.registry import CommandRegistry

    assert SlashParser is not None
    assert CommandRegistry is not None


def test_mcp_builder_legacy_namespace() -> None:
    import sin_mcp_server_builder as legacy
    from sin_mcp_server_builder.cli_shims.mcp_scaffold import main as scaffold_main
    from sin_mcp_server_builder.scaffolder import Scaffolder

    assert legacy.__version__ == "0.2.0"
    assert Scaffolder is not None
    assert scaffold_main is not None


def test_marketplace_legacy_namespace() -> None:
    from sin_marketplace import Catalog, Installer, Registry, Updater
    from sin_marketplace.cli import main

    assert all(item is not None for item in (Catalog, Installer, Registry, Updater, main))
