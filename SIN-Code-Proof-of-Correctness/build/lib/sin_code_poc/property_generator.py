"""Generiert Properties (formale Invarianten) fuer Python-Funktionen.

Wichtiger Fix gegenueber dem urspruenglichen Entwurf: Properties tragen jetzt
ein echtes Callable (`predicate`) statt eines per `eval` ausgewerteten Strings.
Das ist sicher und vermeidet NameError fuer Properties, die eine Inverse
benoetigen (diese werden als `needs_inverse` markiert und in der automatischen
Verifikation uebersprungen).
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Callable


@dataclass
class Property:
    name: str
    description: str
    strategy_hint: str          # menschенlesbarer hypothesis-Strategiename
    predicate: Callable         # predicate(fn, x) -> bool
    check_code: str             # menschenlesbarer Ausdruck (nur fuer Rendering)
    needs_inverse: bool = False  # benoetigt eine Inverse g -> nicht autoverifizierbar


def _p_idempotent(fn, x):
    return fn(fn(x)) == fn(x)


def _p_pure(fn, x):
    return fn(x) == fn(x)


def _p_monotonic(fn, x):
    # x ist hier ein Tupel (a, b)
    a, b = x
    if a <= b:
        return fn(a) <= fn(b)
    return True


def _p_non_negative(fn, x):
    return fn(x) >= 0


def _p_length_preserving(fn, x):
    return len(fn(x)) == len(x)


def _p_sorted_output(fn, x):
    out = fn(x)
    return list(out) == sorted(out)


def _p_reversible(fn, x):  # pragma: no cover - benoetigt Inverse
    raise NotImplementedError("reversible requires an inverse function g")


class PropertyGenerator:
    """Erkennt Funktionssignaturen und schlaegt passende Properties vor."""

    BUILTIN_TEMPLATES = {
        "idempotent": Property(
            name="idempotent",
            description="f(f(x)) == f(x)",
            strategy_hint="integers()",
            predicate=_p_idempotent,
            check_code="f(f(x)) == f(x)",
        ),
        "reversible": Property(
            name="reversible",
            description="g(f(x)) == x",
            strategy_hint="integers()",
            predicate=_p_reversible,
            check_code="g(f(x)) == x",
            needs_inverse=True,
        ),
        "pure_no_side_effects": Property(
            name="pure",
            description="Same input always yields same output",
            strategy_hint="integers()",
            predicate=_p_pure,
            check_code="f(x) == f(x)",
        ),
        "monotonic": Property(
            name="monotonic",
            description="x <= y implies f(x) <= f(y)",
            strategy_hint="tuples(integers(), integers())",
            predicate=_p_monotonic,
            check_code="(a <= b) implies (f(a) <= f(b))",
        ),
        "boundary_non_negative": Property(
            name="non_negative_output",
            description="Output is always >= 0",
            strategy_hint="integers()",
            predicate=_p_non_negative,
            check_code="f(x) >= 0",
        ),
        "length_preserving": Property(
            name="length_preserving",
            description="len(output) == len(input)",
            strategy_hint="lists(integers())",
            predicate=_p_length_preserving,
            check_code="len(f(x)) == len(x)",
        ),
        "sorted_output": Property(
            name="sorted_output",
            description="Output is sorted",
            strategy_hint="lists(integers())",
            predicate=_p_sorted_output,
            check_code="sorted(f(x)) == f(x)",
        ),
    }

    def suggest(self, signature: dict) -> list[Property]:
        props: list[Property] = []
        ret = (signature.get("return_type") or "").lower()
        name = (signature.get("name") or "").lower()

        if ret in ("int", "float"):
            props.append(self.BUILTIN_TEMPLATES["boundary_non_negative"])
        if ret in ("list", "tuple", "str"):
            props.append(self.BUILTIN_TEMPLATES["length_preserving"])
        if "sort" in name:
            props.append(self.BUILTIN_TEMPLATES["sorted_output"])
        if "encode" in name or "decode" in name:
            props.append(self.BUILTIN_TEMPLATES["reversible"])
        if "normalize" in name or "canonical" in name:
            props.append(self.BUILTIN_TEMPLATES["idempotent"])

        props.append(self.BUILTIN_TEMPLATES["pure_no_side_effects"])
        # Deduplizieren, Reihenfolge erhalten
        seen = set()
        unique = []
        for p in props:
            if p.name not in seen:
                seen.add(p.name)
                unique.append(p)
        return unique

    def render_hypothesis_test(self, func_name: str, properties: list[Property]) -> str:
        lines = [
            "from hypothesis import given, strategies as st",
            f"from your_module import {func_name}",
            "",
            f"f = {func_name}",
            "",
        ]
        for p in properties:
            if p.needs_inverse:
                lines.append(f"# NOTE: '{p.name}' requires an inverse g; provide it manually.")
            lines.append(f"@given(x=st.{p.strategy_hint})")
            lines.append(f"def test_{p.name}(x):")
            lines.append(f'    """{p.description}"""')
            lines.append(f"    assert {p.check_code}")
            lines.append("")
        return "\n".join(lines)
