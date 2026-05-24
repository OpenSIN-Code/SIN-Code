from __future__ import annotations

import os
from typing import Any

from .core import _workspace_root


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

        client = QdrantClient(url=qdrant_url)
        collections = client.get_collections().collections
        if not collections:
            return []
        results: list[dict[str, Any]] = []
        for col in collections[:3]:
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


def _query_neo4j(
    symbol: str, uri: str, user: str, password: str
) -> list[dict[str, Any]]:
    try:
        from neo4j import GraphDatabase

        driver = GraphDatabase.driver(uri, auth=(user, password))
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
        driver.close()
        return results
    except Exception:
        return []
