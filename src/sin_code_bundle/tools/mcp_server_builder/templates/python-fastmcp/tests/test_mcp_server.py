"""
Purpose: Smoke tests for the {{ name }} MCP server tools.
Docs: test_mcp_server.doc.md
"""
import json

import pytest

from {{ pkg }}.mcp_server import {% for tool in tools %}{{ tool }}{% if not loop.last %}, {% endif %}{% endfor %}


{% for tool in tools %}
class Test{{ tool | capitalize }}:
    def test_{{ tool }}_returns_dict(self):
        """`{{ tool }}` must return a JSON object."""
        result = {{ tool }}()
        data = json.loads(result)
        assert isinstance(data, dict)
        assert data.get("ok") is True

    def test_{{ tool }}_has_tool_field(self):
        """`{{ tool }}` should include its tool name in the result."""
        result = {{ tool }}()
        data = json.loads(result)
        assert data.get("tool") == "{{ tool }}"
{% endfor %}
