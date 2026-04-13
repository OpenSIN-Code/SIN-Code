# Simone MCP Architecture Documentation

> **Bilder sagen mehr als tausend Worte** - Visuelle Dokumentation der Simone MCP Architektur

## Übersicht

Simone MCP ist ein production-grade Code Worker für das OpenSIN Ökosystem mit dualen MCP Transports, A2A Discovery, Symbol-Level Code Operationen und Hybrid Memory Integration.

---

## 1. System Architecture Overview

```mermaid
graph TB
    subgraph Clients["🖥️ Clients"]
        OC[OpenCode CLI]
        CX[Codex]
        A2A[A2A Agents]
        WEB[Web Dashboard]
    end

    subgraph Transport["🔌 Transport Layer"]
        STDIO[MCP stdio Server]
        HTTP[FastAPI HTTP Server]
    end

    subgraph Endpoints["🌐 API Endpoints"]
        MCP[MCP Streamable HTTP<br/>/mcp]
        A2AEP[A2A JSON-RPC<br/>/a2a/v1]
        WELL[.well-known<br/>Metadata]
        HEALTH[Health<br/>/health]
        DASH[Dashboard<br/>/dashboard]
    end

    subgraph Core["⚙️ Simone Core Engine"]
        EXEC[Action Executor]
        SYMBOL[Symbol Operations]
        MEMORY[Memory Facade]
        AUTH[OAuth 2.1 Validator]
    end

    subgraph Storage["💾 Memory & Storage"]
        QDRANT[(Qdrant<br/>Vector DB)]
        NEO4J[(Neo4j<br/>Graph DB)]
        SUPABASE[(Supabase<br/>Relational)]
    end

    OC -->|stdio| STDIO
    CX -->|stdio| STDIO
    A2A -->|HTTP| HTTP
    WEB -->|HTTP| HTTP

    STDIO --> EXEC
    HTTP --> MCP
    HTTP --> A2AEP
    HTTP --> WELL
    HTTP --> HEALTH
    HTTP --> DASH

    MCP --> AUTH
    A2AEP --> EXEC
    WELL --> EXEC

    AUTH --> EXEC
    EXEC --> SYMBOL
    EXEC --> MEMORY

    SYMBOL --> FILES[Python AST<br/>File Operations]
    MEMORY --> QDRANT
    MEMORY --> NEO4J
    MEMORY --> SUPABASE

    classDef client fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef transport fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef endpoint fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef core fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px
    classDef storage fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class OC,CX,A2A,WEB client
    class STDIO,HTTP transport
    class MCP,A2AEP,WELL,HEALTH,DASH endpoint
    class EXEC,SYMBOL,MEMORY,AUTH core
    class QDRANT,NEO4J,SUPABASE storage
```

---

## 2. Request Flow Diagrams

### 2.1 Local Development Flow (stdio)

```mermaid
sequenceDiagram
    participant User as 👤 Developer
    participant CLI as CLI<br/>serve-mcp
    participant STDIO as MCP stdio<br/>Server
    participant CORE as Core Engine
    participant AST as Python AST<br/>Parser
    participant FS as File System

    User->>CLI: python src/cli.py serve-mcp
    CLI->>STDIO: Start stdio loop
    
    User->>STDIO: {"method": "initialize"}
    STDIO->>STDIO: Generate session_id
    STDIO-->>User: {"result": {protocolVersion, sessionId}}
    
    User->>STDIO: {"method": "tools/list"}
    STDIO-->>User: {"tools": [find_symbol, ...]}
    
    User->>STDIO: {"method": "tools/call",<br/>"params": {"name": "code.find_symbol"}}
    STDIO->>CORE: execute_simone_action(payload)
    CORE->>AST: Parse Python files
    AST->>FS: Read .py files
    FS-->>AST: File content
    AST-->>CORE: Symbol matches
    CORE-->>STDIO: {ok: true, matches: [...]}
    STDIO-->>User: JSON-RPC response
```

### 2.2 Remote HTTP Flow (Streamable HTTP)

```mermaid
sequenceDiagram
    participant Agent as 🤖 A2A Agent
    participant HTTP as FastAPI Server<br/>:8234
    participant AUTH as OAuth 2.1<br/>Validator
    participant CORE as Core Engine
    participant A2A as A2A JSON-RPC<br/>Handler
    participant MCP as MCP Streamable<br/>Handler

    Agent->>HTTP: POST /a2a/v1<br/>{method: "message/send"}
    HTTP->>AUTH: Validate Origin & Bearer Token
    AUTH-->>HTTP: Token valid
    
    HTTP->>A2A: Parse A2A request
    A2A->>CORE: execute_simone_action(action)
    CORE-->>A2A: Result
    
    A2A-->>HTTP: JSON-RPC response<br/>with artifacts
    HTTP-->>Agent: {jsonrpc: "2.0", result: {...}}
    
    Note over Agent,MCP: Alternative: MCP Streamable HTTP
    Agent->>HTTP: POST /mcp<br/>{method: "tools/call"}
    HTTP->>AUTH: Validate request
    AUTH-->>HTTP: Auth OK
    HTTP->>MCP: Process MCP request
    MCP->>CORE: execute_simone_action(action)
    CORE-->>MCP: Result
    MCP-->>Agent: MCP response with session_id
```

---

## 3. MCP Transport Comparison

```mermaid
graph LR
    subgraph STDIO["📡 MCP stdio Mode"]
        direction TB
        S1[Local CLI] -->|stdin| S2[JSON-RPC]
        S2 -->|stdout| S3[Response]
        S4[Use Case: Local Dev]
    end

    subgraph HTTP["🌐 Streamable HTTP Mode"]
        direction TB
        H1[Remote Client] -->|POST /mcp| H2[FastAPI Server]
        H2 -->|Session ID| H3[Stateful Connection]
        H3 -->|GET /mcp| H4[SSE Event Stream]
        H5[Use Case: HF Spaces, Remote]
    end

    subgraph A2A["🤖 A2A JSON-RPC Mode"]
        direction TB
        A1[A2A Agent] -->|POST /a2a/v1| A2[Message Router]
        A2 -->|agent/getCard| A3[Agent Card]
        A2 -->|message/send| A4[Action Executor]
        A5[Use Case: Agent-to-Agent]
    end

    STDIO -.->|Both use same| CORE[(Core Engine)]
    HTTP -.->|Core| CORE
    A2A -.->|Core| CORE

    classDef stdio fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    classDef http fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef a2a fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef core fill:#e8f5e9,stroke:#2e7d32,stroke-width:3px

    class S1,S2,S3,S4 stdio
    class H1,H2,H3,H4,H5 http
    class A1,A2,A3,A4,A5 a2a
    class CORE core
```

---

## 4. Symbol Operations Deep Dive

```mermaid
graph TD
    subgraph FindSymbol["🔍 code.find_symbol"]
        FS1[Input: symbol name + root] --> FS2[Scan .py files recursively]
        FS2 --> FS3[Parse each file with ast.parse]
        FS3 --> FS4[Walk AST tree]
        FS4 --> FS5{Node type?}
        FS5 -->|FunctionDef| FS6[function]
        FS5 -->|AsyncFunctionDef| FS7[async_function]
        FS5 -->|ClassDef| FS8[class]
        FS6 --> FS9[Match symbol name]
        FS7 --> FS9
        FS8 --> FS9
        FS9 --> FS10[Return: file, line, column, kind]
    end

    subgraph ReplaceBody["✏️ code.replace_symbol_body"]
        RB1[Input: symbol + file + new body] --> RB2[Find symbol node in AST]
        RB2 --> RB3[Extract function boundaries]
        RB3 --> RB4[Calculate indentation]
        RB4 --> RB5[Replace line range]
        RB5 --> RB6[Preserve trailing newline]
        RB6 --> RB7[Write updated file]
    end

    subgraph InsertAfter["📝 code.insert_after_symbol"]
        IA1[Input: symbol + file + text] --> IA2[Find symbol end_lineno]
        IA2 --> IA3[Insert text after node]
        IA3 --> IA4[Write updated file]
    end

    classDef find fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef replace fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef insert fill:#f3e5f5,stroke:#4a148c,stroke-width:2px

    class FS1,FS2,FS3,FS4,FS5,FS6,FS7,FS8,FS9,FS10 find
    class RB1,RB2,RB3,RB4,RB5,RB6,RB7 replace
    class IA1,IA2,IA3,IA4 insert
```

---

## 5. OAuth 2.1 Authentication Flow

```mermaid
sequenceDiagram
    participant Client as 🖥️ Client Agent
    participant Simone as Simone MCP<br/>Server
    participant JWKS as JWKS Endpoint
    participant Issuer as OAuth Issuer

    Client->>Simone: GET /.well-known/oauth-client.json
    Simone-->>Client: {redirect_uris, grant_types}
    
    Client->>Simone: GET /.well-known/oauth-authorization-server
    Simone-->>Client: {authorize_endpoint, token_endpoint, jwks_uri}
    
    Note over Client,Issuer: Authorization Code Flow
    Client->>Issuer: Authorization Request<br/>+ PKCE (S256)
    Issuer->>Client: Authorization Code
    
    Client->>Issuer: POST /token<br/>{code, code_verifier}
    Issuer-->>Client: Access Token (JWT)
    
    Note over Client,Simone: Protected Request
    Client->>Simone: POST /mcp<br/>Authorization: Bearer <JWT>
    Simone->>JWKS: Fetch signing key
    JWKS-->>Simone: Public Key
    Simone->>Simone: Verify JWT signature,<br/>audience, issuer
    Simone-->>Client: ✅ Authorized Response
```

---

## 6. Memory Integration Architecture

```mermaid
graph TB
    subgraph Query["🧠 Hybrid Memory Query"]
        Q1[memory.query<br/>Input: search query] --> Q2[Memory Facade]
    end

    subgraph Vector["📊 Vector Search - Qdrant"]
        Q2 -->|If QDRANT_URL configured| V1[Generate embeddings]
        V1 --> V2[Query Qdrant collections]
        V2 --> V3[Return top-k similar chunks]
    end

    subgraph Graph["🕸️ Graph Search - Neo4j"]
        Q2 -->|If NEO4J_URI configured| G1[Cypher query generation]
        G1 --> G2[Query Neo4j graph]
        G2 --> G3[Return related entities]
    end

    subgraph Fusion["🔄 Result Fusion"]
        V3 --> F1[Merge & rank results]
        G3 --> F1
        F1 --> F2[Return unified response]
    end

    Q1:::query
    Q2:::facade
    V1:::vector
    V2:::vector
    V3:::vector
    G1:::graph
    G2:::graph
    G3:::graph
    F1:::fusion
    F2:::fusion

    classDef query fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef facade fill:#fff9c4,stroke:#f57f17,stroke-width:2px
    classDef vector fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef graph fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef fusion fill:#ffe0b2,stroke:#e65100,stroke-width:2px
```

---

## 7. Deployment Topology

### 7.1 Local Development

```mermaid
graph LR
    DEV[Developer Machine] --> PYTHON[Python 3.12 venv]
    PYTHON --> PIP[pip install -e .[dev]]
    PIP --> TEST[pytest tests/ -v]
    TEST --> SERVE[python src/cli.py serve]
    SERVE --> LOCAL[http://localhost:8234]
    
    classDef dev fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDev DEV,PYTHON,PIP,TEST,SERVE,LOCAL dev
```

### 7.2 Docker Compose Stack

```mermaid
graph TB
    subgraph Docker["🐳 Docker Compose Stack"]
        subgraph Services["Services"]
            SIMONE[Simone MCP<br/>:8234]
            QDRANT[Qdrant<br/>:6333]
            NEO4J[Neo4j<br/>:7474, :7687]
        end

        subgraph Volumes["💾 Persistent Volumes"]
            QDATA[(qdrant_data)]
            NDATA[(neo4j_data)]
        end

        subgraph Network["🌐 Docker Network"]
            SIMONE_NET[simone<br/>bridge network]
        end
    end

    SIMONE -->|http://qdrant:6333| QDRANT
    SIMONE -->|bolt://neo4j:7687| NEO4J
    QDRANT --> QDATA
    NEO4J --> NDATA
    SIMONE --> SIMONE_NET
    QDRANT --> SIMONE_NET
    NEO4J --> SIMONE_NET

    classDef service fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef volume fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef network fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px

    class SIMONE,QDRANT,NEO4J service
    class QDATA,NDATA volume
    class SIMONE_NET network
```

### 7.3 Hugging Face Spaces Deployment

```mermaid
graph TB
    subgraph HF["Hugging Face Spaces"]
        SPACE[Space Runtime<br/>Stateless Compute]
        APP[Simone MCP App<br/>Port 7860]
    end

    subgraph External["External Persistent Storage"]
        SUPABASE[(Supabase DB)]
        REMOTE_QDRANT[(Remote Qdrant)]
        REMOTE_NEO4J[(Remote Neo4j)]
    end

    SPACE --> APP
    APP -->|Environment Variables| SUPABASE
    APP -->|SIMONE_* env vars| REMOTE_QDRANT
    APP -->|NEO4J_URI env var| REMOTE_NEO4J

    Note over SPACE: ⚠️ No local disk persistence!<br/>Use external services for state

    classDef hf fill:#ffeb3b,stroke:#f57f17,stroke-width:2px
    classDef external fill:#e1f5fe,stroke:#01579b,stroke-width:2px

    class SPACE,APP hf
    class SUPABASE,REMOTE_QDRANT,REMOTE_NEO4J external
```

---

## 8. Tool Surface & Capabilities

```mermaid
mindmap
  root((Simone MCP<br/>Tools))
    Read-Only Tools
      code.find_symbol
        ::icon(fa fa-search)
        Locate symbols across workspace
      code.find_references
        ::icon(fa fa-link)
        Find textual references
      code.project_overview
        ::icon(fa fa-folder)
        Workspace footprint summary
      memory.query
        ::icon(fa fa-database)
        Hybrid memory search
    Write Tools
      code.replace_symbol_body
        ::icon(fa fa-edit)
        Replace Python function body
      code.insert_after_symbol
        ::icon(fa fa-plus)
        Insert text after symbol
    Meta Tools
      agent.help
        ::icon(fa fa-question)
        List available actions
      simone.mcp.health
        ::icon(fa fa-heartbeat)
        Server health check
    Capabilities
      memory.hybrid
      transport.streamable_http
      auth.oauth2.1
```

---

## 9. CI/CD Pipeline

```mermaid
graph LR
    subgraph Dev["👨‍💻 Development"]
        CODE[Code Changes] --> COMMIT[Git Commit]
        COMMIT --> PUSH[Push to GitHub]
    end

    subgraph CI["🔄 CI Pipeline"]
        PUSH --> TRIGGER[GitHub Actions Trigger]
        TRIGGER --> LINT[Lint & Type Check]
        LINT --> TEST[pytest tests/ -v]
        TEST --> CARD[print-card validation]
        CARD --> HEALTH[health check]
    end

    subgraph Deploy["🚀 Deployment"]
        HEALTH --> BUILD[Docker Build]
        BUILD --> PUSH_IMG[Push to Registry]
        PUSH_IMG --> DEPLOY_HF[Deploy to HF Spaces]
        DEPLOY_HF --> VERIFY[Health Verification]
    end

    classDef dev fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef ci fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef deploy fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px

    class CODE,COMMIT,PUSH dev
    class TRIGGER,LINT,TEST,CARD,HEALTH ci
    class BUILD,PUSH_IMG,DEPLOY_HF,VERIFY deploy
```

---

## 10. File Structure

```mermaid
graph TD
    ROOT[Simone-MCP/] --> SRC[src/]
    ROOT --> TEST[tests/]
    ROOT --> WELL[.well-known/]
    ROOT --> DOCS[docs/]
    ROOT --> ASSETS[assets/]
    ROOT --> CONF[Config Files]

    SRC --> SIMONE_MCP[simone_mcp/]
    SRC --> MAIN[main.py]
    SRC --> CLI[cli.py]
    SRC --> MCP[mcp_server.py]

    SIMONE_MCP --> CORE[core.py<br/>Action Engine]
    SIMONE_MCP --> HTTP[http_app.py<br/>FastAPI Server]
    SIMONE_MCP --> STDIO[mcp_stdio.py<br/>stdio Server]
    SIMONE_MCP --> CLI_MOD[cli.py<br/>CLI Handler]
    SIMONE_MCP --> INIT[__init__.py]

    CONF --> PYPROJECT[pyproject.toml]
    CONF --> DOCKERFILE[Dockerfile]
    CONF --> COMPOSE[docker-compose.yml]
    CONF --> ENV[.env.example]
    CONF --> MCP_CFG[mcp-config.json]
    CONF --> AGENT[agent.json]
    CONF --> CARD[A2A-CARD.md]

    classDef dir fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    classDef py fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef cfg fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class ROOT,SRC,TEST,WELL,DOCS,ASSETS,CONF,SIMONE_MCP dir
    class CORE,HTTP,STDIO,CLI_MOD,INIT,MAIN,CLI,MCP py
    class PYPROJECT,DOCKERFILE,COMPOSE,ENV,MCP_CFG,AGENT,CARD cfg
```

---

## 11. Security Architecture

```mermaid
graph TB
    subgraph Request["📨 Incoming Request"]
        REQ[HTTP Request] --> ORIGIN[Origin Validation]
    end

    subgraph Auth["🔐 Authentication Layer"]
        ORIGIN -->|Origin allowed?| AUTH_CHECK{Auth Required?}
        AUTH_CHECK -->|No| PASS[Proceed]
        AUTH_CHECK -->|Yes| BEARER{Bearer Token?}
        BEARER -->|No| 401[401 Unauthorized]
        BEARER -->|Yes| JWT[JWT Validation]
        JWT --> JWKS[Fetch JWKS Key]
        JWKS --> VERIFY{Valid Signature?}
        VERIFY -->|No| 401_ERR[401 Invalid Token]
        VERIFY -->|Yes| AUD{Audience Match?}
        AUD -->|No| 401_AUD[401 Invalid Audience]
        AUD -->|Yes| PASS
    end

    subgraph Open["🔓 Open Endpoints"]
        OPEN_LIST[/, /health, /dashboard,<br/>.well-known/*]
    end

    REQ -.->|Skip auth for| OPEN_LIST

    classDef request fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef auth fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef open fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef error fill:#ffebee,stroke:#c62828,stroke-width:2px

    class REQ,ORIGIN request
    class AUTH_CHECK,BEARER,JWT,JWKS,VERIFY,AUD,PASS auth
    class OPEN_LIST open
    class 401,401_ERR,401_AUD error
```

---

## 12. Agent Card & Discovery

```mermaid
graph LR
    subgraph WellKnown["🔍 .well-known Endpoints"]
        CARD[/.well-known/<br/>agent-card.json]
        AGENT[/.well-known/<br/>agent.json]
        OAUTH_CLIENT[/.well-known/<br/>oauth-client.json]
        OAUTH_SERVER[/.well-known/<br/>oauth-authorization-server]
    end

    subgraph CardContent["📋 Agent Card Content"]
        NAME[name: simone-mcp]
        VERSION[version: 2026.04.12]
        CAPS[capabilities: [...]]
        ENDPOINTS[endpoints: {...}]
        SKILLS[skills: [...]]
        AUTH_CONFIG[auth: {type: oauth2.1}]
    end

    CARD --> NAME
    CARD --> VERSION
    CARD --> CAPS
    CARD --> ENDPOINTS
    CARD --> SKILLS
    CARD --> AUTH_CONFIG

    classDef wk fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef content fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px

    class CARD,AGENT,OAUTH_CLIENT,OAUTH_SERVER wk
    class NAME,VERSION,CAPS,ENDPOINTS,SKILLS,AUTH_CONFIG content
```

---

## Zusammenfassung

Simone MCP bietet:

- **Duale Transports**: stdio für lokale Entwicklung, Streamable HTTP für Remote
- **A2A Integration**: JSON-RPC Endpoint für Agent-to-Agent Kommunikation  
- **Symbol-Level Operations**: Python AST-basierte Code-Navigation und -Manipulation
- **OAuth 2.1 Ready**: Bearer Token Validation mit JWKS
- **Hybrid Memory**: Qdrant (Vector) + Neo4j (Graph) Integration
- **Production-Ready**: Docker, docker-compose, HF Spaces Deployment
- **Discovery**: .well-known Metadata für Agent Card und OAuth Configuration
