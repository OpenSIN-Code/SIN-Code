# SPDX-License-Identifier: MIT
"""Compatibility alias for ``sin_code_bundle.tools.marketplace.installer``."""

import sys as _sys
from importlib import import_module as _import_module

_module = _import_module("sin_code_bundle.tools.marketplace.installer")
_sys.modules[__name__] = _module
