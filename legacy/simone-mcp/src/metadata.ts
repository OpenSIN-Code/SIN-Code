import process from 'node:process';

export const TEMPLATE_AGENT_ID = 'sin-simone-mcp';
export const TEMPLATE_AGENT_NAMESPACE = 'simone.mcp';
export const TEMPLATE_AGENT_NAME = 'SIN-Simone-MCP';
export const TEMPLATE_AGENT_DESCRIPTION = 'Ultra-Duo Team Coder Agent with LSP-powered semantic code analysis, A2A-native async runtime, and cloud semantic memory.';
export const TEMPLATE_AGENT_VERSION = '2026.03.24';
export const TEMPLATE_AGENT_DEFAULT_HOST = '127.0.0.1';
export const TEMPLATE_AGENT_DEFAULT_PORT = 8234;

export const TEMPLATE_AGENT_SKILLS = [
  {
    id: 'sin.simone.mcp.health',
    name: 'Health',
    description: 'Check Simone MCP readiness, model, and capabilities.',
  },
  {
    id: 'sin.simone.mcp.symbol.search',
    name: 'Symbol Search',
    description: 'Search for symbols across the codebase using LSP-powered semantic analysis.',
  },
  {
    id: 'sin.simone.mcp.structural.edit',
    name: 'Structural Edit',
    description: 'Perform structural code edits using LSP-grade symbol resolution and refactoring.',
  },
  {
    id: 'sin.simone.mcp.memory.query',
    name: 'Memory Query',
    description: 'Query the cloud semantic memory (pgvector) for code context and prior analysis results.',
  },
] as const;

export function resolveTemplateAgentConfig() {
  const host =
    process.env.SIN_SIMONE_MCP_HOST?.trim() ||
    (process.env.PORT ? '0.0.0.0' : process.env.HOST?.trim() || TEMPLATE_AGENT_DEFAULT_HOST);
  const port = parseInteger(process.env.SIN_SIMONE_MCP_PORT, parseInteger(process.env.PORT, TEMPLATE_AGENT_DEFAULT_PORT));
  const fallbackPublicHost = host === '0.0.0.0' ? '127.0.0.1' : host;
  const publicBaseUrl =
    process.env.SIN_SIMONE_MCP_PUBLIC_BASE_URL?.trim() ||
    (process.env.SPACE_HOST?.trim() ? `https://${process.env.SPACE_HOST.trim()}` : `http://${fallbackPublicHost}:${port}`);
  return { host, port, publicBaseUrl: publicBaseUrl.replace(/\/+$/, '') };
}

export function buildAgentCard(baseUrl: string) {
  const normalizedBaseUrl = baseUrl.replace(/\/+$/, '');
  const rpcUrl = `${normalizedBaseUrl}/a2a/v1`;
  return {
    name: TEMPLATE_AGENT_NAME,
    description: TEMPLATE_AGENT_DESCRIPTION,
    version: TEMPLATE_AGENT_VERSION,
    documentationUrl: normalizedBaseUrl,
    url: rpcUrl,
    capabilities: { streaming: false, pushNotifications: false },
    defaultInputModes: ['text/plain', 'application/json'],
    defaultOutputModes: ['text/plain', 'application/json'],
    skills: [...TEMPLATE_AGENT_SKILLS],
    supportedInterfaces: [{ url: rpcUrl, protocolBinding: 'JSONRPC', protocolVersion: '1.0' }],
  };
}

function parseInteger(input: string | undefined, fallback: number) {
  const parsed = Number.parseInt(String(input || '').trim(), 10);
  return Number.isFinite(parsed) ? parsed : fallback;
}
