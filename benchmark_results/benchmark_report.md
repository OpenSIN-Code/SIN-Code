# SIN-Code Tool Performance Benchmark Report

Generated: 2026-06-02T14:20:41+02:00

| Tool | Benchmark | Result | Target | Status |
|------|-----------|--------|--------|--------|
| SIN-Brain  | remember_1000                  | 0.364s     | 5.000s     | PASS   |
| SIN-Brain  | recall_1000                    | 0.107s     | 0.500s     | PASS   |
| SIN-Brain  | sqlite_query_1000              | 0.058s     | 0.200s     | PASS   |
| SIN-Brain  | consolidation_1000             | 0.074s     | 1.000s     | PASS   |
| SIN-Brain  | remember_10000                 | 2.675s     | 30.000s    | PASS   |
| SIN-Brain  | recall_10000                   | 0.086s     | 1.000s     | PASS   |
| SIN-Brain  | sqlite_query_10000             | 0.053s     | 0.500s     | PASS   |
| SIN-Brain  | consolidation_10000            | 0.086s     | 5.000s     | PASS   |
| Bundle     | mcp_startup                    | 0.911s     | 5.000s     | PASS   |
| Bundle     | tool_dispatch                  | 0.667s     | 2.000s     | PASS   |
| Bundle     | concurrent_10_requests         | 0.666s     | 2.000s     | PASS   |
| Bundle     | concurrent_50_requests         | 0.644s     | 3.000s     | PASS   |
| Bundle     | concurrent_100_requests        | 0.641s     | 5.000s     | PASS   |
| Discover   | discovery_100_py_files         | 0.208s     | 1.000s     | PASS   |
| Discover   | discovery_500_py_files         | 5.888s     | 3.000s     | FAIL   |
| Discover   | discovery_1000_py_files        | 25.318s    | 5.000s     | FAIL   |
| Discover   | max_results_early_stop_10      | 26.405s    | 0.500s     | FAIL   |
| Discover   | max_results_early_stop_100     | 24.303s    | 1.000s     | FAIL   |
| Discover   | max_results_early_stop_1000    | 23.270s    | 3.000s     | FAIL   |
| Discover   | relevance_scoring_500          | 6.307s     | 5.000s     | FAIL   |
| Discover   | extension_filter_py            | 6.654s     | 2.000s     | FAIL   |
| Discover   | extension_filter_go            | 0.649s     | 2.000s     | PASS   |
| Discover   | extension_filter_js            | 0.709s     | 2.000s     | PASS   |
| Execute    | overhead_empty_command         | 0.016s     | 0.500s     | PASS   |
| Execute    | overhead_empty_command         | 0.015s     | 0.500s     | PASS   |
| Execute    | overhead_empty_command         | 0.017s     | 0.500s     | PASS   |
| Execute    | overhead_real_command          | 0.130s     | 1.000s     | PASS   |
| Execute    | secret_redaction_1mb_100_secrets | 0.067s     | 3.000s     | PASS   |
| Execute    | timeout_kill_latency           | 1.009s     | 1.500s     | PASS   |
| Execute    | timeout_overhead               | 0.009s     | 0.100s     | PASS   |
| SCKG       | build_100_files                | 0.183s     | 2.000s     | PASS   |
| SCKG       | simple_query_100_files         | 0.089s     | 0.050s     | FAIL   |
| SCKG       | complex_query_100_files        | 0.080s     | 0.050s     | FAIL   |
| SCKG       | memory_100_files               | 0.191s     | 50.000s    | PASS   |
| SCKG       | build_1000_files               | 1.125s     | 15.000s    | PASS   |
| SCKG       | simple_query_1000_files        | 0.249s     | 0.200s     | FAIL   |
| SCKG       | complex_query_1000_files       | 0.217s     | 0.200s     | FAIL   |
| SCKG       | memory_1000_files              | 1.553s     | 150.000s   | PASS   |
| SCKG       | build_10000_files              | ERROR: Build failed: Traceback (most recent call last):
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/storage.py", line 21, in save
    json.dump(data, f, indent=2, default=str)
    ~~~~~~~~~^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/__init__.py", line 182, in dump
    fp.write(chunk)
    ~~~~~~~~^^^^^^^
OSError: [Errno 28] No space left on device

During handling of the above exception, another exception occurred:

OSError: [Errno 28] No space left on device

During handling of the above exception, another exception occurred:

Traceback (most recent call last):
  File "<string>", line 8, in <module>
    stats = kg.build_from_repo("/var/folders/4k/w1vg2tbj7718gc0mj308m95m0000gn/T/tmp9mqgmrsx/synthetic_10000")
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 84, in build_from_repo
    self.save()
    ~~~~~~~~~^^
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 105, in save
    self.storage.save(self.to_dict())
    ~~~~~~~~~~~~~~~~~^^^^^^^^^^^^^^^^
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/storage.py", line 20, in save
    with open(self.path, "w", encoding="utf-8") as f:
         ~~~~^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
OSError: [Errno 28] No space left on device
 | 120.000s   | ERROR  |
| SCKG       | simple_query_10000_files       | ERROR: Query failed: Traceback (most recent call last):
  File "<string>", line 6, in <module>
    kg = KnowledgeGraph(storage_path="/tmp/sckg_bench_10000.graph")
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 25, in __init__
    self._load()
    ~~~~~~~~~~^^
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 29, in _load
    data = self.storage.load()
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/storage.py", line 28, in load
    return json.load(f)
           ~~~~~~~~~^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/__init__.py", line 298, in load
    return loads(fp.read(),
        cls=cls, object_hook=object_hook,
        parse_float=parse_float, parse_int=parse_int,
        parse_constant=parse_constant, object_pairs_hook=object_pairs_hook, **kw)
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/__init__.py", line 352, in loads
    return _default_decoder.decode(s)
           ~~~~~~~~~~~~~~~~~~~~~~~^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/decoder.py", line 345, in decode
    obj, end = self.raw_decode(s, idx=_w(s, 0).end())
               ~~~~~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/decoder.py", line 361, in raw_decode
    obj, end = self.scan_once(s, idx)
               ~~~~~~~~~~~~~~^^^^^^^^
json.decoder.JSONDecodeError: Expecting ':' delimiter: line 3008133 column 17 (char 85399225)
 | 1.000s     | ERROR  |
| SCKG       | complex_query_10000_files      | ERROR: Query failed: Traceback (most recent call last):
  File "<string>", line 6, in <module>
    kg = KnowledgeGraph(storage_path="/tmp/sckg_bench_10000.graph")
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 25, in __init__
    self._load()
    ~~~~~~~~~~^^
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/graph.py", line 29, in _load
    data = self.storage.load()
  File "/Users/jeremy/dev/SIN-Code-Semantic-Codebase-Knowledge-Graphs/src/sin_code_sckg/storage.py", line 28, in load
    return json.load(f)
           ~~~~~~~~~^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/__init__.py", line 298, in load
    return loads(fp.read(),
        cls=cls, object_hook=object_hook,
        parse_float=parse_float, parse_int=parse_int,
        parse_constant=parse_constant, object_pairs_hook=object_pairs_hook, **kw)
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/__init__.py", line 352, in loads
    return _default_decoder.decode(s)
           ~~~~~~~~~~~~~~~~~~~~~~~^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/decoder.py", line 345, in decode
    obj, end = self.raw_decode(s, idx=_w(s, 0).end())
               ~~~~~~~~~~~~~~~^^^^^^^^^^^^^^^^^^^^^^^
  File "/opt/homebrew/Cellar/python@3.14/3.14.4_1/Frameworks/Python.framework/Versions/3.14/lib/python3.14/json/decoder.py", line 361, in raw_decode
    obj, end = self.scan_once(s, idx)
               ~~~~~~~~~~~~~~^^^^^^^^
json.decoder.JSONDecodeError: Expecting ':' delimiter: line 3008133 column 17 (char 85399225)
 | 1.000s     | ERROR  |
| SCKG       | memory_10000_files             | 14.024s    | 500.000s   | PASS   |

## Summary

- **Total benchmarks**: 42
- **PASS**: 28
- **FAIL**: 11
- **ERROR**: 3

### Performance Issues Found

- **Discover — discovery_500_py_files**: 5.888s (target: 3.000s)
- **Discover — discovery_1000_py_files**: 25.318s (target: 5.000s)
- **Discover — max_results_early_stop_10**: 26.405s (target: 0.500s)
- **Discover — max_results_early_stop_100**: 24.303s (target: 1.000s)
- **Discover — max_results_early_stop_1000**: 23.270s (target: 3.000s)
- **Discover — relevance_scoring_500**: 6.307s (target: 5.000s)
- **Discover — extension_filter_py**: 6.654s (target: 2.000s)
- **SCKG — simple_query_100_files**: 0.089s (target: 0.050s)
- **SCKG — complex_query_100_files**: 0.080s (target: 0.050s)
- **SCKG — simple_query_1000_files**: 0.249s (target: 0.200s)
- **SCKG — complex_query_1000_files**: 0.217s (target: 0.200s)


## Raw Data

Individual JSON files are in `/Users/jeremy/dev/SIN-Code-Bundle/benchmark_results`.
