<p align="center">
  <img src="./assets/simone-mcp-banner.PNG" alt="Simone MCP banner" width="960" />
</p>

<p align="center">
  <a href="https://github.com/Delqhi/Simone-MCP/blob/main/LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License" />
  </a>
  <a href="https://www.python.org/downloads/">
    <img src="https://img.shields.io/badge/python-3.12+-3776AB.svg?logo=python&logoColor=white" alt="Python" />
  </a>
  <a href="https://fastapi.tiangolo.com/">
    <img src="https://img.shields.io/badge/FastAPI-005571?logo=fastapi" alt="FastAPI" />
  </a>
   <a href=".gitnexus/">
    <img src="https://img.shields.io/badge/GitNexus-knowledge%20graph-8B5CF6" alt="GitNexus" />
  </a>
  <a href="https://github.com/modelcontextprotocol">
    <img src="https://img.shields.io/badge/MCP-2026--06--30-068A0A?logo=mcp" alt="MCP Protocol" />
  </a>
  <a href="https://github.com/Delqhi/Simone-MCP/stargazers">
    <img src="https://img.shields.io/badge/A2A-Ready-7B3FE4" alt="A2A Ready" />
  </a>
  <a href="https://github.com/Delqhi/Simone-MCP/actions">
    <img src="https://img.shields.io/badge/build-passing-2EA043" alt="Build" />
  </a>
</p>

<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#use-cases">Use Cases</a> ·
  <a href="#commands">Commands</a> ·
  <a href="#deploy">Deploy</a> ·
  <a href="#contributing">Contributing</a>
</p>

<p align="center">
  <em>Production-grade Code-Worker with MCP 2026-06-30 compliance, symbol operations, dual transports, Tasks v2 (SEP-2663), structured output, HTTP header standardization (SEP-2243), list TTL (SEP-2549), OAuth 2.1 readiness, and hybrid memory integrations.</em>
</p>

---

## What is Simone MCP?

> Simone MCP is a **production-grade code worker** that transforms how AI agents navigate and manipulate code. Powered by **OpenAI** (`gpt-5.4`) with **NVIDIA** fallback, it provides **AST-level symbol operations** through the Model Context Protocol — giving OpenCode, Codex, and A2A agents surgical precision for code understanding and transformation.

**Think of it as a semantic search engine + code surgeon combined into one MCP server.**

---

## Quick Start

> [!TIP]
> Get Simone MCP running in under 60 seconds. No configuration required for basic usage.

<table>
<tr>
<td width="33%" align="center">
<strong>1. Clone</strong><br/><br/>
<code>git clone</code><br/><code>Delqhi/Simone-MCP</code><br/><br/>
<img src="https://img.shields.io/badge/⏱️_30s-Blue?style=flat" />
</td>
<td width="33%" align="center">
<strong>2. Install</strong><br/><br/>
<code>pip install -e .[dev]</code><br/><br/>
<img src="https://img.shields.io/badge/⏱️_30s-Blue?style=flat" />
</td>
<td width="33%" align="center">
<strong>3. Run</strong><br/><br/>
<code>python src/cli.py serve</code><br/><br/>
<img src="https://img.shields.io/badge/⏱️_Go!-Green?style=flat" />
</td>
</tr>
</table>

**That's it.** Server is now running at `http://localhost:8234` with full MCP, A2A, and `.well-known` endpoints.

---

## Features

| Capability | Description | Status |
|:---|:---|:---:|
| **Symbol Operations** | AST-level find, replace, and insert for Python functions and classes | ✅ |
| **MCP 2026-06-30** | Tasks v2 (SEP-2663), HTTP headers (SEP-2243), list TTL (SEP-2549), structured output, outputSchema, resource_link, _meta propagation | ✅ |
| **Dual Transport** | stdio for local clients + streamable HTTP for remote deployments | ✅ |
| **A2A Integration** | JSON-RPC endpoint for agent-to-agent communication | ✅ |
| **OAuth 2.1 Ready** | Bearer token validation with JWKS support | ✅ |
| **Hybrid Memory** | Qdrant (vector) + Neo4j (graph) retrieval architecture | ✅ |
| **Discovery** | `.well-known` metadata for agent cards and OAuth config | ✅ |
| **Docker Ready** | Single-image deployment with docker-compose for full stack | ✅ |
| **HF Spaces** | Stateless compute deployment pattern documented | ✅ |

<details>
<summary>📦 Full tool surface</summary>

| Tool | Title | Type | Task Support | Description |
|:---|:---|:---|:---|:---|
| `sin_simone_mcp_health` | Health Check | Meta | forbidden | Server health check and status |
| `sin_simone_mcp_symbol_search` | Symbol Search | Read | forbidden | Locate symbol definitions across workspace |
| `sin_simone_mcp_find_references` | Find References | Read | forbidden | Find textual references to a symbol |
| `sin_simone_mcp_structural_edit` | Structural Edit | Write | forbidden | Replace/insert code via structural payload |
| `sin_simone_mcp_memory_query` | Memory Query | Read | forbidden | Hybrid memory search via Qdrant + Neo4j |
| `sin_simone_mcp_project_overview` | Project Overview | Read | forbidden | Summarize workspace footprint and file types |

All tools provide `outputSchema` (JSON Schema 2020-12) and return `structuredContent` inline (no task deferral).

</details>

<details>
<summary>🧩 MCP 2026-06-30 Features</summary>

| Feature | Description |
|:---|:---|
| **Tasks v2 (SEP-2663)** | `tasks/get` (inline result), `tasks/update`, `tasks/cancel` + `notifications/tasks` — server decides task creation autonomously |
| **HTTP Headers (SEP-2243)** | `Mcp-Method`, `Mcp-Name`, `Mcp-Param-*` header validation with `-32001` HeaderMismatch error |
| **List TTL (SEP-2549)** | `ttlMs` + `cacheScope` on all list responses (tools, resources, prompts, templates) |
| **Structured Output** | `structuredContent` + `outputSchema` on all tool results |
| **Tool Title & Icons** | `title` field on all tools, `execution.taskSupport` (forbidden) |
| **Resource Links** | `resource_link` type in tool results for file references |
| **Input Validation (SEP-1303)** | Validation errors return `isError: true`, not JSON-RPC protocol errors |
| **_meta Propagation** | Request `_meta` echoed in all response results |
| **Logging Notifications** | `notifications/message` emitted on `logging/setLevel` |
| **Version Negotiation** | Highest `<=` client version selected during initialize |
| **SSE Retry** | `retry:` field in SSE streams |
| **MCP-Protocol-Version Header** | HTTP request/response header support |
| **Session Cleanup** | `DELETE /mcp` cleans up session + task store |
| **Extensions** | `io.modelcontextprotocol/tasks` extension declaration |

</details>

---

## Architecture

```mermaid
flowchart TB
    subgraph Clients["Clients"]
        direction LR
        OC["OpenCode CLI"]
        CX["Codex"]
        A2A["A2A Agents"]
    end

    subgraph Transport["Transport Layer"]
        direction LR
        STDIO["MCP stdio"]
        HTTP["FastAPI :8234"]
    end

    subgraph Core["Core Engine"]
        direction LR
        EXEC["Action Executor"]
        SYMBOL["Python AST"]
        MEMORY["Memory Facade"]
    end

    subgraph Storage["Storage"]
        direction LR
        QDRANT[("Qdrant")]
        NEO4J[("Neo4j")]
    end

    OC -->|stdio| STDIO
    CX -->|stdio| STDIO
    A2A -->|HTTP| HTTP
    STDIO --> EXEC
    HTTP --> EXEC
    EXEC --> SYMBOL
    EXEC --> MEMORY
    MEMORY --> QDRANT
    MEMORY --> NEO4J

    classDef clientClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef transportClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef coreClass fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px
    classDef storageClass fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class OC,CX,A2A clientClass
    class STDIO,HTTP transportClass
    class EXEC,SYMBOL,MEMORY coreClass
    class QDRANT,NEO4J storageClass
```

> 📖 **Deep dive:** Full architecture docs with 12+ diagrams at [docs/architecture.md](docs/architecture.md)

---

## Use Cases

| Who | Problem | Solution |
|:---|:---|:---|
| **🧑‍💻 Developer** | Manual code navigation in large repos | Symbol-level precision lookup in milliseconds |
| **🤖 A2A Agent** | No code understanding beyond text search | AST-parsed operations via standardized MCP |
| **👨‍💼 Team Lead** | Slow developer onboarding | Instant repo structure overview and navigation |
| **🏢 Enterprise** | Code quality and consistency at scale | Automated structural edits with validation |

---

## Commands

<details open>
<summary>🚀 Serve (Production)</summary>

```bash
# HTTP Server (default port 8234)
python3 src/cli.py serve

# Custom port
python3 src/cli.py serve 9000
```

</details>

<details>
<summary>🔌 MCP stdio (Local Development)</summary>

```bash
# MCP stdio mode for OpenCode/Codex
python3 src/cli.py serve-mcp
```

</details>

<details>
<summary>🃏 Agent Card & Actions</summary>

```bash
# Print agent discovery card
python3 src/cli.py print-card

# Execute a specific action
python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
python3 src/cli.py run-action '{"action":"code.find_symbol","symbol":"my_function"}'
```

</details>

<details>
<summary>🧪 Validation</summary>

```bash
# Run test suite
pytest tests/ -v
```

</details>

---

## Deploy

| Method | Command | Best For |
|:---|:---|:---|
| **Local** | `pip install -e .[dev]` | Development, testing |
| **Docker** | `docker-compose up --build` | Production, CI/CD |
| **HF Spaces** | Push to Hugging Face Spaces | Stateless compute, demos |

<details>
<summary>🐳 Docker Compose (Full Stack)</summary>

```bash
# Starts Simone MCP + Qdrant + Neo4j
docker-compose up --build

# Services:
#   Simone MCP  → http://localhost:8234
#   Qdrant      → http://localhost:6333
#   Neo4j       → http://localhost:7474
```

</details>

> [!WARNING]
> **HF Spaces:** Hugging Face Spaces provide stateless compute only. Use external services (Supabase, Qdrant Cloud, Neo4j Aura) for persistent state. Never assume local disk durability.

---

## Configuration

Copy `.env.example` to `.env` and configure the values you need:

<details>
<summary>🔐 Environment Variables</summary>

| Variable | Purpose | Default |
|:---|:---|:---|
| `SIMONE_AUTH_REQUIRED` | Enable OAuth validation | `false` |
| `SIMONE_OAUTH_AUDIENCE` | JWT audience claim | `simone-mcp` |
| `SIMONE_OAUTH_ISSUER` | OAuth issuer URL | — |
| `SIMONE_OAUTH_JWKS_URL` | JWKS endpoint for token validation | — |
| `SIMONE_ALLOWED_ORIGINS` | CORS origin whitelist | `http://localhost` |
| `QDRANT_URL` | Vector database endpoint | — |
| `NEO4J_URI` | Graph database endpoint | — |
| `SUPABASE_URL` | Supabase project URL | — |

</details>

---

## Contributing

Contributions are welcome! Here's how to get started:

1. **Fork** the repository
2. **Create** your feature branch (`git checkout -b feature/amazing-feature`)
3. **Commit** your changes (`git commit -m 'Add amazing feature'`)
4. **Push** to the branch (`git push origin feature/amazing-feature`)
5. **Open** a Pull Request

> [!NOTE]
> Please ensure all tests pass (`pytest tests/ -v`) and run `python3 src/cli.py print-card` to verify the agent card is valid before submitting.

---

## License

Distributed under the **MIT License**. See [LICENSE](LICENSE) for more information.

---

<p align="center">
  <a href="https://opensin.ai">
    <img src="https://img.shields.io/badge/🤖_Powered_by-OpenSIN--AI-7B3FE4?style=for-the-badge&logo=github&logoColor=white" alt="Powered by OpenSIN-AI" />
  </a>
</p>

<p align="center">
  <sub>Entwickelt vom <a href="https://opensin.ai"><strong>OpenSIN-AI</strong></a> Ökosystem – Enterprise AI Agents die autonom arbeiten.</sub><br/>
  <sub>🌐 <a href="https://opensin.ai">opensin.ai</a> · 💬 <a href="https://opensin.ai/agents">Alle Agenten</a> · 🚀 <a href="https://opensin.ai/dashboard">Dashboard</a></sub>
</p>
