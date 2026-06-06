# Purpose: Shared fixtures and configuration for test suite
# Docs: conftest.py.doc.md
"""Shared pytest configuration for sin-marketplace tests."""

import pytest


@pytest.fixture(autouse=True)
def _suppress_logging():
    """Suppress logging during tests to reduce noise."""
    import logging
    logging.disable(logging.CRITICAL)
    yield
    logging.disable(logging.NOTSET)
