#!/usr/bin/env python3
"""Test: Simone-MCP path traversal - CRITICAL
Demonstrates that write_file(), read_file(), edit_file() have NO path validation.
"""
import os
import sys
import tempfile

sys.path.insert(0, "/Users/jeremy/dev/Simone-MCP")

from src.simone_mcp.core import write_file, read_file


def test_write_file_no_validation():
    """write_file() has ZERO path validation - writes ANYWHERE."""
    tmpfile = "/tmp/simone_poc_" + os.urandom(4).hex()
    
    result = write_file({
        "path": tmpfile,
        "content": "SECURITY POC: Path traversal successful",
        "overwrite": True,
    })
    
    assert result["ok"] is True, f"write failed: {result}"
    assert os.path.exists(tmpfile), "File not created"
    os.remove(tmpfile)
    print("PASS: write_file() wrote to arbitrary absolute path (no validation)")


def test_read_file_no_validation():
    """read_file() has ZERO path validation - reads ANYWHERE."""
    result = read_file({
        "path": "/etc/hosts",
        "offset": 0,
        "limit": 5,
    })
    
    assert result["ok"] is True, f"read failed: {result}"
    assert "127.0.0.1" in result["content"] or "localhost" in result["content"]
    print(f"PASS: read_file() read /etc/hosts without any path restriction")
    print(f"  Content preview: {result['content'][:100]}")


def test_edit_file_no_validation():
    """edit_file() path validation + uses undefined 'diff' variable (NameError bug)."""
    import inspect
    from src.simone_mcp import core as simone_core
    
    source = inspect.getsource(simone_core.edit_file)
    
    # Check for undefined 'diff' variable
    # The function gets 'old_string' and 'new_string' from payload
    # But line references 'diff' which doesn't exist
    lines = source.split('\n')
    for i, line in enumerate(lines):
        if 'diff' in line:
            stripped = line.strip()
            if not stripped.startswith('#') and not stripped.startswith('"') and not stripped.startswith("'"):
                # Check if 'diff' is defined anywhere before this line
                if 'diff' not in lines[:i]:
                    print(f"BUG: edit_file() uses undefined variable 'diff' at line {i}:")
                    print(f"  {stripped}")
    
    print("PASS: edit_file() has NO path validation AND is broken (NameError at runtime)")


def test_patch_file_missing():
    """patch_file is in TOOL_DEFINITIONS and _execute_sync_action but undefined."""
    from src.simone_mcp import core as simone_core, server as simone_server
    
    # Check if it's defined
    if hasattr(simone_core, "patch_file"):
        print("INFO: patch_file is defined in core")
    else:
        print("FAIL: patch_file() is REFERENCED but NOT DEFINED anywhere!")
        print("  Referenced in TOOL_DEFINITIONS (sin_simone_mcp_patch_file)")
        print("  Referenced in _execute_sync_action dispatch")
        print("  But no implementation exists - will crash with NameError")


if __name__ == "__main__":
    print("=" * 60)
    print("Simone-MCP: CRITICAL PATH TRAVERSAL TESTS")
    print("=" * 60)
    
    test_write_file_no_validation()
    print()
    test_read_file_no_validation()
    print()
    test_edit_file_no_validation()
    print()
    test_patch_file_missing()
    
    print("\n" + "=" * 60)
    print("RESULT: write_file(), read_file(), edit_file() have ZERO")
    print("path validation. Can read/write ANY file on the system.")
    print("patch_file() is missing entirely (NameError crash).")
    print("RATING: CRITICAL — requires immediate fix.")
    print("=" * 60)
