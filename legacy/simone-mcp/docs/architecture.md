# Simone MCP Architecture Documentation

> **Bilder sagen mehr als tausend Worte** - Visuelle Dokumentation der Simone MCP Architektur

## Übersicht

Simone MCP ist ein production-grade Code Worker für das OpenSIN Ökosystem mit dualen MCP Transports, A2A Discovery, Symbol-Level Code Operationen und Hybrid Memory Integration.

---

## 1. System Architecture Overview

```mermaid
flowchart TB
    subgraph Clients["Clients"]
        direction LR
        OC["OpenCode CLI"]
        CX["Codex"]
        A2A["A2A Agents"]
        WEB["Web Dashboard"]
    end

    subgraph Transport["Transport Layer"]
        direction LR
        STDIO["MCP stdio Server"]
        HTTP["FastAPI HTTP Server"]
    end

    subgraph Endpoints["API Endpoints"]
        direction LR
        MCP["MCP /mcp"]
        A2AEP["A2A /a2a/v1"]
        WELL[".well-known"]
        HEALTH["/health"]
        DASH["/dashboard"]
    end

    subgraph Core["Simone Core Engine"]
        direction LR
        EXEC["Action Executor"]
        SYMBOL["Symbol Operations"]
        MEMORY["Memory Facade"]
        AUTH["OAuth 2.1 Validator"]
    end

    subgraph Storage["Memory & Storage"]
        direction LR
        QDRANT[("Qdrant Vector DB")]
        NEO4J[("Neo4j Graph DB")]
        SUPABASE[("Supabase")]
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

    SYMBOL --> FILES["Python AST File Operations"]
    MEMORY --> QDRANT
    MEMORY --> NEO4J
    MEMORY --> SUPABASE

    classDef clientClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef transportClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef endpointClass fill:#f3e5f5,stroke:#4a148c,stroke-width:2px
    classDef coreClass fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px
    classDef storageClass fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class OC,CX,A2A,WEB clientClass
    class STDIO,HTTP transportClass
    class MCP,A2AEP,WELL,HEALTH,DASH endpointClass
    class EXEC,SYMBOL,MEMORY,AUTH,FILES coreClass
    class QDRANT,NEO4J,SUPABASE storageClass
```

---

## 2. Request Flow Diagrams

### 2.1 Local Development Flow (stdio)

```mermaid
sequenceDiagram
    participant User as Developer
    participant CLI as CLI serve-mcp
    participant STDIO as MCP stdio Server
    participant CORE as Core Engine
    participant AST as Python AST Parser
    participant FS as File System

    User->>CLI: python src/cli.py serve-mcp
    CLI->>STDIO: Start stdio loop
    
    User->>STDIO: initialize
    STDIO->>STDIO: Generate session_id
    STDIO-->>User: protocolVersion + sessionId
    
    User->>STDIO: tools/list
    STDIO-->>User: tools array
    
    User->>STDIO: tools/call
    STDIO->>CORE: execute_simone_action
    CORE->>AST: Parse Python files
    AST->>FS: Read .py files
    FS-->>AST: File content
    AST-->>CORE: Symbol matches
    CORE-->>STDIO: Result
    STDIO-->>User: JSON-RPC response
```

### 2.2 Remote HTTP Flow (Streamable HTTP)

```mermaid
sequenceDiagram
    participant Agent as A2A Agent
    participant HTTP as FastAPI Server
    participant AUTH as OAuth 2.1 Validator
    participant CORE as Core Engine
    participant A2A as A2A Handler
    participant MCP as MCP Handler

    Agent->>HTTP: POST /a2a/v1
    HTTP->>AUTH: Validate Origin + Bearer
    AUTH-->>HTTP: Token valid
    
    HTTP->>A2A: Parse A2A request
    A2A->>CORE: execute_simone_action
    CORE-->>A2A: Result
    A2A-->>HTTP: JSON-RPC response
    HTTP-->>Agent: Response with artifacts
    
    Note over Agent,MCP: Alternative - MCP Streamable HTTP
    Agent->>HTTP: POST /mcp
    HTTP->>AUTH: Validate request
    AUTH-->>HTTP: Auth OK
    HTTP->>MCP: Process MCP request
    MCP->>CORE: execute_simone_action
    CORE-->>MCP: Result
    MCP-->>Agent: MCP response
```

---

## 3. MCP Transport Comparison

```mermaid
flowchart LR
    subgraph STDIO["MCP stdio Mode"]
        direction TB
        S1["Local CLI"] -->|stdin| S2["JSON-RPC"]
        S2 -->|stdout| S3["Response"]
        S4["Use Case: Local Dev"]
    end

    subgraph HTTPMODE["Streamable HTTP Mode"]
        direction TB
        H1["Remote Client"] -->|POST /mcp| H2["FastAPI Server"]
        H2 -->|Session ID| H3["Stateful Connection"]
        H3 -->|GET /mcp| H4["SSE Event Stream"]
        H5["Use Case: HF Spaces"]
    end

    subgraph A2AMODE["A2A JSON-RPC Mode"]
        direction TB
        A1["A2A Agent"] -->|POST /a2a/v1| A2["Message Router"]
        A2 -->|agent/getCard| A3["Agent Card"]
        A2 -->|message/send| A4["Action Executor"]
        A5["Use Case: Agent-to-Agent"]
    end

    STDIO -.->|Both use same| CORE[("Core Engine")]
    HTTPMODE -.-> CORE
    A2AMODE -.-> CORE

    classDef stdioClass fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    classDef httpClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef a2aClass fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef coreClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:3px

    class S1,S2,S3,S4 stdioClass
    class H1,H2,H3,H4,H5 httpClass
    class A1,A2,A3,A4,A5 a2aClass
    class CORE coreClass
```

---

## 4. Symbol Operations Deep Dive

```mermaid
flowchart TD
    subgraph FindSymbol["code.find_symbol"]
        direction TB
        FS1["Input: symbol name + root"] --> FS2["Scan .py files recursively"]
        FS2 --> FS3["Parse each file with ast.parse"]
        FS3 --> FS4["Walk AST tree"]
        FS4 --> FS5{"Node type?"}
        FS5 -->|FunctionDef| FS6["function"]
        FS5 -->|AsyncFunctionDef| FS7["async_function"]
        FS5 -->|ClassDef| FS8["class"]
        FS6 --> FS9["Match symbol name"]
        FS7 --> FS9
        FS8 --> FS9
        FS9 --> FS10["Return: file, line, column, kind"]
    end

    subgraph ReplaceBody["code.replace_symbol_body"]
        direction TB
        RB1["Input: symbol + file + body"] --> RB2["Find symbol node in AST"]
        RB2 --> RB3["Extract function boundaries"]
        RB3 --> RB4["Calculate indentation"]
        RB4 --> RB5["Replace line range"]
        RB5 --> RB6["Preserve trailing newline"]
        RB6 --> RB7["Write updated file"]
    end

    subgraph InsertAfter["code.insert_after_symbol"]
        direction TB
        IA1["Input: symbol + file + text"] --> IA2["Find symbol end_lineno"]
        IA2 --> IA3["Insert text after node"]
        IA3 --> IA4["Write updated file"]
    end

    classDef findClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef replaceClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef insertClass fill:#f3e5f5,stroke:#4a148c,stroke-width:2px

    class FS1,FS2,FS3,FS4,FS5,FS6,FS7,FS8,FS9,FS10 findClass
    class RB1,RB2,RB3,RB4,RB5,RB6,RB7 replaceClass
    class IA1,IA2,IA3,IA4 insertClass
```

---

## 5. OAuth 2.1 Authentication Flow

```mermaid
sequenceDiagram
    participant Client as Client Agent
    participant Simone as Simone MCP Server
    participant JWKS as JWKS Endpoint
    participant Issuer as OAuth Issuer

    Client->>Simone: GET /.well-known/oauth-client.json
    Simone-->>Client: redirect_uris + grant_types
    
    Client->>Simone: GET /.well-known/oauth-authorization-server
    Simone-->>Client: authorization_endpoint + token_endpoint
    
    Note over Client,Issuer: Authorization Code Flow with PKCE
    Client->>Issuer: Authorization Request + PKCE S256
    Issuer->>Client: Authorization Code
    
    Client->>Issuer: POST /token with code_verifier
    Issuer-->>Client: Access Token JWT
    
    Note over Client,Simone: Protected API Request
    Client->>Simone: POST /mcp with Bearer JWT
    Simone->>JWKS: Fetch signing key
    JWKS-->>Simone: Public Key
    Simone->>Simone: Verify JWT signature + audience + issuer
    Simone-->>Client: Authorized Response
```

---

## 6. Memory Integration Architecture

```mermaid
flowchart TB
    subgraph QueryLayer["Hybrid Memory Query"]
        Q1["memory.query Input"] --> Q2["Memory Facade"]
    end

    subgraph VectorLayer["Vector Search - Qdrant"]
        Q2 -->|QDRANT_URL set| V1["Generate embeddings"]
        V1 --> V2["Query Qdrant collections"]
        V2 --> V3["Return top-k chunks"]
    end

    subgraph GraphLayer["Graph Search - Neo4j"]
        Q2 -->|NEO4J_URI set| G1["Cypher query generation"]
        G1 --> G2["Query Neo4j graph"]
        G2 --> G3["Return related entities"]
    end

    subgraph FusionLayer["Result Fusion"]
        V3 --> F1["Merge and rank results"]
        G3 --> F1
        F1 --> F2["Return unified response"]
    end

    classDef queryClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef facadeClass fill:#fff9c4,stroke:#f57f17,stroke-width:2px
    classDef vectorClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef graphClass fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef fusionClass fill:#ffe0b2,stroke:#e65100,stroke-width:2px

    class Q1 queryClass
    class Q2 facadeClass
    class V1,V2,V3 vectorClass
    class G1,G2,G3 graphClass
    class F1,F2 fusionClass
```

---

## 7. Deployment Topology

### 7.1 Local Development

```mermaid
flowchart LR
    DEV["Developer Machine"] --> PYTHON["Python 3.12 venv"]
    PYTHON --> PIP["pip install -e"]
    PIP --> TEST["pytest tests -v"]
    TEST --> SERVE["python src/cli.py serve"]
    SERVE --> LOCAL["localhost:8234"]
    
    classDef devClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px

    class DEV,PYTHON,PIP,TEST,SERVE,LOCAL devClass
```

### 7.2 Docker Compose Stack

```mermaid
flowchart TB
    subgraph Docker["Docker Compose Stack"]
        subgraph Services["Services"]
            SIMONE["Simone MCP :8234"]
            QDRANT["Qdrant :6333"]
            NEO4J["Neo4j :7687"]
        end

        subgraph Volumes["Persistent Volumes"]
            QDATA[("qdrant_data")]
            NDATA[("neo4j_data")]
        end

        subgraph Network["Docker Network"]
            SIMONE_NET["simone bridge"]
        end
    end

    SIMONE -->|"http://qdrant:6333"| QDRANT
    SIMONE -->|"bolt://neo4j:7687"| NEO4J
    QDRANT --> QDATA
    NEO4J --> NDATA
    SIMONE --> SIMONE_NET
    QDRANT --> SIMONE_NET
    NEO4J --> SIMONE_NET

    classDef serviceClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef volumeClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef networkClass fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px

    class SIMONE,QDRANT,NEO4J serviceClass
    class QDATA,NDATA volumeClass
    class SIMONE_NET networkClass
```

### 7.3 Hugging Face Spaces Deployment

```mermaid
flowchart TB
    subgraph HF["Hugging Face Spaces"]
        SPACE["Space Runtime"]
        APP["Simone MCP App"]
    end

    subgraph External["External Persistent Storage"]
        SUPABASE[("Supabase DB")]
        REMOTE_QDRANT[("Remote Qdrant")]
        REMOTE_NEO4J[("Remote Neo4j")]
    end

    SPACE --> APP
    APP -->|"Environment Variables"| SUPABASE
    APP -->|"SIMONE_* env vars"| REMOTE_QDRANT
    APP -->|"NEO4J_URI env var"| REMOTE_NEO4J

    WARNING["No local disk persistence - Use external services"]
    APP -.-> WARNING

    classDef hfClass fill:#ffeb3b,stroke:#f57f17,stroke-width:2px
    classDef externalClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef warningClass fill:#ffebee,stroke:#c62828,stroke-width:2px,stroke-dasharray: 5 5

    class SPACE,APP hfClass
    class SUPABASE,REMOTE_QDRANT,REMOTE_NEO4J externalClass
    class WARNING warningClass
```

---

## 8. Tool Surface & Capabilities

```mermaid
mindmap
  root((Simone MCP Tools))
    Read-Only Tools
      code.find_symbol
      code.find_references
      code.project_overview
      memory.query
    Write Tools
      code.replace_symbol_body
      code.insert_after_symbol
    Meta Tools
      agent.help
      simone.mcp.health
    Capabilities
      memory.hybrid
      transport.streamable_http
      auth.oauth2.1
```

---

## 9. CI/CD Pipeline

```mermaid
flowchart LR
    subgraph DevPhase["Development"]
        CODE["Code Changes"] --> COMMIT["Git Commit"]
        COMMIT --> PUSH["Push to GitHub"]
    end

    subgraph CIPhase["CI Pipeline"]
        TRIGGER["GitHub Actions"] --> LINT["Lint + Type Check"]
        LINT --> TEST["pytest tests -v"]
        TEST --> CARD["print-card validation"]
        CARD --> HEALTHCHECK["health check"]
    end

    subgraph DeployPhase["Deployment"]
        BUILD["Docker Build"] --> PUSHIMG["Push to Registry"]
        PUSHIMG --> DEPLOYHF["Deploy to HF Spaces"]
        DEPLOYHF --> VERIFY["Health Verification"]
    end

    PUSH --> TRIGGER
    HEALTHCHECK --> BUILD

    classDef devClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef ciClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef deployClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px

    class CODE,COMMIT,PUSH devClass
    class TRIGGER,LINT,TEST,CARD,HEALTHCHECK ciClass
    class BUILD,PUSHIMG,DEPLOYHF,VERIFY deployClass
```

---

## 10. File Structure

```mermaid
flowchart TD
    ROOT["Simone-MCP"] --> SRC["src/"]
    ROOT --> TEST["tests/"]
    ROOT --> WELLDIR[".well-known/"]
    ROOT --> DOCSDIR["docs/"]
    ROOT --> ASSETSDIR["assets/"]
    ROOT --> CONF["Config Files"]

    SRC --> SIMONE_MCP["simone_mcp/"]
    SRC --> MAIN["main.py"]
    SRC --> CLIMAIN["cli.py"]
    SRC --> MCPSRV["mcp_server.py"]

    SIMONE_MCP --> COREFILE["core.py - Action Engine"]
    SIMONE_MCP --> HTTPFILE["http_app.py - FastAPI Server"]
    SIMONE_MCP --> STDIOFILE["mcp_stdio.py - stdio Server"]
    SIMONE_MCP --> CLIFILE["cli.py - CLI Handler"]
    SIMONE_MCP --> INITFILE["__init__.py"]

    CONF --> PYPROJECT["pyproject.toml"]
    CONF --> DOCKERFILE["Dockerfile"]
    CONF --> COMPOSE["docker-compose.yml"]
    CONF --> ENVFILE[".env.example"]
    CONF --> MCPCFG["mcp-config.json"]
    CONF --> AGENTFILE["agent.json"]
    CONF --> CARDFILE["A2A-CARD.md"]

    classDef dirClass fill:#e3f2fd,stroke:#1565c0,stroke-width:2px
    classDef pyClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef cfgClass fill:#fff3e0,stroke:#e65100,stroke-width:2px

    class ROOT,SRC,TEST,WELLDIR,DOCSDIR,ASSETSDIR,CONF,SIMONE_MCP dirClass
    class COREFILE,HTTPFILE,STDIOFILE,CLIFILE,INITFILE,MAIN,CLIMAIN,MCPSRV pyClass
    class PYPROJECT,DOCKERFILE,COMPOSE,ENVFILE,MCPCFG,AGENTFILE,CARDFILE cfgClass
```

---

## 11. Security Architecture

```mermaid
flowchart TB
    subgraph RequestPhase["Incoming Request"]
        REQ["HTTP Request"] --> ORIGIN["Origin Validation"]
    end

    subgraph AuthPhase["Authentication Layer"]
        ORIGIN -->|"Origin allowed?"| AUTHCHECK{"Auth Required?"}
        AUTHCHECK -->|No| PASS["Proceed"]
        AUTHCHECK -->|Yes| BEARERCHECK{"Bearer Token?"}
        BEARERCHECK -->|No| ERRUNAUTH["401 Unauthorized"]
        BEARERCHECK -->|Yes| JWTVAL["JWT Validation"]
        JWTVAL --> JWKS["Fetch JWKS Key"]
        JWKS --> VERIFY{"Valid Signature?"}
        VERIFY -->|No| ERRINVALID["401 Invalid Token"]
        VERIFY -->|Yes| AUDCHECK{"Audience Match?"}
        AUDCHECK -->|No| ERRAUD["401 Invalid Audience"]
        AUDCHECK -->|Yes| PASS
    end

    subgraph OpenEndpoints["Open Endpoints"]
        OPENLIST["/health /dashboard /.well-known"]
    end

    REQ -.->|"Skip auth"| OPENLIST

    classDef requestClass fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef authClass fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef openClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px
    classDef errorClass fill:#ffebee,stroke:#c62828,stroke-width:2px

    class REQ,ORIGIN requestClass
    class AUTHCHECK,BEARERCHECK,JWTVAL,JWKS,VERIFY,AUDCHECK,PASS authClass
    class OPENLIST openClass
    class ERRUNAUTH,ERRINVALID,ERRAUD errorClass
```

---

## 12. Agent Card & Discovery

```mermaid
flowchart LR
    subgraph WellKnown[".well-known Endpoints"]
        direction TB
        CARD["agent-card.json"]
        AGENTFILE["agent.json"]
        OAUTHCLIENT["oauth-client.json"]
        OAUTHSERVER["oauth-authorization-server"]
    end

    subgraph CardContent["Agent Card Content"]
        direction TB
        NAMEFIELD["name: simone-mcp"]
        VERSIONFIELD["version: 2026.04.12"]
        CAPSFIELD["capabilities array"]
        ENDPOINTSFIELD["endpoints object"]
        SKILLSFIELD["skills array"]
        AUTHFIELD["auth: oauth2.1"]
    end

    CARD --> NAMEFIELD
    CARD --> VERSIONFIELD
    CARD --> CAPSFIELD
    CARD --> ENDPOINTSFIELD
    CARD --> SKILLSFIELD
    CARD --> AUTHFIELD

    classDef wkClass fill:#f3e5f5,stroke:#7b1fa2,stroke-width:2px
    classDef contentClass fill:#e8f5e9,stroke:#2e7d32,stroke-width:2px

    class CARD,AGENTFILE,OAUTHCLIENT,OAUTHSERVER wkClass
    class NAMEFIELD,VERSIONFIELD,CAPSFIELD,ENDPOINTSFIELD,SKILLSFIELD,AUTHFIELD contentClass
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
