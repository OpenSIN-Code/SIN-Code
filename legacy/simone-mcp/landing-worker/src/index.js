const AGENT = {
  name: 'simone-mcp',
  displayName: 'Simone MCP',
  description: 'Ultra-Duo Team Coder Agent with LSP-powered semantic code analysis.',
  version: '2026.03.24',
  team: 'team-coding',
  runtime: 'FastAPI + Supabase Realtime + PostgreSQL/pgvector',
  endpoints: {
    health: '/agents/simone-mcp/health',
    a2a: '/agents/simone-mcp/a2a/v1',
    card: '/agents/simone-mcp/.well-known/agent-card.json',
    dashboard: '/agents/simone-mcp/',
  },
  capabilities: [
    'code.find_symbol',
    'code.find_references',
    'code.insert_after_symbol',
    'code.replace_symbol_body',
    'code.get_project_overview',
    'code.semantic_search',
  ],
};

function htmlPage() {
  return `<!doctype html>
<html lang="de">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Simone MCP</title>
  <meta name="robots" content="index,follow" />
  <style>
    body{font-family:system-ui,sans-serif;background:#0f172a;color:#f8fafc;margin:0;padding:2rem}
    .card{max-width:960px;margin:0 auto;background:#111827;border:1px solid #334155;border-radius:16px;padding:2rem}
    .pill{display:inline-block;padding:.35rem .75rem;border-radius:999px;background:#2563eb;margin-right:.5rem}
    .grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:1rem;margin-top:1rem}
    .box{background:#1f2937;border-radius:12px;padding:1rem;border-left:4px solid #3b82f6}
    code{color:#a7f3d0}
    a{color:#93c5fd}
  </style>
</head>
<body>
  <main class="card">
    <h1>Simone MCP</h1>
    <p><span class="pill">A2A-Native</span><span class="pill">Async-First</span><span class="pill">LSP-Powered</span></p>
    <p>Serena-Grade Symbol Intelligence, aber mit A2A Discovery, Cloud Memory und einem echten Operator-Dashboard.</p>
    <div class="grid">
      <section class="box"><h3>Why Simone?</h3><p>Same LSP power as Serena, but better runtime, better memory, better fleet integration.</p></section>
      <section class="box"><h3>Commands</h3><p><code>activate_simone</code><br><code>activate_simone serve-mcp</code><br><code>activate_simone print-card</code></p></section>
      <section class="box"><h3>Endpoints</h3><p><code>/health</code><br><code>/.well-known/agent-card.json</code><br><code>/a2a/v1</code></p></section>
      <section class="box"><h3>Public Card</h3><p><a href="/.well-known/agent-card.json">agent-card.json</a></p></section>
    </div>
  </main>
</body>
</html>`;
}

export default {
  async fetch(request) {
    const url = new URL(request.url);
    if (url.pathname === '/agents/simone-mcp' || url.pathname === '/agents/simone-mcp/') {
      return new Response(htmlPage(), { headers: { 'content-type': 'text/html; charset=utf-8' } });
    }
    if (url.pathname === '/agents/simone-mcp/health') {
      return Response.json({ status: 'ok', service: 'simone-mcp', deployment: 'cloudflare-route' });
    }
    if (url.pathname === '/agents/simone-mcp/.well-known/agent-card.json' || url.pathname === '/agents/simone-mcp/.well-known/agent.json') {
      return Response.json(AGENT, { headers: { 'cache-control': 'no-store' } });
    }
    if (url.pathname === '/agents/simone-mcp/a2a/v1') {
      return Response.json({ ok: true, service: 'simone-mcp', transport: 'jsonrpc-over-http' });
    }
    if (url.pathname === '/agents/simone-mcp') {
      return Response.redirect('https://a2a.delqhi.com/agents/simone-mcp/', 307);
    }
    return new Response('not found', { status: 404 });
  },
};
