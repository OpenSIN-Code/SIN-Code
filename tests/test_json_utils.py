# SPDX-License-Identifier: MIT
from dataclasses import dataclass

from sin_code_bundle.json_utils import jsonable


@dataclass(slots=True)
class SlotResult:
    score: float
    factors: list[str]


class DictResult:
    def __init__(self) -> None:
        self.value = "ok"


class ToDictResult:
    def to_dict(self):
        return {"nested": SlotResult(0.5, ["change"])}


def test_jsonable_handles_current_ibd_result_shapes():
    assert jsonable("No semantic changes detected.") == "No semantic changes detected."
    assert jsonable(SlotResult(0.0, [])) == {"score": 0.0, "factors": []}


def test_jsonable_handles_nested_and_legacy_objects():
    assert jsonable(ToDictResult()) == {"nested": {"score": 0.5, "factors": ["change"]}}
    assert jsonable(DictResult()) == {"value": "ok"}
    assert sorted(jsonable({"b", "a"})) == ["a", "b"]
