from __future__ import annotations

import logging
import os
import threading
from typing import Any

from .core import _workspace_root

logger = logging.getLogger(__name__)

_neo4j_driver_lock = threading.Lock()
_neo4j_driver_cache: dict[str, Any] = {}
_qdrant_client_lock = threading.Lock()
_qdrant_client_cache: dict[str, Any] = {}


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


_embedding_model_lock = threading.Lock()
_embedding_model_cache: dict[str, Any] = {}


def _get_embedding_model(model_name: str) -> Any:
    with _embedding_model_lock:
        if model_name in _embedding_model_cache:
            return _embedding_model_cache[model_name]
        from sentence_transformers import SentenceTransformer  # type: ignore[import-not-found]
        model = SentenceTransformer(model_name)
        _embedding_model_cache[model_name] = model
        return model


def query_hybrid_memory(payload: dict[str, Any]) -> dict[str, Any]:
    query = str(payload.get("query") or "").strip()
    root = _workspace_root(payload.get("root"))
    qdrant_url = os.getenv("QDRANT_URL", "")
    neo4j_uri = os.getenv("NEO4J_URI", "")
    neo4j_user = os.getenv("NEO4J_USER", "neo4j")
    neo4j_password = os.getenv("NEO4J_PASSWORD", "")

    semantic_results: list[dict[str, Any]] = []
    structural_results: list[dict[str, Any]] = []

    if qdrant_url:
        semantic_results = _query_qdrant(query, qdrant_url)

    if neo4j_uri and neo4j_user and neo4j_password:
        graph_symbol = payload.get("target_symbol")
        if graph_symbol:
            structural_results = _query_neo4j(str(graph_symbol), neo4j_uri, neo4j_user, neo4j_password)

    enabled = bool(qdrant_url and neo4j_uri)
    return {
        "ok": True,
        "enabled": enabled,
        "query": query,
        "root": str(root),
        "vectorStore": qdrant_url or None,
        "graphStore": neo4j_uri or None,
        "semantic": semantic_results,
        "structural": structural_results,
        "totalResults": len(semantic_results) + len(structural_results),
    }


def _get_qdrant_client(qdrant_url: str) -> Any:
    with _qdrant_client_lock:
        if qdrant_url in _qdrant_client_cache:
            return _qdrant_client_cache[qdrant_url]
        from qdrant_client import QdrantClient  # type: ignore[import-not-found]
        client = QdrantClient(url=qdrant_url, timeout=5)
        _qdrant_client_cache[qdrant_url] = client
        return client


def _query_qdrant(query: str, qdrant_url: str) -> list[dict[str, Any]]:
    try:
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
    except Exception:
        logger.debug("Qdrant query failed for url=%s", qdrant_url, exc_info=True)
        return []


def _get_embedding(query: str, qdrant_url: str, client: Any) -> list[float] | None:
    try:
        if hasattr(client, "query"):
            dense = client.query(collection_name="_default", query_text=query)
            if dense:
                return None
        embedding_model = os.getenv("QDRANT_EMBEDDING_MODEL", "")
        if not embedding_model:
            return None
        try:
            model = _get_embedding_model(embedding_model)
            vec = model.encode(query)
            return vec.tolist()  # type: ignore[no-any-return]
        except ImportError:
            return None
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
    try:
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
    except Exception:
        logger.debug("Neo4j query failed for symbol=%s", symbol, exc_info=True)
        return []
