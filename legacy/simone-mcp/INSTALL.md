# Simone MCP — Installation

*Production-grade MCP 2.0 code worker with symbol operations, hybrid memory (SQLite
local → Qdrant/Neo4j optional), streamable HTTP transport, and OAuth 2.1 readiness.*

---

## 1. Voraussetzungen

| Abhängigkeit | Version | Prüfung |
|---|---|---|
| Python | ≥ 3.12 | `python3 --version` |
| pip | aktuell | `python3 -m pip --version` |
| Git | aktuell | `git --version` |

**Optional (Docker-Modus für Produktion):**

```bash
docker --version
# ✅ "Docker version ..."
# ❌ → Docker Desktop installieren: https://docs.docker.com/desktop/
```

---

## 2. Repository klonen

```bash
git clone git@github.com:OpenSIN-Code/Simone-MCP.git
cd Simone-MCP
```

---

## 3. Installation + Auto-Migration (Einzeiler)

```bash
./install.sh
```

Das installiert alle Abhängigkeiten und führt `scripts/migrate-opencode.py` aus.
Der Migrator scannt `~/.config/opencode/AGENTS.md`, `**/AGENTS.md` und `**/opencode.json`
und ersetzt automatisch alle grep/find/edit Referenzen durch Simone MCP Tools.

### 3.1 Manuelle Installation

```bash
python3 -m venv .venv
source .venv/bin/activate
pip install -e .
```

### 3.2 OpenCode Integration (manuell)

```bash
# AGENTS.md patchen + opencode.json MCP Server + Permissions
python3 scripts/migrate-opencode.py
```

Danach OpenCode neustarten — Simone MCP Tools sind automatisch verfügbar.

---

## 4. Verifikation

### 4.1 Memory-Status prüfen

```bash
cd ~/dev/Simone-MCP
PYTHONPATH=src python3 -c "
from simone_mcp.hybrid_memory import query_hybrid_memory, get_local_stats
r = query_hybrid_memory({'query': 'test'})
print(f'Enabled: {r[\"enabled\"]}')
print(f'VectorStore: {r[\"vectorStore\"]}')
print(f'GraphStore: {r[\"graphStore\"]}')
stats = get_local_stats()
print(f'Vectors: {stats[\"vectors\"]}')
print(f'Symbols: {stats[\"symbols\"]}')
"
# ✅ Enabled: True
# ✅ VectorStore: local
# ✅ GraphStore: local
```

### 4.2 MCP-Tools testen (OpenCode)

Starte OpenCode und rufe die Simone-MCP Tools auf:

- `sin_simone_mcp_health` → Prüft Status + Memory-Enabled
- `sin_simone_mcp_memory_query` → Query mit `{"query": "was ist im memory?"}`
- `sin_simone_mcp_symbol_search` → Suche Symbol im Workspace
- `sin_simone_mcp_project_overview` → Workspace-Übersicht

---

## 5. Docker-Modus (Produktion)

Für Produktion mit Qdrant (Vektor-DB) + Neo4j (Graph-DB):

### 5.1 Docker starten

```bash
docker compose up -d qdrant neo4j
# ✅ Container running
```

### 5.2 .env konfigurieren

```bash
cp .env.example .env
# QDRANT_URL=http://qdrant:6333
# NEO4J_URI=bolt://neo4j:7687
# NEO4J_USER=neo4j
# NEO4J_PASSWORD=change_me_in_prod
```

### 5.3 Simone MCP im Docker starten

```bash
docker compose up -d simone-mcp
curl http://localhost:8234/health
# ✅ {"ok": true, "status": "ok", "name": "simone-mcp"}
```

### 5.4 HTTP-MCP Endpoint

```
POST http://localhost:8234/mcp
Content-Type: application/json
```

---

## 6. Deinstallation

```bash
# Docker stoppen
docker compose down -v

# Lokale Memory-DB löschen
rm -rf ~/.simone

# Repository löschen
rm -rf ~/dev/Simone-MCP
```

---

*Stand: 2026-05-30 | v0.1.0 | Delqhi/Simone-MCP*
*Hybrid Memory: SQLite (lokal, immer aktiv) → Qdrant/Neo4j (Docker, optional)*
