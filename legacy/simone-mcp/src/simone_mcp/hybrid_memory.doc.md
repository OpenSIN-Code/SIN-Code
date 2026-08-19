# `hybrid_memory.py` — Hybrid Memory Backend

What this file does: the "hybrid" memory layer — vector (Qdrant) + graph (Neo4j) when configured, falling back to a local SQLite store with the same shape. Powers the `sin_simone_mcp_memory_query` tool and the local knowledge graph.

## Dependency map

- Imports: stdlib (`json`, `logging`, `math`, `os`, `sqlite3`, `threading`, `time`, `pathlib`).
- Optional deps (lazy): `qdrant_client`, `neo4j`, `sentence_transformers`.
- Imported by: `core.py` (`query_hybrid_memory`), `http_app.py` (`shutdown_stores`).

## Public API

| Function                            | Purpose                                                          |
|-------------------------------------|------------------------------------------------------------------|
| `query_hybrid_memory(payload)`      | Single semantic + structural query; returns both result lists    |
| `store_symbol(name, kind, file, line, db_name?)` | Insert a symbol; no-op if `(name, file)` exists  |
| `store_relation(source_id, target_id, rel_type, db_name?)` | Insert an edge                              |
| `store_vector(text, collection, file, symbol, db_name?)` | Embed + insert a vector                 |
| `get_local_stats(db_name?)`         | Counts of vectors / symbols / relations in the local store        |
| `shutdown_stores()`                  | Close all backend connections                                    |

## Important config / limits

- **Local store path**: `$SIMONE_MEMORY_DIR` or `~/.simone/<subdir>/memory.db` by default.
- **SQLite in WAL mode** with `synchronous=OFF` — fast, but a hard crash can lose the last few transactions.
- **Embedding model**: `LOCAL_EMBEDDING_MODEL` (default `all-MiniLM-L6-v2`, 384 dims). Falls back to a SHA-256 pseudo-vector (rank 64) if `sentence-transformers` is missing.
- **Neo4j driver TTL: 1 hour** (env: not configurable; hardcoded in `_get_neo4j_driver`).
- **Qdrant client timeout: 5s** (env: not configurable; hardcoded).

## Design decisions

- **Why hybrid in the same module?** Both backends have the same shape (semantic + structural), so a single entry point can dispatch to either. Callers don't need to know which backend served their query.
- **Why SHA-256 fallback for embeddings?** A meaningful 64-dim vector is still better than no vector at all. The warning is loud so users notice.
- **Why lazy-load Neo4j / Qdrant / sentence-transformers?** They're heavy imports. Caches under locks ensure one-load-per-process.
- **Why WAL mode for SQLite?** Multiple FastAPI worker threads can read concurrently without blocking on a write. Tradeoff: a small window of uncommitted data on hard crash.

## Usage example

```python
from simone_mcp.hybrid_memory import query_hybrid_memory

result = query_hybrid_memory({
    "query": "where is auth handled?",
    "target_symbol": "authenticate",
})
print(result["semantic"], result["structural"])
```

## Caveats / footguns

- **The local store is NOT a database** — it's a cache. Production should use Qdrant + Neo4j for durability.
- **The SHA-256 fallback is NOT semantically meaningful.** Similarity scores are noise. Use real embeddings.
- **`query_hybrid_memory` swallows backend errors and falls back to local** — failures are logged but not surfaced to the caller.
