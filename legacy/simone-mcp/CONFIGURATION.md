# Simone MCP — Configuration

*Reference for all environment variables, configuration options, and architecture.*

---

## 1. Environment Variables

### 1.1 Memory & Storage

| Variable | Default | Beschreibung |
|---|---|---|
| `QDRANT_URL` | `""` | Qdrant HTTP URL (z.B. `http://qdrant:6333`). Wenn leer → lokaler SQLite-Store |
| `NEO4J_URI` | `""` | Neo4j Bolt URI (z.B. `bolt://neo4j:7687`). Wenn leer → lokaler SQLite-Store |
| `NEO4J_USER` | `neo4j` | Neo4j-Benutzer |
| `NEO4J_PASSWORD` | `""` | Neo4j-Passwort |
| `SIMONE_MEMORY_DIR` | `~/.simone` | Lokaler SQLite-Speicherort (Memory-DBs in Unterordnern) |
| `LOCAL_EMBEDDING_MODEL` | `all-MiniLM-L6-v2` | Sentence-Transformer Modell für lokale Embeddings |
| `QDRANT_EMBEDDING_MODEL` | `""` | Sentence-Transformer Modell für Qdrant-Embeddings |

### 1.2 Authentication (OAuth 2.1)

| Variable | Default | Beschreibung |
|---|---|---|
| `SIMONE_AUTH_REQUIRED` | `false` | OAuth 2.1 erforderlich (`true/false`) |
| `SIMONE_OAUTH_ISSUER` | `""` | OAuth-Issuer (z.B. `https://accounts.google.com`) |
| `SIMONE_OAUTH_AUDIENCE` | `simone-mcp` | OAuth-Audience |
| `SIMONE_OAUTH_JWKS_URL` | `""` | JWKS-URL für Token-Validierung |
| `SIMONE_OAUTH_ALGORITHMS` | `RS256,ES256` | Erlaubte Signatur-Algorithmen |

### 1.3 Rate Limiting

| Variable | Default | Beschreibung |
|---|---|---|
| `SIMONE_RATE_LIMIT_WINDOW` | `60` | Rate-Limit Fenster in Sekunden |
| `SIMONE_RATE_LIMIT_MAX` | `100` | Maximale Requests pro Fenster |

### 1.4 CORS

| Variable | Default | Beschreibung |
|---|---|---|
| `SIMONE_ALLOWED_ORIGINS` | `http://localhost,http://127.0.0.1,https://opensin.ai` | Erlaubte CORS-Origins |

### 1.5 Supabase (optional)

| Variable | Default | Beschreibung |
|---|---|---|
| `SUPABASE_URL` | `""` | Supabase Project URL |
| `SUPABASE_ANON_KEY` | `""` | Supabase Anon Key |
| `SUPABASE_SERVICE_ROLE_KEY` | `""` | Supabase Service Role Key |

### 1.6 Observability (OpenTelemetry)

| Variable | Default | Beschreibung |
|---|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `""` | OTLP-Endpunkt |
| `OTEL_SERVICE_NAME` | `simone-mcp` | Service-Name für Traces |

### 1.7 Application

| Variable | Default | Beschreibung |
|---|---|---|
| `LOG_LEVEL` | `INFO` | Log-Level |
| `SIMONE_MAX_REQUEST_BODY` | `1048576` | Maximale Request-Body-Größe (Bytes) |

---

## 2. Architecture

```
┌─────────────────────────────────────────────────┐
│                  Client                         │
│  (OpenCode / CLI / HTTP)                       │
└────────────┬────────────────────────┬───────────┘
             │ MCP (stdio/HTTP)       │ A2A
             ▼                        ▼
┌───────────────────────┐  ┌──────────────────┐
│    mcp_stdio.py       │  │   a2a_handler.py │
│    protocol.py        │  │                  │
└──────────┬────────────┘  └──────────────────┘
           │
           ▼
┌───────────────────────┐
│      core.py          │
│  execute_simone_action│
└──────────┬────────────┘
           │
     ┌─────┴──────┐
     ▼            ▼
┌─────────┐ ┌────────────┐
│ Symbol  │ │  Hybrid    │
│ Search  │ │  Memory    │
│(ast/jedi│ │            │
│libcst/  │ │ ┌────────┐ │
│treesit) │ │ │Local   │ │
│         │ │ │SQLite  │ │
│ Find    │ │ │(always)│ │
│ Refs    │ │ ├────────┤ │
│ Edit    │ │ │Qdrant  │ │
│         │ │ │(opt)   │ │
│ Overview│ │ ├────────┤ │
│         │ │ │Neo4j   │ │
│         │ │ │(opt)   │ │
└─────────┘ └────────────┘
```

### Memory-Fallback-Strategie

```
query_hybrid_memory(payload)
  ├─ QDRANT_URL gesetzt? → _query_qdrant()
  │   └─ Fehler? → _query_local_semantic()   [SQLite Fallback]
  ├─ KEIN Qdrant → _query_local_semantic()   [SQLite direkt]
  ├─ NEO4J gesetzt + target_symbol? → _query_neo4j()
  │   └─ Fehler? → _query_local_graph()      [SQLite Fallback]
  └─ KEIN Neo4j → _query_local_graph()       [SQLite direkt]
```

**Immer enabled:** `enabled: True` wird in allen Modi zurückgegeben.

---

## 3. MCP Tool Definitions

| Tool Name | Beschreibung | ReadOnly |
|---|---|---|
| `sin_simone_mcp_health` | Status + Version + Memory-Enabled | ✅ |
| `sin_simone_mcp_symbol_search` | LSP-Symbolsuche (AST/Jedi/LibCST/TreeSitter) | ✅ |
| `sin_simone_mcp_structural_edit` | Strukturelle Code-Editierung (LibCST/AST) | ❌ |
| `sin_simone_mcp_memory_query` | Semantische + strukturelle Memory-Suche | ✅ |
| `sin_simone_mcp_find_references` | Referenzsuche (Jedi/Regex) | ✅ |
| `sin_simone_mcp_project_overview` | Workspace-Footprint + Dateitypen | ✅ |

---

## 4. Local Memory Store (SQLite)

Der lokale Memory-Store speichert:

### vectors (Semantische Suche)

```sql
CREATE TABLE vectors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    collection TEXT NOT NULL DEFAULT 'default',
    file TEXT NOT NULL DEFAULT '',
    symbol TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    embedding BLOB,
    created_at REAL NOT NULL DEFAULT (unixepoch())
);
```

### symbols (Graph-Struktur)

```sql
CREATE TABLE symbols (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'unknown',
    file TEXT NOT NULL DEFAULT '',
    line INTEGER DEFAULT 0,
    created_at REAL NOT NULL DEFAULT (unixepoch())
);
```

### symbol_relations (Graph-Kanten)

```sql
CREATE TABLE symbol_relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_id INTEGER NOT NULL,
    target_id INTEGER NOT NULL,
    rel_type TEXT NOT NULL DEFAULT 'references',
    created_at REAL NOT NULL DEFAULT (unixepoch())
);
```

**Speicherort:** `~/.simone/<db_name>/memory.db` (konfigurierbar via `SIMONE_MEMORY_DIR`)

**Embeddings:** Sentence-Transformer (`all-MiniLM-L6-v2`) oder SHA-256 Fallback.
**Similarity:** Cosine Similarity, Threshold ≥ 0.3.

---

## 5. Docker Production Setup

```bash
# .env.example → .env kopieren + anpassen
cp .env.example .env

# Alle Services starten
docker compose up -d

# Status prüfen
docker compose ps
# NAME                    STATUS
# simone-mcp              Up
# qdrant                  Up
# neo4j                   Up
```

### Ports

| Service | Port | Protokoll |
|---|---|---|
| Simone MCP HTTP | 8234 | HTTP (MCP + A2A) |
| Qdrant | 6333 | HTTP |
| Neo4j | 7474 | HTTP (Browser) |
| Neo4j | 7687 | Bolt (Client) |

---

## 6. OAuth 2.1

```
SIMONE_AUTH_REQUIRED=true
SIMONE_OAUTH_ISSUER=https://accounts.google.com
SIMONE_OAUTH_AUDIENCE=simone-mcp
SIMONE_OAUTH_JWKS_URL=https://www.googleapis.com/oauth2/v3/certs
```

Wenn `SIMONE_AUTH_REQUIRED=false` (default): kein Auth erforderlich.
Alle öffentlichen Pfade (`/health`, `/dashboard`, `/.well-known/*`) sind immer frei.

---

## 7. Troubleshooting

### Memory: `enabled: false`

**Seit V0.1.0 (lokaler SQLite-Store) ist memory IMMER enabled.**

Falls trotzdem `false`:
1. Alte Code-Version? → `git pull`
2. Python-Pfad korrekt? → `PYTHONPATH=src` setzen
3. Schreibrechte auf `~/.simone`? → `ls -la ~/.simone`

### MCP-Tools nicht sichtbar

1. Simone MCP in OpenCode aktiviert? → `"enabled": true` in opencode.json
2. Python3 vorhanden? → `which python3`
3. Neustart von OpenCode nötig

### Qdrant/Neo4j Verbindung fehlschlägt

Docker nicht gestartet? → `docker compose up -d qdrant neo4j`
Oder: lokalen Modus nutzen — env vars einfach leer lassen.

### Embedding-Qualität schlecht

`pip install sentence-transformers` fehlt? Ohne dieses Paket werden SHA-256-basierte
Embeddings genutzt — funktional aber semantisch schwach.

---

*Stand: 2026-05-30 | v0.1.0 | Delqhi/Simone-MCP*
