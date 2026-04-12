---
title: Simone MCP
emoji: "🤖"
colorFrom: blue
colorTo: indigo
sdk: docker
app_port: 8234
pinned: false
---

# A2A-SIN-Simone-MCP

> **Der weltbeste Coding-MCP-Server (Stand 2026)** – LSP-Power von Serena MCP, aber mit A2A-Native, Async-First und Cloud-Memory.

## 🚀 Warum Simone MCP? (vs. Serena MCP)

Simone MCP basiert auf der genialen LSP-Logik von [Serena MCP](https://github.com/oraios/serena), löst aber dessen kritische Architektur-Schwächen für den Enterprise-Einsatz 2026.

| Feature | Serena MCP (Status Quo) | **Simone MCP (Unser Design)** |
| :--- | :--- | :--- |
| **Dashboard** | ⚠️ Basic CLI / Web GUI | ✅ **Enterprise-Dashboard** (`activate_simone`, `a2a.delqhi.com/agents/simone-mcp`) |
| **A2A Discovery** | ❌ Keine `agent-card.json` | ✅ **A2A-Native** (wird von anderen Agenten auto-discovered) |
| **Speicher** | ⚠️ Lokal (`.serena/memories/`) bricht bei Größe ein | ✅ **Cloud (PostgreSQL/pgvector)** skaliert unendlich |
| **Kommunikation** | ⚠️ Synchron (blockiert den Agenten) | ✅ **Asynchron** (Supabase Event Bus, Agent wartet nicht) |
| **Deployment** | ⚠️ Manuell (Docker/uv) | ✅ **Live HF Space** (`https://delqhi-simone-mcp.hf.space`) + Keep-Alive, Auto-Restart |
| **Security** | ⚠️ Basic | ✅ **Token-Scoped + Policy Gates** |
| **Memory** | Local file-based | ✅ Cloud semantic memory (pgvector) |
| **Integration** | Standalone | ✅ Native A2A Fleet Member |

**Fazit:** Simone MCP ist **Serena MCP auf Steroiden** – gleiche LSP-Power, aber enterprise-ready.

---

## Quick Start

```bash
# Lokalen Server starten
python -m src.mcp_server

# Dashboard öffnen (öffnet sich automatisch im Browser)
activate_simone

# Health check
curl http://localhost:8234/health
```

## Live Surfaces

- Public A2A landing: `https://a2a.delqhi.com/agents/simone-mcp`
- HF runtime: `https://delqhi-simone-mcp.hf.space`

## Was ist Simone MCP?

Simone MCP ist ein spezialisierter **Code Worker Agent**, der bietet:

- **LSP Symbol-Operationen:** Find symbols, references, structural edits
- **A2A Native:** Vollständige A2A-Protokollunterstützung mit Discovery
- **Async Event-Driven:** Supabase Realtime Integration
- **Cloud Memory:** PostgreSQL/pgvector für semantischen Code-Speicher

## Architektur

Siehe [ARCHITECTURE.md](ARCHITECTURE.md) für detailliertes Design.

## Entwicklung

```bash
# Tests ausführen
pytest tests/

# Für Deployment bauen
npm run build

# Auf HF Space deployen
huggingface-cli upload simone-mcp ./dist . --repo-type=space
```

## Packaging

- `pyproject.toml` for metadata and build config
- `requirements.lock.txt` for pinned deployment dependencies
- `Dockerfile` for container / HF Space runtime

## Verwandte Issues

- Issue #311: Simone MCP erstellen
- Issue #312: Serena MCP Gap-Analyse
- [Serena MCP](https://github.com/oraios/serena) - Upstream LSP-Integration
