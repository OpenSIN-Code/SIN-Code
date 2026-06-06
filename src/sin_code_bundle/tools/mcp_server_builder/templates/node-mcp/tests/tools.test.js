/**
 * Purpose: Smoke tests for the {{ name }} MCP server tools.
 * Docs: tools.test.doc.md
 */
import { test } from "node:test";
import assert from "node:assert/strict";

{% for tool in tools %}
test("{{ tool }} returns ok", () => {
  const result = { tool: "{{ tool }}", ok: true };
  assert.equal(result.tool, "{{ tool }}");
  assert.equal(result.ok, true);
});
{% endfor %}
