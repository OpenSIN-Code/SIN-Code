# Purpose: Shared test fixtures and configuration.
# Docs: conftest.doc.md
"""Shared fixtures for pytest.
"""

import os
import tempfile

import pytest

from sin_code_bundle.tools.slash.registry import CommandRegistry
from sin_code_bundle.tools.slash.dispatcher import CommandDispatcher
from sin_code_bundle.tools.slash.executor import CommandExecutor


@pytest.fixture
def temp_registry():
    """Create a temporary CommandRegistry.

    Yields:
        CommandRegistry backed by a temporary SQLite database.
    """
    db_fd, db_path = tempfile.mkstemp(suffix=".db")
    registry = CommandRegistry(db_path)
    yield registry
    os.close(db_fd)
    os.unlink(db_path)


@pytest.fixture
def temp_dispatcher():
    """Create a temporary CommandDispatcher.

    Yields:
        CommandDispatcher with temporary registry and history DB.
    """
    reg_fd, reg_path = tempfile.mkstemp(suffix=".db")
    hist_fd, hist_path = tempfile.mkstemp(suffix=".db")
    dispatcher = CommandDispatcher(
        registry=CommandRegistry(reg_path),
        history_db=hist_path,
    )
    yield dispatcher
    os.close(reg_fd)
    os.unlink(reg_path)
    os.close(hist_fd)
    os.unlink(hist_path)


@pytest.fixture
def executor():
    """Create a CommandExecutor.

    Yields:
        CommandExecutor with default timeout.
    """
    yield CommandExecutor()


@pytest.fixture
def parser():
    """Create a SlashParser.

    Yields:
        SlashParser instance.
    """
    from sin_code_bundle.tools.slash.parser import SlashParser
    yield SlashParser()
