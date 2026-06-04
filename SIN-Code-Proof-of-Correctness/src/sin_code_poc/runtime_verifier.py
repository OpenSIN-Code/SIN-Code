"""Runtime-Verifikation fuer bereits generierten Code.

Nutzt echte Callables (keine eval-Strings) und passende hypothesis-Strategien
je nach Property.
"""
from __future__ import annotations

import ast
import inspect
import textwrap
from typing import Any, Callable

from .property_generator import Property, PropertyGenerator


class RuntimeVerifier:
    def __init__(self, max_examples: int = 50):
        self.max_examples = max_examples

    def _strategy_for(self, prop: Property):
        from hypothesis import strategies as st
        hint = prop.strategy_hint
        if hint.startswith("lists"):
            return st.lists(st.integers(min_value=-1000, max_value=1000), max_size=20)
        if hint.startswith("tuples"):
            return st.tuples(
                st.integers(min_value=-1000, max_value=1000),
                st.integers(min_value=-1000, max_value=1000),
            )
        return st.integers(min_value=-1000, max_value=1000)

    def verify_function(
        self, fn: Callable, properties: list[Property] | None = None
    ) -> dict:
        from hypothesis import given, settings, HealthCheck

        if properties is None:
            sig = self.extract_signature(fn)
            properties = PropertyGenerator().suggest(sig)

        results: dict[str, str] = {}
        for prop in properties:
            if prop.needs_inverse:
                results[prop.name] = "SKIPPED (needs inverse)"
                continue
            strategy = self._strategy_for(prop)

            def _make_test(active_prop):
                @given(x=strategy)
                @settings(
                    max_examples=min(self.max_examples, 50),
                    suppress_health_check=[HealthCheck.too_slow],
                    deadline=None,
                )
                def _test(x):
                    assert active_prop.predicate(fn, x)

                return _test

            try:
                _make_test(prop)()
                results[prop.name] = "PASS"
            except AssertionError:
                results[prop.name] = "FAIL"
            except Exception as e:  # TypeError etc. -> property nicht anwendbar
                results[prop.name] = f"N/A ({type(e).__name__})"
        return results

    @staticmethod
    def extract_signature(fn: Callable) -> dict:
        try:
            src = inspect.getsource(fn)
        except (OSError, TypeError):
            return {"name": getattr(fn, "__name__", "fn"), "params": [], "return_type": None}
        src = textwrap.dedent(src)
        try:
            tree = ast.parse(src)
        except (SyntaxError, IndentationError):
            return {"name": getattr(fn, "__name__", "fn"), "params": [], "return_type": None}
        for node in ast.walk(tree):
            if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
                params = [
                    {
                        "name": a.arg,
                        "annotation": ast.unparse(a.annotation) if a.annotation else None,
                    }
                    for a in node.args.args
                ]
                ret = ast.unparse(node.returns) if node.returns else None
                return {
                    "name": node.name,
                    "params": params,
                    "return_type": ret,
                    "docstring": ast.get_docstring(node),
                }
        return {}
