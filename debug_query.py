# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Debug the query issue."""

import sys

sys.path.insert(0, "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src")
import tempfile
from pathlib import Path

from sin_code_sckg.graph import KnowledgeGraph

# Create a tiny synthetic repo
with tempfile.TemporaryDirectory() as tmpdir:
    repo = Path(tmpdir) / "test_repo"
    repo.mkdir()
    (repo / "module_0.py").write_text("def helper_0(): pass\nclass ClassA_0: pass\n")

    kg = KnowledgeGraph(storage_path=f"{tmpdir}/test.graph")
    stats = kg.build_from_repo(repo)
    print(f"Build: {stats}")
    print(f"Nodes: {len(kg.nodes)}")
    print(f"Index keys (sample): {list(kg._inverted_index.keys())[:20]}")

    for q in ["helper", "ClassA", "helper_0", "module_0"]:
        results = kg.query(q)
        print(f"Query '{q}': {len(results)} results")
        for r in results:
            print(f"  - {r.name} ({r.type}) @ {r.file_path}")
