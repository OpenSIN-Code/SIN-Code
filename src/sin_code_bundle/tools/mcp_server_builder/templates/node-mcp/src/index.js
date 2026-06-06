/**
 * Purpose: {{ name }} MCP server entry point.
 * Docs: index.doc.md
 */
import { Server } from "@modelcontextprotocol/sdk/server/index.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";

const server = new Server(
  {
    name: "sin-{{ slug }}",
    version: "{{ version }}",
  },
  {
    capabilities: {
      tools: {},
    },
  }
);

{% for tool in tools %}
server.setRequestHandler("tools/{{ tool }}", async () => {
  return {
    content: [{ type: "text", text: JSON.stringify({ tool: "{{ tool }}", ok: true }) }],
  };
});
{% endfor %}

const transport = new StdioServerTransport();
await server.connect(transport);
