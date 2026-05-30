from __future__ import annotations

import json
import logging
import math
import os
import sqlite3
import threading
import time
from pathlib import Path
from typing import Any

from .core import _workspace_root

logger = logging.getLogger(__name__)

_neo4j_driver_lock = threading.Lock()
_neo4j_driver_cache: dict[str, Any] = {}
_qdrant_client_lock = threading.Lock()
_qdrant_client_cache: dict[str, Any] = {}
_embedding_model_lock = threading.Lock()
_embedding_model_cache: dict[str, Any] = {}

_LOCAL_DB_DIR: str | None = None
_LOCAL_DB_LOCK = threading.Lock()
_LOCAL_DB_CACHE: dict[str, Any] = {}


def _get_local_db_dir() -> str:
    global _LOCAL_DB_DIR
    if _LOCAL_DB_DIR is not None:
        return _LOCAL_DB_DIR
    base = os.getenv("SIMONE_MEMORY_DIR", "")
    if base:
        _LOCAL_DB_DIR = base
    else:
        _LOCAL_DB_DIR = str(Path.home() / ".simone")
    return _LOCAL_DB_DIR


def _get_db_path(subdir: str) -> str:
    db_dir = os.path.join(_get_local_db_dir(), subdir)
    os.makedirs(db_dir, exist_ok=True)
    return os.path.join(db_dir, "memory.db")


def _get_connection(db_name: str = "default") -> sqlite3.Connection:
    with _LOCAL_DB_LOCK:
        cache_key = db_name
        if cache_key in _LOCAL_DB_CACHE:
            conn = _LOCAL_DB_CACHE[cache_key]
            try:
                conn.execute("SELECT 1")
                return conn
            except sqlite3.ProgrammingError:
                pass
        path = _get_db_path(db_name)
        conn = sqlite3.connect(path, check_same_thread=False)
        conn.row_factory = sqlite3.Row
        conn.execute("PRAGMA journal_mode=WAL")
        conn.execute("PRAGMA synchronous=OFF")
        _LOCAL_DB_CACHE[cache_key] = conn
        return conn


def _init_vector_store(conn: sqlite3.Connection) -> None:
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS vectors (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            collection TEXT NOT NULL DEFAULT 'default',
            file TEXT NOT NULL DEFAULT '',
            symbol TEXT NOT NULL DEFAULT '',
            text TEXT NOT NULL DEFAULT '',
            embedding BLOB,
            created_at REAL NOT NULL DEFAULT (unixepoch())
        );
        CREATE INDEX IF NOT EXISTS idx_vectors_collection ON vectors(collection);
    """)


def _init_graph_store(conn: sqlite3.Connection) -> None:
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS symbols (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            kind TEXT NOT NULL DEFAULT 'unknown',
            file TEXT NOT NULL DEFAULT '',
            line INTEGER DEFAULT 0,
            created_at REAL NOT NULL DEFAULT (unixepoch())
        );
        CREATE INDEX IF NOT EXISTS idx_symbols_name ON symbols(name);

        CREATE TABLE IF NOT EXISTS symbol_relations (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            source_id INTEGER NOT NULL,
            target_id INTEGER NOT NULL,
            rel_type TEXT NOT NULL DEFAULT 'references',
            created_at REAL NOT NULL DEFAULT (unixepoch()),
            FOREIGN KEY (source_id) REFERENCES symbols(id),
            FOREIGN KEY (target_id) REFERENCES symbols(id)
        );
        CREATE INDEX IF NOT EXISTS idx_relations_source ON symbol_relations(source_id);
        CREATE INDEX IF NOT EXISTS idx_relations_target ON symbol_relations(target_id);
    """)


def _lazy_init(db_name: str = "default") -> sqlite3.Connection:
    conn = _get_connection(db_name)
    _init_vector_store(conn)
    _init_graph_store(conn)
    return conn


def _cosine_similarity(a: list[float], b: list[float]) -> float:
    dot = sum(av * bv for av, bv in zip(a, b, strict=False))
    na = math.sqrt(sum(av * av for av in a))
    nb = math.sqrt(sum(bv * bv for bv in b))
    if na == 0 or nb == 0:
        return 0.0
    return dot / (na * nb)


def _compute_embedding(text: str) -> list[float] | None:
    try:
        model_name = os.getenv("LOCAL_EMBEDDING_MODEL", "all-MiniLM-L6-v2")
        with _embedding_model_lock:
            if model_name not in _embedding_model_cache:
                from sentence_transformers import SentenceTransformer  # type: ignore[import-not-found]
                _embedding_model_cache[model_name] = SentenceTransformer(model_name)
            model = _embedding_model_cache[model_name]
        vec = model.encode(text)
        return vec.tolist()  # type: ignore[no-any-return]
    except ImportError:
        logger.warning("sentence-transformers not installed; using random embeddings (results will be poor)")
        import hashlib
        h = hashlib.sha256(text.encode()).digest()
        norm = math.sqrt(sum(b * b for b in h))
        return [b / norm for b in h[:64]]
    except Exception:
        logger.debug("Embedding generation failed", exc_info=True)
        return None


def shutdown_stores() -> None:
    with _neo4j_driver_lock:
        for cache_key, driver in list(_neo4j_driver_cache.items()):
            try:
                driver.close()
                logger.info("Closed Neo4j driver for %s", cache_key)
            except Exception:
                logger.debug("Error closing Neo4j driver for %s", cache_key, exc_info=True)
        _neo4j_driver_cache.clear()
    with _qdrant_client_lock:
        for url, client in list(_qdrant_client_cache.items()):
            try:
                client.close()
                logger.info("Closed Qdrant client for %s", url)
            except Exception:
                logger.debug("Error closing Qdrant client for %s", url, exc_info=True)
        _qdrant_client_cache.clear()
    with _embedding_model_lock:
        _embedding_model_cache.clear()
    with _LOCAL_DB_LOCK:
        for cache_key, conn in list(_LOCAL_DB_CACHE.items()):
            try:
                conn.close()
                logger.info("Closed local SQLite memory store: %s", cache_key)
            except Exception:
                logger.debug("Error closing SQLite store: %s", cache_key, exc_info=True)
        _LOCAL_DB_CACHE.clear()


def query_hybrid_memory(payload: dict[str, Any]) -> dict[str, Any]:
    query = str(payload.get("query") or "").strip()
    root = _workspace_root(payload.get("root"))
    target_symbol = payload.get("target_symbol")

    qdrant_url = os.getenv("QDRANT_URL", "")
    neo4j_uri = os.getenv("NEO4J_URI", "")
    neo4j_user = os.getenv("NEO4J_USER", "neo4j")
    neo4j_password = os.getenv("NEO4J_PASSWORD", "")

    semantic_results: list[dict[str, Any]] = []
    structural_results: list[dict[str, Any]] = []

    if qdrant_url:
        try:
            semantic_results = _query_qdrant(query, qdrant_url)
        except Exception:
            logger.debug("Qdrant query failed, falling back to local", exc_info=True)

    if not semantic_results:
        semantic_results = _query_local_semantic(query)

    if neo4j_uri and neo4j_user and neo4j_password and target_symbol:
        try:
            structural_results = _query_neo4j(str(target_symbol), neo4j_uri, neo4j_user, neo4j_password)
        except Exception:
            logger.debug("Neo4j query failed, falling back to local", exc_info=True)

    if not structural_results and target_symbol:
        structural_results = _query_local_graph(str(target_symbol))

    return {
        "ok": True,
        "enabled": True,
        "query": query,
        "root": str(root),
        "vectorStore": qdrant_url or "local",
        "graphStore": neo4j_uri or "local",
        "semantic": semantic_results,
        "structural": structural_results,
        "totalResults": len(semantic_results) + len(structural_results),
    }


def _query_local_semantic(query: str) -> list[dict[str, Any]]:
    if not query:
        return _list_local_collections()
    try:
        emb = _compute_embedding(query)
        if emb is None:
            return _list_local_collections()
        conn = _lazy_init()
        rows = conn.execute(
            "SELECT id, collection, file, symbol, text, embedding FROM vectors ORDER BY id DESC LIMIT 500"
        ).fetchall()
        scored: list[tuple[float, dict[str, Any]]] = []
        for row in rows:
            stored = row["embedding"]
            if stored is None:
                continue
            try:
                stored_vec = json.loads(stored)
            except (json.JSONDecodeError, TypeError):
                continue
            score = _cosine_similarity(emb, stored_vec)
            scored.append((
                score,
                {
                    "collection": row["collection"],
                    "id": str(row["id"]),
                    "score": round(score, 4),
                    "file": row["file"],
                    "symbol": row["symbol"],
                    "text": row["text"][:200],
                },
            ))
        scored.sort(key=lambda x: -x[0])
        return [item for _, item in scored[:20] if item["score"] > 0.3]
    except Exception:
        logger.debug("Local semantic query failed", exc_info=True)
        return []


def _list_local_collections() -> list[dict[str, Any]]:
    try:
        conn = _lazy_init()
        counts = conn.execute(
            "SELECT collection, COUNT(*) as cnt FROM vectors GROUP BY collection ORDER BY cnt DESC"
        ).fetchall()
        return [
            {
                "collection": row["collection"],
                "vectorCount": row["cnt"],
                "status": "local",
            }
            for row in counts
        ]
    except Exception:
        return []


def _query_local_graph(symbol: str) -> list[dict[str, Any]]:
    try:
        conn = _lazy_init()
        rows = conn.execute(
            "SELECT s2.name, s2.file, s2.kind, sr.rel_type "
            "FROM symbol_relations sr "
            "JOIN symbols s1 ON sr.target_id = s1.id "
            "JOIN symbols s2 ON sr.source_id = s2.id "
            "WHERE s1.name = ? "
            "LIMIT 50",
            (symbol,),
        ).fetchall()
        return [
            {
                "name": row["name"],
                "file": row["file"],
                "kind": row["kind"],
                "relation": row["rel_type"],
            }
            for row in rows
        ]
    except Exception:
        logger.debug("Local graph query failed for symbol=%s", symbol, exc_info=True)
        return []


def store_symbol(name: str, kind: str, file_path: str, line: int = 0, db_name: str = "default") -> dict[str, Any]:
    try:
        conn = _lazy_init(db_name)
        existing = conn.execute(
            "SELECT id FROM symbols WHERE name = ? AND file = ?", (name, file_path)
        ).fetchone()
        if existing:
            return {"ok": True, "id": existing["id"], "exists": True}
        cur = conn.execute(
            "INSERT INTO symbols (name, kind, file, line) VALUES (?, ?, ?, ?)",
            (name, kind, file_path, line),
        )
        conn.commit()
        return {"ok": True, "id": cur.lastrowid, "exists": False}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def store_relation(source_id: int, target_id: int, rel_type: str = "references", db_name: str = "default") -> dict[str, Any]:
    try:
        conn = _lazy_init(db_name)
        conn.execute(
            "INSERT INTO symbol_relations (source_id, target_id, rel_type) VALUES (?, ?, ?)",
            (source_id, target_id, rel_type),
        )
        conn.commit()
        return {"ok": True}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def store_vector(
    text: str,
    collection: str = "default",
    file_path: str = "",
    symbol: str = "",
    db_name: str = "default",
) -> dict[str, Any]:
    try:
        emb = _compute_embedding(text)
        if emb is None:
            return {"ok": False, "error": "embedding_failed"}
        conn = _lazy_init(db_name)
        conn.execute(
            "INSERT INTO vectors (collection, file, symbol, text, embedding) VALUES (?, ?, ?, ?, ?)",
            (collection, file_path, symbol, text, json.dumps(emb)),
        )
        conn.commit()
        return {"ok": True}
    except Exception as e:
        return {"ok": False, "error": str(e)}


def get_local_stats(db_name: str = "default") -> dict[str, Any]:
    try:
        conn = _lazy_init(db_name)
        vec_count = conn.execute("SELECT COUNT(*) as cnt FROM vectors").fetchone()["cnt"]
        sym_count = conn.execute("SELECT COUNT(*) as cnt FROM symbols").fetchone()["cnt"]
        rel_count = conn.execute("SELECT COUNT(*) as cnt FROM symbol_relations").fetchone()["cnt"]
        return {
            "ok": True,
            "vectors": vec_count,
            "symbols": sym_count,
            "relations": rel_count,
            "db_path": _get_db_path(db_name),
        }
    except Exception:
        return {"ok": False, "error": "failed_to_read_stats"}


def _get_qdrant_client(qdrant_url: str) -> Any:
    with _qdrant_client_lock:
        if qdrant_url in _qdrant_client_cache:
            return _qdrant_client_cache[qdrant_url]
        from qdrant_client import QdrantClient  # type: ignore[import-not-found]
        client = QdrantClient(url=qdrant_url, timeout=5)
        _qdrant_client_cache[qdrant_url] = client
        return client


def _query_qdrant(query: str, qdrant_url: str) -> list[dict[str, Any]]:
    client = _get_qdrant_client(qdrant_url)
    collections = client.get_collections().collections
    if not collections:
        return []
    embedding = _get_embedding(query, qdrant_url, client)
    results: list[dict[str, Any]] = []
    for col in collections[:5]:
        if embedding:
            hits = client.search(
                collection_name=col.name,
                query_vector=embedding,
                limit=5,
            )
            for hit in hits:
                payload = hit.payload or {}
                results.append(
                    {
                        "collection": col.name,
                        "id": str(hit.id),
                        "score": hit.score,
                        "file": payload.get("file", payload.get("path", "")),
                        "symbol": payload.get("symbol", payload.get("name", "")),
                        "text": payload.get("text", payload.get("content", ""))[:200],
                    }
                )
        else:
            info = client.get_collection(col.name)
            results.append(
                {
                    "collection": col.name,
                    "vectorCount": info.points_count or 0,
                    "status": info.status,
                }
            )
    return results


def _get_embedding(query: str, qdrant_url: str, client: Any) -> list[float] | None:
    try:
        if hasattr(client, "query"):
            dense = client.query(collection_name="_default", query_text=query)
            if dense:
                return None
        embedding_model = os.getenv("QDRANT_EMBEDDING_MODEL", "")
        if not embedding_model:
            return None
        return _compute_embedding(query)
    except Exception:
        logger.debug("Embedding generation failed", exc_info=True)
        return None


def _get_neo4j_driver(uri: str, user: str, password: str) -> Any:
    cache_key = f"{uri}:{user}"
    with _neo4j_driver_lock:
        if cache_key in _neo4j_driver_cache:
            return _neo4j_driver_cache[cache_key]
        from neo4j import GraphDatabase  # type: ignore[import-not-found]
        driver = GraphDatabase.driver(uri, auth=(user, password), max_connection_lifetime=3600)
        _neo4j_driver_cache[cache_key] = driver
        return driver


def _query_neo4j(
    symbol: str, uri: str, user: str, password: str
) -> list[dict[str, Any]]:
    driver = _get_neo4j_driver(uri, user, password)
    results: list[dict[str, Any]] = []
    with driver.session() as session:
        query = (
            "MATCH (n:Symbol {name: $symbol})<-[:CALLS|IMPORTS]-(caller:Symbol) "
            "RETURN caller.name AS name, caller.file AS file "
            "LIMIT 10"
        )
        records = session.run(query, symbol=symbol)
        for record in records:
            results.append({"name": record["name"], "file": record["file"]})
    return results
