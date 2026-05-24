from __future__ import annotations

import os
import threading
from typing import Any

from .core import _workspace_root

_neo4j_driver_lock = threading.Lock()
_neo4j_driver_cache: dict[str, Any] = {}


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


def _query_qdrant(query: str, qdrant_url: str) -> list[dict[str, Any]]:
    try:
        from qdrant_client import QdrantClient

        client = QdrantClient(url=qdrant_url, timeout=5)
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
            from sentence_transformers import SentenceTransformer

            model = SentenceTransformer(embedding_model)
            vec = model.encode(query)
            return vec.tolist()
        except ImportError:
            return None
    except Exception:
        return None


def _get_neo4j_driver(uri: str, user: str, password: str) -> Any:
    cache_key = f"{uri}:{user}"
    if cache_key in _neo4j_driver_cache:
        return _neo4j_driver_cache[cache_key]
    from neo4j import GraphDatabase

    driver = GraphDatabase.driver(uri, auth=(user, password), max_connection_lifetime=3600)
    with _neo4j_driver_lock:
        if cache_key not in _neo4j_driver_cache:
            _neo4j_driver_cache[cache_key] = driver
        return _neo4j_driver_cache[cache_key]


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
        return []
