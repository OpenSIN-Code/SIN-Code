import os
import tempfile

import pytest
from sin_code_sckg.graph import KnowledgeGraph
from sin_code_sckg.parser import SemanticParser

_parser = SemanticParser(["python"])
_skip = pytest.mark.skipif(not _parser.available, reason="tree-sitter python grammar not available")


@_skip
def test_parse_python():
    with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
        f.write(
            "def hello():\n"
            '    """Docstring"""\n'
            '    print("hi")\n\n'
            "def user():\n"
            "    hello()\n\n"
            "class Service:\n"
            "    def run(self):\n"
            "        user()\n"
        )
        f.flush()
        path = f.name
    try:
        symbols = _parser.parse_file(path)
    finally:
        os.unlink(path)
    names = {s.name for s in symbols}
    assert "hello" in names
    assert "user" in names
    assert "Service" in names


@_skip
def test_docstring_detection():
    with tempfile.NamedTemporaryFile(mode="w", suffix=".py", delete=False) as f:
        f.write('def doc():\n    """real doc"""\n    pass\n')
        f.flush()
        path = f.name
    try:
        symbols = _parser.parse_file(path)
    finally:
        os.unlink(path)
    doc_sym = next(s for s in symbols if s.name == "doc")
    assert doc_sym.docstring is not None
    assert "real doc" in doc_sym.docstring


@_skip
def test_graph_build():
    with tempfile.TemporaryDirectory() as d:
        with open(os.path.join(d, "a.py"), "w") as f:
            f.write("def a():\n    b()\ndef b():\n    pass\n")
        kg = KnowledgeGraph()
        stats = kg.build_from_repo(d, exclude=[], include_intents=False)
        assert stats["symbols"] >= 2
        assert stats["edges"] >= 1


@_skip
def test_persistence_roundtrip(tmp_path):
    with tempfile.TemporaryDirectory() as d:
        with open(os.path.join(d, "a.py"), "w") as f:
            f.write("def a():\n    pass\n")
        store = str(tmp_path / "g.json")
        kg = KnowledgeGraph(storage_path=store)
        kg.build_from_repo(d, exclude=[], include_intents=False)
        kg2 = KnowledgeGraph(storage_path=store)
        assert len(kg2.graph) == len(kg.graph)


def test_graph_query_no_crash_on_missing():
    kg = KnowledgeGraph()
    assert kg.impact_analysis("nonexistent")["error"]
    assert kg.explain_architecture()["total_nodes"] == 0
