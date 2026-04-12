import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import { CallToolRequestSchema, ListToolsRequestSchema } from '@modelcontextprotocol/sdk/types.js';
import { executeSimoneMcpAgentAction, type SimoneMcpAgentAction } from './runtime.js';

const server = new Server(
  { name: 'sin-simone-mcp', version: '2026.03.24' },
  { capabilities: { tools: {} } },
);

server.setRequestHandler(ListToolsRequestSchema, async () => {
  return {
    tools: [
      {
        name: 'sin_simone_mcp_health',
        description: 'Check Simone MCP readiness, model, and capabilities.',
        inputSchema: { type: 'object' as const, properties: {} },
      },
      {
        name: 'sin_simone_mcp_symbol_search',
        description: 'Search for symbols across the codebase using LSP-powered semantic analysis.',
        inputSchema: {
          type: 'object' as const,
          properties: {
            query: { type: 'string', description: 'Symbol search query' },
          },
          required: ['query'],
        },
      },
      {
        name: 'sin_simone_mcp_structural_edit',
        description: 'Perform structural code edits using LSP-grade symbol resolution and refactoring.',
        inputSchema: {
          type: 'object' as const,
          properties: {
            editPayload: { type: 'string', description: 'Structural edit payload in JSON or natural language' },
          },
          required: ['editPayload'],
        },
      },
      {
        name: 'sin_simone_mcp_memory_query',
        description: 'Query the cloud semantic memory for code context and prior analysis results.',
        inputSchema: {
          type: 'object' as const,
          properties: {
            query: { type: 'string', description: 'Semantic memory query' },
          },
          required: ['query'],
        },
      },
    ],
  };
});

server.setRequestHandler(CallToolRequestSchema, async (request) => {
  try {
    let action: SimoneMcpAgentAction;
    const args = request.params.arguments as any;

    switch (request.params.name) {
      case 'sin_simone_mcp_health':
        action = { action: 'sin.simone.mcp.health' };
        break;
      case 'sin_simone_mcp_symbol_search':
        action = { action: 'sin.simone.mcp.symbol.search', query: args.query };
        break;
      case 'sin_simone_mcp_structural_edit':
        action = { action: 'sin.simone.mcp.structural.edit', editPayload: args.editPayload };
        break;
      case 'sin_simone_mcp_memory_query':
        action = { action: 'sin.simone.mcp.memory.query', query: args.query };
        break;
      default:
        throw new Error(`Unknown tool: ${request.params.name}`);
    }

    const result = await executeSimoneMcpAgentAction(action);
    return {
      content: [{ type: 'text', text: JSON.stringify(result, null, 2) }],
    };
  } catch (error: any) {
    return {
      content: [{ type: 'text', text: `Error: ${error.message}` }],
      isError: true,
    };
  }
});

export async function startMcpServer(): Promise<void> {
  const transport = new StdioServerTransport();
  await server.connect(transport);
  console.error('A2A-SIN-Simone-MCP MCP Server running on stdio');
}
