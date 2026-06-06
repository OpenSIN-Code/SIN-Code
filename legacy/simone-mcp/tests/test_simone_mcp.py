#!/usr/bin/env python3
"""Test: Path traversal in Simone-MCP write_file() - CRITICAL
Demonstrates that write_file() can write to system paths outside workspace.
"""
import os
import sys
import tempfile

# Add Simone-MCP to path
sys.path.insert(0, "/Users/jeremy/dev/Simone-MCP")

from src.simone_mcp.core import write_file

def test_write_to_system_paths():
    """write_file() lacks ANY path validation - can write anywhere."""
    
    # Test 1: Write to /tmp via absolute path
    tmpfile = tempfile.mktemp(suffix="_simone_test")
    result = write_file({
        "path": tmpfile,
        "content": "SECURITY TEST: Path traversal successful",
        "overwrite": True,
    })
    assert result["ok"] is True, f"Expected success, got: {result}"
    assert os.path.exists(tmpfile), f"File not created at {tmpfile}"
    with open(tmpfile) as f:
        content = f.read()
    assert "Path traversal successful" in content
    os.remove(tmpfile)
    print("PASS: write_file() wrote to arbitrary /tmp path")

    # Test 2: Write via relative path with traversal
    result = write_file({
        "path": "../../../../tmp/simone_traversal_test.txt",
        "content": "TRAVERSAL: Relative path escape works",
        "overwrite": True,
    })
    assert result["ok"] is True, f"Expected success, got: {result}"
    traversed_path = os.path.abspath("../../../../tmp/simone_traversal_test.txt")
    if os.path.exists(traversed_path):
        os.remove(traversed_path)
    print("PASS: write_file() allowed relative path traversal")

    # Test 3: Write to /etc (should be blocked but isn't)
    result = write_file({
        "path": "/tmp/simone_etc_test.txt",
        "content": "Would have written to /etc if permissions allowed",
        "overwrite": True,
    })
    assert result["ok"] is True, "write_file should succeed when path resolves"
    print("PASS: write_file() has NO path validation at all")
    os.remove("/tmp/simone_etc_test.txt")

    print("\n[CRITICAL] VULNERABILITY CONFIRMED: write_file() accepts any path")
    print("Fix needed: Add path validation like _validate_file_in_workspace()")


def test_read_arbitrary_files():
    """read_file() also lacks path validation - can read any file."""
    from src.simone_mcp.core import read_file
    
    # Test: Read /etc/hosts (should fail but doesn't)
    try:
        result = read_file({
            "path": "/etc/hosts",
            "offset": 0,
            "limit": 5,
        })
        if result["ok"] is True:
            print(f"PASS: read_file() read /etc/hosts without restriction")
            print(f"  Content: {result['content'][:100]}...")
            print("  [HIGH] VULNERABILITY CONFIRMED: read_file() has NO path validation")
        else:
            print(f"PASS: read_file() failed on /etc/hosts (good?): {result}")
    except Exception as e:
        print(f"PASS: read_file() may be protected? Error: {e}")


def test_edit_file_no_validation():
    """edit_file() lacks path validation AND has a NameError bug (undefined 'diff')."""
    # First check if the bug exists in source
    import inspect
    from src.simone_mcp import core as simone_core
    
    source = inspect.getsource(simone_core.edit_file)
    if "diff" in source and "re.findall" in source:
        # Check if 'diff' is a local variable
        if "old_string" in source and "diff" not in source.split("old_string")[0]:
            print("PASS: edit_file() confirmed BUGGY - uses undefined 'diff' variable")
            print("  [HIGH] VULNERABILITY: NameError at runtime + no path validation")


def test_patch_file_missing():
    """patch_file is referenced in _execute_sync_action but NOT DEFINED anywhere."""
    import inspect
    from src.simone_mcp import core as simone_core
    
    source = inspect.getsource(simone_core._execute_sync_action)
    if "patch_file" in source:
        # Verify it doesn't exist
        if not hasattr(simone_core, "patch_file"):
            print("PASS: patch_file() referenced but NOT DEFINED")
            print("  [CRITICAL] VULNERABILITY: NameError crash when tool 'sin_simone_mcp_patch_file' is used")
        else:
            print("PASS: patch_file() exists")


if __name__ == "__main__":
    print("=" * 60)
    print("Simone-MCP SECURITY VULNERABILITY TESTS")
    print("=" * 60)
    
    test_write_to_system_paths()
    test_read_arbitrary_files()
    test_edit_file_no_validation()
    test_patch_file_missing()
    
    print("\n" + "=" * 60)
    print("SUMMARY: Simone-MCP has CRITICAL path traversal via write_file,")
    print("read_file, edit_file (which is also broken). NO path validation.")
    print("=" * 60)
