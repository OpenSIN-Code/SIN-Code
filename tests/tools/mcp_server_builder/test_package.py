# SPDX-License-Identifier: MIT
"""
Purpose: Test the top-level package API and version (mcp_server_builder).
Docs: test_package.doc.md
"""

import sin_code_bundle.tools.mcp_server_builder as pkg
from sin_code_bundle.tools.mcp_server_builder import __all__, __version__


class TestPackageExports:
    def test_all_exports_resolve(self):
        for name in __all__:
            obj = getattr(pkg, name)
            assert obj is not None

    def test_version_is_string(self):
        assert isinstance(__version__, str)
        assert __version__ == "0.1.0"

    def test_auditor_exported(self):
        assert hasattr(pkg, "Auditor")

    def test_each_module_importable(self):
        from sin_code_bundle.tools.mcp_server_builder import (
            auditor,
            publisher,
            registrar,
            scaffolder,
            templates,
            test_gen,
            tool_adder,
            validator,
        )

        for mod in [
            auditor,
            publisher,
            registrar,
            scaffolder,
            templates,
            test_gen,
            tool_adder,
            validator,
        ]:
            assert mod.__file__ is not None
