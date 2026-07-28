# SPDX-License-Identifier: MIT
"""Compatibility alias for ``sin_code_bundle.tools.mcp_server_builder.auditor``."""

import sys as _sys
from importlib import import_module as _import_module

_module = _import_module("sin_code_bundle.tools.mcp_server_builder.auditor")
_sys.modules[__name__] = _module
