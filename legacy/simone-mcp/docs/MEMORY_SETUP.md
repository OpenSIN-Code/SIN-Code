# Simone Hybrid Memory — local production setup

Simone ships its **own** hybrid memory (see `src/simone_mcp/hybrid_memory.py`):

- **Vectors** → Qdrant (semantic search)
- **Graph** → Neo4j (structural / symbol relations)
- **Fallback** → local SQLite under `~/.simone/` when neither is configured

> It does **not** need claude-mem or sin-brain. Those are separate memory
> systems; wiring them in would duplicate what Simone already does. The
> CEO-pro move is to arm Simone's *native* backend, which is what this doc
> describes.

## What "armed" means

The backend is **live** only when *all three* hold:

1. `QDRANT_URL` + `NEO4J_URI` + `NEO4J_USER` + `NEO4J_PASSWORD` are in the
   server process's environment.
2. The Qdrant + Neo4j services are reachable.
3. The Python client libs `qdrant-client` and `neo4j` are importable by the
   **same interpreter** that runs the server (`/opt/homebrew/bin/python3`).

If any is missing, `hybrid_memory.py` silently catches the failure and falls
back to SQLite — the API still returns `vectorStore: "<url>"`, so **don't
trust that field alone**; verify with a live client connection (below).

## One-time setup (done 2026-07-19)

```bash
cd ~/.local/share/sin-code/skills/Simone-MCP

# 1) Local secrets (git-ignored, chmod 600). NEO4J_PASSWORD is generated.
cat .env    # QDRANT_URL, NEO4J_URI, NEO4J_USER, NEO4J_PASSWORD

# 2) Bring up ONLY the two databases (NOT the compose simone-mcp service —
#    launchd already owns :8234). Containers get restart=unless-stopped.
docker-compose up -d qdrant neo4j
docker update --restart unless-stopped simone-mcp-qdrant-1 simone-mcp-neo4j-1

# 3) Client libs into the interpreter launchd uses:
/opt/homebrew/bin/python3 -m pip install --break-system-packages \
  'qdrant-client>=1.9.0' 'neo4j>=5.24.0'
```

The launchd job `~/Library/LaunchAgents/ai.opensin.simone-mcp.plist` carries the
four env vars under `EnvironmentVariables` (plist is `chmod 600` — it holds the
Neo4j password). Reload after editing:

```bash
launchctl bootout   "gui/$(id -u)/ai.opensin.simone-mcp" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" ~/Library/LaunchAgents/ai.opensin.simone-mcp.plist
launchctl kickstart -k "gui/$(id -u)/ai.opensin.simone-mcp"
```

## Verify it's really hybrid (not SQLite)

```bash
set -a; . ./.env; set +a
/opt/homebrew/bin/python3 - <<'PY'
import os
from qdrant_client import QdrantClient
print("QDRANT:", [c.name for c in QdrantClient(url=os.environ["QDRANT_URL"]).get_collections().collections])
from neo4j import GraphDatabase
d = GraphDatabase.driver(os.environ["NEO4J_URI"], auth=(os.environ["NEO4J_USER"], os.environ["NEO4J_PASSWORD"]))
with d.session() as s: print("NEO4J RETURN 1 =>", s.run("RETURN 1 AS ok").single()["ok"])
d.close()
PY
```

Both lines returning cleanly = backend armed. Endpoints:
Qdrant `http://localhost:6333` · Neo4j browser `http://localhost:7474`
(bolt `bolt://localhost:7687`) · Simone `http://localhost:8234/dashboard`.

## Boot persistence

This Mac runs **OrbStack** as the container runtime (Docker Desktop is not used —
`docker context show` → `orbstack`).

- **DBs**: `restart=unless-stopped` → restart with the OrbStack engine.
- **Simone**: launchd `KeepAlive` → self-heals.
- **Runtime**: OrbStack has `app.start_at_login: true` (verify with
  `orb config show | grep start_at_login`), so the engine — and therefore the
  DBs — come back automatically after a reboot. No manual step required. If that
  flag is ever off, Simone silently degrades to SQLite until OrbStack is up.
