# memory.py

Persistent memory layer with a best-effort **SCKG** semantic backend and
a durable **SQLite** store. Inspired by the retain / recall / reflect
patterns from Hindsight and Letta. SCKG is an *optional* dependency;
SQLite is always available.

## Dependencies

- stdlib: `sqlite3`, `json`, `datetime`, `pathlib`
- optional: `sin_code_sckg` (`KnowledgeGraph` with `add_node`)

## Touched by

- (none yet — this is a leaf module exposed for agent use)

## What it does

1. **`SINMemory(repo_root, db_path)`** — opens a SQLite DB at
   `<repo_root>/.sin_memory.db` (overridable), then *attempts* to
   construct a `sin_code_sckg.KnowledgeGraph` rooted at `repo_root`.
   If SCKG is missing or its init throws, the instance still works
   in SQLite-only mode.
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
6. **`get_stats()`** — returns `{total_facts, tags, backend}` where
   `backend` is `"SQLite + SCKG"` or `"SQLite only"`.

## Important config

- `db_path` — defaults to `<repo_root>/.sin_memory.db`. Pass an
  explicit path to keep multiple memory stores side-by-side.
- `repo_root` — defaults to `Path.cwd()`. Used as the SCKG root and
  as the parent for the default DB path.
- `tags` in `retain` — stored as a comma-joined string, so a tag
  cannot contain a comma. `recall(tags=[...])` uses
  `LIKE '%tag%'` per tag (AND-combined), so substring collisions
  between tags are possible — keep tag names short and atomic.

## Usage

```python
from pathlib import Path
from sin_code_bundle.memory import SINMemory

mem = SINMemory(Path("/path/to/repo"))
result = mem.retain("User prefers TypeScript", tags=["preference"])
print(result["id"], result["stored_in"])  # → "SQLite only" if no SCKG

hits = mem.recall("typescript", limit=5)
for h in hits:
    print(h["id"], h["fact"], h["tags"])

print(mem.reflect("typescript")["confidence"])  # 0.0 or 0.5
print(mem.get_stats())
```

## Known caveats

- Search is **LIKE-based**, not semantic — substring matches only.
  SCKG integration exists for *graph* queries, not for vector search.
- No schema migrations yet — adding columns requires manual ALTER.
- Reflect is intentionally dumb; a real synthesizer (LLM) should
  replace it before relying on `reflect()` for decisions.
- SQLite writes are not WAL-mode here; high-frequency retains from
  many threads should switch to WAL or move to a real server.
