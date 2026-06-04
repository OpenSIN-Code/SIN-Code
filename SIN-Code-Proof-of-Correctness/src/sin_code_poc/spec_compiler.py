"""Kompiliert einfache Spezifikationen in testbare Properties."""
from __future__ import annotations

from dataclasses import dataclass, field


@dataclass
class Specification:
    function_name: str
    preconditions: list[str] = field(default_factory=list)
    postconditions: list[str] = field(default_factory=list)
    invariants: list[str] = field(default_factory=list)


class SpecCompiler:
    """Uebersetzt deklarative Specs in Property-Tests (als Quelltext)."""

    def compile(self, spec: Specification) -> str:
        lines = [
            "from hypothesis import given, strategies as st, assume",
            f"from your_module import {spec.function_name}",
            "",
            "",
            "@given(x=st.integers())",
            f"def test_{spec.function_name}_spec(x):",
        ]
        body = []
        for pre in spec.preconditions:
            body.append(f"    assume({pre})")
        body.append(f"    result = {spec.function_name}(x)")
        for post in spec.postconditions:
            body.append(f"    assert {post}, 'postcondition failed: {post}'")
        if not spec.postconditions:
            body.append("    assert result is not None")
        return "\n".join(lines + body)

    def from_intent(self, intent: str, function_name: str) -> Specification:
        intent = intent.lower()
        pre, post, inv = [], [], []
        if "positive" in intent or "non-negative" in intent or "nicht-negativ" in intent:
            post.append("result >= 0")
        if "sorted" in intent or "sortiert" in intent:
            post.append("result == sorted(result)")
        if "non-empty input" in intent or "nicht-leer" in intent:
            pre.append("len(x) > 0")
        if "idempotent" in intent:
            post.append(f"{function_name}(result) == result")
        if not post:
            post.append("result is not None")
        return Specification(
            function_name=function_name,
            preconditions=pre,
            postconditions=post,
            invariants=inv,
        )
