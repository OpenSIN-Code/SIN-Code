# SPDX-License-Identifier: MIT
"""JSON conversion helpers for optional ecosystem subsystem results."""

from __future__ import annotations

from dataclasses import fields, is_dataclass
from typing import Any


def jsonable(value: Any) -> Any:
    """Convert heterogeneous subsystem return values into JSON-safe data."""
    if value is None or isinstance(value, (str, int, float, bool)):
        return value
    if hasattr(value, "to_dict"):
        return jsonable(value.to_dict())
    if isinstance(value, dict):
        return {str(key): jsonable(item) for key, item in value.items()}
    if isinstance(value, (list, tuple, set, frozenset)):
        return [jsonable(item) for item in value]
    if is_dataclass(value) and not isinstance(value, type):
        return {field.name: jsonable(getattr(value, field.name)) for field in fields(value)}
    slots = getattr(type(value), "__slots__", ())
    if isinstance(slots, str):
        slots = (slots,)
    if slots:
        return {name: jsonable(getattr(value, name)) for name in slots if hasattr(value, name)}
    if hasattr(value, "__dict__"):
        return jsonable(vars(value))
    return str(value)
