# SPDX-License-Identifier: MIT
#!/usr/bin/env python3
"""Quick benchmark for SCKG query performance after inverted index fix."""

import gc
import sys
import tempfile
import time
from pathlib import Path

sys.path.insert(0, "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src")
from sin_code_sckg.graph import KnowledgeGraph


def generate_synthetic_repo(num_files: int, root: str) -> str:
    repo = Path(root) / f"synthetic_{num_files}"
    repo.mkdir(parents=True, exist_ok=True)
    for i in range(num_files):
        file_path = repo / f"module_{i}.py"
        lines = [
            f'"""Module {i} — auto-generated for benchmarking."""',
            "import os",
            "import sys",
            f"from module_{(i + 1) % num_files} import helper_{(i + 1) % num_files}",
            "",
            f"class ClassA_{i}:",
            '    """A sample class."""',
            f"    def method_{i}(self, x: int) -> int:",
            "        if x > 0:",
            "            for j in range(x):",
            "                if j % 2 == 0:",
            "                    continue",
            "        return x * 2",
            "",
            f"class ClassB_{i}(ClassA_{i}):",
            "    pass",
            "",
            f"def helper_{i}(a: int, b: int) -> int:",
            "    result = a + b",
            "    while result < 100:",
            "        result += 1",
            "    return result",
            "",
            f"def main_{i}():",
            f"    obj = ClassA_{i}()",
            f"    obj.method_{i}(10)",
            f"    helper_{i}(1, 2)",
            "",
            'if __name__ == "__main__":',
            f"    main_{i}()",
            "",
        ]
        file_path.write_text("\n".join(lines), encoding="utf-8")
    return str(repo)


def benchmark_query(files: int, query: str) -> float:
    gc.collect()
    start = time.perf_counter()
    results = kg.query(query)
    duration = time.perf_counter() - start
    return duration, len(results)


if __name__ == "__main__":
    with tempfile.TemporaryDirectory() as tmpdir:
        for num_files in [100, 1000, 10000]:
            repo = generate_synthetic_repo(num_files, tmpdir)
            kg = KnowledgeGraph(storage_path=f"{tmpdir}/sckg_bench_{num_files}.graph")
            stats = kg.build_from_repo(repo)
            print(
                f"\nBuild {num_files}: {stats['files']} files, {stats['functions']} funcs, {stats['classes']} classes, {stats['edges']} edges"
            )

            for query in ["helper", "ClassA"]:
                gc.collect()
                # Warm-up
                kg.query(query)
                # Timed runs
                times = []
                for _ in range(10):
                    gc.collect()
                    start = time.perf_counter()
                    results = kg.query(query)
                    times.append(time.perf_counter() - start)
                avg = sum(times) / len(times)
                min_t = min(times)
                print(
                    f"  Query '{query}' on {num_files}: avg={avg:.4f}s min={min_t:.4f}s results={len(results)}"
                )

            # Clear for next iteration
            del kg
            gc.collect()
