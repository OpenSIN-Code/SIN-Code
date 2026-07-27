# SPDX-License-Identifier: MIT
"""Compatibility alias for the former ``sin_mcp_server_builder.cli_shims.mcp_tool_add``."""
import sys as _sys
from importlib import import_module as _import_module

_module = _import_module("sin_code_bundle.tools.mcp_server_builder.cli_shims.mcp_tool_add")
_sys.modules[__name__] = _module
