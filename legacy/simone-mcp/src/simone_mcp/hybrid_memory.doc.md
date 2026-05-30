# `src/simone_mcp/hybrid_memory.py` — Hybrid Memory Layer

Partner file: `src/simone_mcp/hybrid_memory.py`

## Purpose
Provides multi-tier memory storage: local SQLite (vectors + graph), Qdrant (vector DB), and Neo4j (graph DB). Supports semantic search via cosine similarity and structural search via graph relations.

## Key Symbols
| Symbol | Kind | Purpose |
|--------|------|---------|
| `query_hybrid_memory()` | function | Query all memory tiers with fallback chain |
| `_query_local_semantic()` | function | SQLite vector similarity search |
| `_query_local_graph()` | function | SQLite graph relation lookup |
| `_query_qdrant()` | function | Qdrant vector search |
| `_query_neo4j()` | function | Neo4j CALLS/IMPORTS relation query |
| `store_symbol()` | function | Store symbol in local graph DB |
| `store_relation()` | function | Store relation in local graph DB |
| `store_vector()` | function | Store vector embedding in local DB |
| `get_local_stats()` | function | Return local DB stats |
| `shutdown_stores()` | function | Close all connections (lifespan cleanup) |
| `_compute_embedding()` | function | Generate embeddings via sentence-transformers or hash fallback |
| `_cosine_similarity()` | function | Cosine similarity between two vectors |

## Database Schema (SQLite)
**vectors** table: id, collection, file, symbol, text, embedding, created_at
**symbols** table: id, name, kind, file, line, created_at
**symbol_relations** table: id, source_id, target_id, rel_type, created_at

## Fallback Chain
1. Try Qdrant for semantic search
2. Fallback to local SQLite semantic search
3. Try Neo4j for structural search (if target_symbol provided)
4. Fallback to local SQLite graph search

## Relationship
- `src/simone_mcp/core.py` — calls `query_hybrid_memory()` via `execute_simone_action()`
- `src/simone_mcp/http_app.py` — calls `shutdown_stores()` on app shutdown
- `src/simone_mcp/cli.py` — validates Qdrant/Neo4j in `_validate_config()`

## Dependencies
| Optional | Purpose |
|----------|---------|
| `sentence-transformers` | Embedding generation (all-MiniLM-L6-v2) |
| `qdrant_client` | Qdrant vector DB |
| `neo4j` | Neo4j graph DB |

## Environment Variables
| Variable | Default | Purpose |
|----------|---------|---------|
| `QDRANT_URL` | — | Qdrant server URL |
| `NEO4J_URI` | — | Neo4j Bolt URI |
| `NEO4J_USER` | neo4j | Neo4j username |
| `NEO4J_PASSWORD` | — | Neo4j password |
| `SIMONE_MEMORY_DIR` | ~/.simone | Local SQLite directory |
| `LOCAL_EMBEDDING_MODEL` | all-MiniLM-L6-v2 | Embedding model name |
| `QDRANT_EMBEDDING_MODEL` | — | Qdrant model override |

## Thread Safety
- SQLite connections are cached per db_name with `check_same_thread=False`
- Neo4j drivers and Qdrant clients are cached with locks
- All cache operations are protected by threading.Lock

## Broken Links Check
- No internal links to other `.doc.md` files in this module.
