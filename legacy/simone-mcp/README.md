<p align="center">
  <img src="./assets/simone-mcp-banner.PNG" alt="Simone MCP banner" />
</p>

# Simone MCP

Simone MCP is a production-grade code worker for the OpenSIN ecosystem. It combines a real Python implementation, dual MCP transports, A2A discovery, symbol-level code operations, OAuth 2.1 readiness, and hybrid memory integration points.

## 📊 Visual Architecture Overview

```mermaid
graph TB
    subgraph Clients["🖥️ Clients"]
        OC[OpenCode CLI]
        CX[Codex]
        A2A[A2A Agents]
    end

    subgraph Transport["🔌 Transport Layer"]
        STDIO[MCP stdio<br/>Server]
        HTTP[FastAPI HTTP<br/>Server :8234]
    end

    subgraph Core["⚙️ Simone Core Engine"]
        EXEC[Action Executor]
        SYMBOL[Symbol Operations<br/>Python AST]
        MEMORY[Memory Facade]
        AUTH[OAuth 2.1<br/>Validator]
    end

    subgraph Storage["💾 Memory & Storage"]
        QDRANT[(Qdrant<br/>Vector DB)]
        NEO4J[(Neo4j<br/>Graph DB)]
        SUPABASE[(Supabase)]
    end

    OC -->|stdio| STDIO
    CX -->|stdio| STDIO
    A2A -->|HTTP| HTTP

    STDIO --> EXEC
    HTTP --> MCP[MCP /mcp]
    HTTP --> A2AEP[A2A /a2a/v1]
    HTTP --> WELL[.well-known]
    
    MCP --> AUTH
    A2AEP --> EXEC
    WELL --> EXEC
    AUTH --> EXEC
    
    EXEC --> SYMBOL
    EXEC --> MEMORY
    MEMORY --> QDRANT
    MEMORY --> NEO4J
    MEMORY --> SUPABASE

    classDef client fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef transport fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef core fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px
    classDef storage fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class OC,CX,A2A client
    class STDIO,HTTP transport
    class EXEC,SYMBOL,MEMORY,AUTH,MCP,A2AEP,WELL core
    class QDRANT,NEO4J,SUPABASE storage
```

## 🔄 Request Flow

### Local Development (stdio)

```mermaid
sequenceDiagram
    participant User as 👤 Developer
    participant CLI as CLI serve-mcp
    participant STDIO as MCP stdio Server
    participant CORE as Core Engine
    participant AST as Python AST Parser

    User->>CLI: python src/cli.py serve-mcp
    CLI->>STDIO: Start stdio loop
    
    User->>STDIO: {"method": "initialize"}
    STDIO-->>User: {sessionId, protocolVersion}
    
    User->>STDIO: {"method": "tools/call"}
    STDIO->>CORE: execute_simone_action()
    CORE->>AST: Parse .py files
    AST-->>CORE: Symbol matches
    CORE-->>STDIO: Result
    STDIO-->>User: JSON-RPC response
```

### Remote HTTP (Streamable)

```mermaid
sequenceDiagram
    participant Agent as 🤖 A2A Agent
    participant HTTP as FastAPI Server
    participant AUTH as OAuth Validator
    participant CORE as Core Engine

    Agent->>HTTP: POST /a2a/v1 or /mcp
    HTTP->>AUTH: Validate Origin & Bearer
    AUTH-->>HTTP: Token valid
    HTTP->>CORE: execute_simone_action()
    CORE-->>HTTP: Result
    HTTP-->>Agent: JSON-RPC response
```

## 🚀 Deployment Options

```mermaid
graph LR
    subgraph Local["💻 Local Development"]
        L1[Python venv] --> L2[pip install -e .]
        L2 --> L3[pytest + serve]
    end

    subgraph Docker["🐳 Docker Compose"]
        D1[Simone MCP :8234] --> D2[(Qdrant :6333)]
        D1 --> D3[(Neo4j :7687)]
    end

    subgraph Cloud["☁️ Hugging Face Spaces"]
        C1[Stateless Runtime] --> C2[(External DBs)]
    end

    classDef local fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef docker fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef cloud fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class L1,L2,L3 local
    class D1,D2,D3 docker
    class C1,C2 cloud
```

## 🛠️ Tool Surface

```mermaid
mindmap
  root((Simone MCP))
    Read-Only
      code.find_symbol
      code.find_references
      code.project_overview
      memory.query
    Write
      code.replace_symbol_body
      code.insert_after_symbol
    Meta
      agent.help
      simone.mcp.health
```

---

## Fleet policy

- Every OpenCode agent in this ecosystem must use Simone MCP when it is available.
- PCPM is the required planning/memory layer before a repo task begins.
- Local development uses stdio; remote use uses the HF Space / streamable HTTP shape.

## What is implemented now

- Python source of truth under `src/`
- MCP stdio server for local OpenCode/Codex usage
- MCP streamable HTTP server at `/mcp`
- A2A JSON-RPC endpoint at `/a2a/v1`
- `.well-known` discovery metadata
- Symbol tools for Python workspaces
- Structural edits for Python functions and class-adjacent insertion
- Dashboard endpoint with operator quick actions
- Docker and docker-compose scaffolding
- n8n-dispatch CI wrapper workflow

## April 2026 design choices

- Use **Streamable HTTP** for remote MCP, not deprecated HTTP+SSE split endpoints
- Keep **stdio** for local client compatibility
- Validate **Origin** on HTTP transport
- Prepare for **OAuth 2.1** with Bearer + JWKS validation
- Prefer **hybrid retrieval**: vector-first candidate selection and graph-aware expansion
- Treat **Hugging Face Spaces as stateless compute** and keep durable state remote

## Quick start

```bash
git clone https://github.com/Delqhi/Simone-MCP.git
cd Simone-MCP
python3 -m venv .venv
source .venv/bin/activate
pip install -e .[dev]
pytest tests/ -v
python3 src/cli.py print-card
python3 src/cli.py serve
```

## Main commands

```bash
python3 src/cli.py serve
python3 src/cli.py serve-mcp
python3 src/cli.py print-card
python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
```

## HTTP endpoints

- `GET /health`
- `GET /dashboard`
- `GET /.well-known/agent-card.json`
- `GET /.well-known/agent.json`
- `GET /.well-known/oauth-client.json`
- `GET /.well-known/oauth-authorization-server`
- `POST /a2a/v1`
- `GET|POST|DELETE /mcp`

## Core tool surface

- `code.find_symbol`
- `code.find_references`
- `code.replace_symbol_body`
- `code.insert_after_symbol`
- `code.project_overview`
- `memory.query`
- `simone.mcp.health`

## Docker

```bash
docker-compose up --build
```

## Configuration

Copy `.env.example` to `.env` and set the values you actually use.

Important runtime variables:

- `SIMONE_AUTH_REQUIRED`
- `SIMONE_OAUTH_AUDIENCE`
- `SIMONE_OAUTH_ISSUER`
- `SIMONE_OAUTH_JWKS_URL`
- `SIMONE_ALLOWED_ORIGINS`
- `QDRANT_URL`
- `NEO4J_URI`
- `SUPABASE_URL`

## Validation

```bash
pytest tests/ -v
python3 src/cli.py print-card
python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
```

## CI

The repository is configured for the thin OpenSIN CI dispatch model through `OpenSIN-AI/sin-github-action` and an n8n webhook secret.

## Deployment note

For Hugging Face Spaces, prefer remote persistence or mounted volumes instead of assuming local disk durability for long-lived agent state.

## 📚 Detailed Documentation

For comprehensive visual documentation including:
- OAuth 2.1 Authentication Flow
- Memory Integration Architecture  
- Security Architecture
- CI/CD Pipeline
- File Structure Diagram
- Agent Card & Discovery

👉 See [docs/architecture.md](docs/architecture.md)

## License

MIT
