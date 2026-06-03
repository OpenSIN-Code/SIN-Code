# memory.py

Persistent memory layer with three stacked backends:

1. **SQLite** — always-on durable store (facts, tags, context).
2. **SCKG** — optional code-knowledge graph (`sin_code_sckg`).
3. **Honcho** — optional behavioral-memory layer (`honcho-ai`).

Inspired by the retain / recall / reflect patterns from Hindsight and
Letta. SCKG and Honcho are both *optional* dependencies and gracefully
degrade to no-ops when the package is missing or the server is
unreachable. SQLite is the durable source of truth.

## Dependencies

- stdlib: `sqlite3`, `json`, `datetime`, `pathlib`
- optional: `sin_code_sckg` (`KnowledgeGraph` with `add_node`)
- optional: `honcho-ai` 2.1+ (`Honcho`, `Honcho.peer`, `Honcho.session`)

## Touched by

- (none yet — this is a leaf module exposed for agent use)

## What it does

1. **`SINMemory(repo_root, db_path, honcho_workspace, honcho_base_url)`**
   — opens a SQLite DB at `<repo_root>/.sin_memory.db` (overridable),
   then *attempts* to construct a `sin_code_sckg.KnowledgeGraph` rooted
   at `repo_root`, then attaches a `HonchoBackend`. All optional
   backends degrade silently on failure.
2. **`retain(fact, context, tags)`** — inserts a row into
   `memories(id, fact, context, tags, created_at)`, then best-effort
   adds a `memory:<id>` node to SCKG. SQLite is the source of truth.
3. **`recall(query, limit, tags)`** — `LIKE %query%` search,
   AND-combined over the supplied `tags` list, ordered newest-first.
4. **`reflect(query, context)`** — convenience wrapper that
   concatenates the top-5 recall hits into a single answer with a
   fixed 0.5 confidence and a hint to use an LLM for real synthesis.
5. **`forget(memory_id)`** — deletes a single row by id, returns
   whether a row was actually removed.
6. **`get_context_for_query(query)`** — unified context retrieval
   that fans out to SCKG (code knowledge) and Honcho (behavioral
   insights) and produces a single `synthesis` string suitable for
   LLM prompt injection. Always returns a well-formed dict.
7. **`get_stats()`** — returns `{total_facts, tags, backend, honcho}`
   where `backend` is `"SQLite + SCKG"` or `"SQLite only"`, and
   `honcho` is the `HonchoBackend.get_status()` payload.

## Honcho Backend (Behavioral Memory)

Honcho is a **behavioral memory** service — it remembers things about
*peers* (users, agents) and their *interactions*: preferences, tone,
recurring mistakes, and peer-specific context. It is the
*complementary* half of the memory stack:

| | SCKG (Code) | Honcho (Behavioral) |
|---|---|---|
| **Stores** | Module / File / Function graph | Conversations, preferences, peer-models |
| **Use** | "Which modules are affected?" | "How does this user react to errors?" |
| **Granularity** | Code | Session, peer |

`HonchoBackend` exposes:

- `is_available()` — cheap, cached connectivity check.
- `get_status()` — `{available, workspace_id, base_url, error}`.
- `get_or_create_peer(name)` / `get_or_create_session(name)`.
- `retain_message(peer, content, role, session, metadata)` — stores a
  message; optional session attachment.
- `get_session_context(name)` — dialectic context for a session.
- `chat(peer, query)` — ask the peer a question (dialectic).
- `search(query, peer_name=...)` — semantic search across memory.

### Graceful degradation

Honcho is **optional** in every direction:

- If `honcho-ai` is not installed → `is_available()` returns `False`
  and every other method returns `None` / `[]` / `{"error": ...}`.
- If the server is unreachable → same behaviour; the failure is
  cached in `_init_error` and surfaced via `get_status()`.
- A 2-second `timeout` is the default; we never want a Honcho outage
  to block `retain` / `recall` / agent timeouts.
- `SINMemory` is fully usable with no Honcho at all — `retain` still
  writes to SQLite, and `get_context_for_query` returns
  `{"synthesis": "No context available.", ...}`.

### Setting up the Honcho server

```bash
# Install (already a transitive dep in many envs)
pip install honcho-ai

# Start the local server (default: http://localhost:8000)
honcho serve

# Or set a custom URL
export HONCHO_BASE_URL="https://honcho.example.com"
```

If you don't run the server, `HonchoBackend` will detect the
connection error on first use and silently turn into a no-op.

### Using the combined stack

```python
from pathlib import Path
from sin_code_bundle.memory import SINMemory

mem = SINMemory(
    Path("/path/to/repo"),
    honcho_workspace="my-team",       # optional, defaults to f"sin-bundle-{repo_name}"
    honcho_base_url="http://localhost:8000",
)

# Backend status
print(mem.get_stats())
# → {"total_facts": 0, "tags": [], "backend": "SQLite + SCKG", "honcho": {...}}

# Unified context (SCKG + Honcho + SQLite-synthesis)
ctx = mem.get_context_for_query("How should we handle auth errors?")
print(ctx["synthesis"])
print(ctx["backends"])  # {"sqlite": True, "sckg": True, "honcho": False}
```

## Important config

- `db_path` — defaults to `<repo_root>/.sin_memory.db`. Pass an
  explicit path to keep multiple memory stores side-by-side.
- `repo_root` — defaults to `Path.cwd()`. Used as the SCKG root and
  as the parent for the default DB path.
- `honcho_workspace` — defaults to `f"sin-bundle-{repo_root.name}".
  Distinct workspaces isolate peer/session namespaces in Honcho.
- `honcho_base_url` — defaults to `http://localhost:8000`. Honcho is
  treated as a local sidecar in dev.
- `tags` in `retain` — stored as a comma-joined string, so a tag
  cannot contain a comma. `recall(tags=[...])` uses
  `LIKE '%tag%'` per tag (AND-combined), so substring collisions
  between tags are possible — keep tag names short and atomic.

## Usage

```python
from pathlib import Path
from sin_code_bundle.memory import SINMemory, HonchoBackend

# Full stack (SCKG + Honcho, if both are installed/reachable)
mem = SINMemory(Path("/path/to/repo"))
result = mem.retain("User prefers TypeScript", tags=["preference"])
print(result["id"], result["stored_in"])  # → "SQLite only" or "SQLite + SCKG"

hits = mem.recall("typescript", limit=5)
for h in hits:
    print(h["id"], h["fact"], h["tags"])

print(mem.reflect("typescript")["confidence"])  # 0.0 or 0.5
print(mem.get_stats())

# Honcho-only usage (rarely needed; usually go through SINMemory)
hb = HonchoBackend(workspace_id="my-ws")
if hb.is_available():
    hb.retain_message("coding-agent", "Likes small PRs", role="user")
    print(hb.chat("coding-agent", "What does the user prefer?"))
```

## Known caveats

- Search is **LIKE-based**, not semantic — substring matches only.
  SCKG integration exists for *graph* queries, not for vector search.
- Honcho peer models are *evolving* — early sessions yield thin
  insights until the peer has seen enough interactions. Don't
  treat `behavioral_insights` as authoritative on day one.
- No schema migrations yet — adding columns requires manual ALTER.
- Reflect is intentionally dumb; a real synthesizer (LLM) should
  replace it before relying on `reflect()` for decisions.
- SQLite writes are not WAL-mode here; high-frequency retains from
  many threads should switch to WAL or move to a real server.
- Honcho is a **sidecar**, not a source of truth. If the server is
  wiped, behavioral insights are lost; durable facts live in SQLite.
