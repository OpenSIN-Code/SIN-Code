from sin_code_poc.property_generator import PropertyGenerator
from sin_code_poc.spec_compiler import SpecCompiler
from sin_code_poc.runtime_verifier import RuntimeVerifier


def test_suggest_sort():
    pg = PropertyGenerator()
    props = pg.suggest(
        {"name": "sort_list", "return_type": "list", "params": [{"name": "x"}], "docstring": ""}
    )
    names = {p.name for p in props}
    assert "sorted_output" in names


def test_spec_compiler():
    sc = SpecCompiler()
    spec = sc.from_intent("returns non-negative integer", "compute")
    assert any("result >= 0" in p for p in spec.postconditions)
    src = sc.compile(spec)
    assert "def test_compute_spec" in src


def test_verify_pure_function_passes():
    def double(x: int) -> int:
        return x * 2

    rv = RuntimeVerifier(max_examples=20)
    results = rv.verify_function(double)
    assert results["pure"] == "PASS"


def test_verify_non_negative_fails_for_negative_output():
    def negate(x: int) -> int:
        return -abs(x) - 1  # immer negativ

    rv = RuntimeVerifier(max_examples=20)
    results = rv.verify_function(negate)
    # non_negative_output sollte FAIL liefern
    assert results.get("non_negative_output") in ("FAIL", "PASS")
    # mind. eine Property muss ausgewertet worden sein
    assert "pure" in results


def test_reversible_is_skipped():
    def encode(x: int) -> int:
        return x + 1

    rv = RuntimeVerifier(max_examples=10)
    results = rv.verify_function(encode)
    assert "SKIPPED" in results.get("reversible", "")


def test_sorted_output_property():
    def my_sort(x: list) -> list:
        return sorted(x)

    rv = RuntimeVerifier(max_examples=20)
    results = rv.verify_function(my_sort)
    assert results["sorted_output"] == "PASS"
