import { randomUUID } from 'node:crypto';
import { createServer, type IncomingMessage, type ServerResponse } from 'node:http';
import { buildAgentCard, resolveTemplateAgentConfig, TEMPLATE_AGENT_ID, TEMPLATE_AGENT_NAME } from './metadata.js';
import { executeSimoneMcpAgentAction, type SimoneMcpAgentAction } from './runtime.js';

type RpcRequest = { jsonrpc?: string; id?: string | number | null; method?: string; params?: Record<string, unknown> };

export function createTemplateAgentHttpServer() {
  const config = resolveTemplateAgentConfig();
  const server = createServer((request, response) => void handleRequest(request, response, config.publicBaseUrl));
  return {
    server,
    async start() {
      await new Promise<void>((resolve, reject) => {
        server.once('error', reject);
        server.listen(config.port, config.host, () => resolve());
      });
    },
    async stop() {
      await new Promise<void>((resolve, reject) => server.close((error) => (error ? reject(error) : resolve())));
    },
  };
}

async function handleRequest(request: IncomingMessage, response: ServerResponse, baseUrl: string) {
  if (request.method === 'GET' && request.url === '/health') {
    return sendJson(response, 200, { ok: true, agent: TEMPLATE_AGENT_ID });
  }
  if (request.method === 'GET' && request.url === '/') {
    return sendHtml(response, 200, `<html><body><h1>${TEMPLATE_AGENT_NAME}</h1><p>Ultra-Duo Team Coder — LSP-powered semantic code analysis.</p></body></html>`);
  }
  if (request.method === 'GET' && (request.url === '/.well-known/agent-card.json' || request.url === '/.well-known/agent.json')) {
    return sendJson(response, 200, buildAgentCard(baseUrl));
  }
  if (request.method === 'POST' && request.url === '/a2a/v1') {
    const rpc = ((await readJson(request)) || {}) as RpcRequest;
    if (rpc.method === 'agent/getCard') {
      return sendJson(response, 200, { jsonrpc: '2.0', id: rpc.id ?? null, result: buildAgentCard(baseUrl) });
    }
    if (rpc.method === 'message/send') {
      const text = ((rpc.params?.message as { parts?: Array<{ text?: string }> } | undefined)?.parts || [])
        .map((part) => part.text || '')
        .join(' ')
        .trim();
      const action = parseAction(text);
      const result = await executeSimoneMcpAgentAction(action);
      return sendJson(response, 200, {
        jsonrpc: '2.0',
        id: rpc.id ?? null,
        result: {
          id: randomUUID(),
          kind: 'task',
          status: { state: 'completed', timestamp: new Date().toISOString(), message: { role: 'agent', parts: [{ type: 'text', text: 'done' }] } },
          artifacts: [{ id: randomUUID(), name: action.action, description: action.action, parts: [{ type: 'data', data: result }] }],
          metadata: { action: action.action },
        },
      });
    }
  }
  sendJson(response, 404, { error: 'not_found' });
}

function parseAction(text: string): SimoneMcpAgentAction {
  try {
    const parsed = JSON.parse(text);
    if (parsed && typeof parsed === 'object' && typeof parsed.action === 'string') return parsed as SimoneMcpAgentAction;
  } catch { /* fall through to text matching */ }

  const value = text.toLowerCase().trim();
  if (value.includes('health')) return { action: 'sin.simone.mcp.health' };
  if (value.includes('symbol') || value.includes('search')) return { action: 'sin.simone.mcp.symbol.search', query: text };
  if (value.includes('edit') || value.includes('refactor')) return { action: 'sin.simone.mcp.structural.edit', editPayload: text };
  if (value.includes('memory') || value.includes('context')) return { action: 'sin.simone.mcp.memory.query', query: text };
  return { action: 'agent.help' };
}

async function readJson(request: IncomingMessage) {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.isBuffer(chunk) ? chunk : Buffer.from(chunk));
  const raw = Buffer.concat(chunks).toString('utf8').trim();
  return raw ? JSON.parse(raw) : null;
}

function sendJson(response: ServerResponse, statusCode: number, payload: unknown) {
  response.statusCode = statusCode;
  response.setHeader('content-type', 'application/json; charset=utf-8');
  response.end(JSON.stringify(payload, null, 2));
}

function sendHtml(response: ServerResponse, statusCode: number, payload: string) {
  response.statusCode = statusCode;
  response.setHeader('content-type', 'text/html; charset=utf-8');
  response.end(payload);
}
