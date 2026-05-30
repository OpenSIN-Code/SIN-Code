from sin_code_bundle.budget import trim


def test_trims_long_list():
    out = trim(list(range(100)), max_list=10)
    assert len(out) == 11  # 10 items + truncation marker
    assert out[-1]["_truncated"] is True
    assert out[-1]["_omitted"] == 90


def test_short_list_unchanged():
    out = trim([1, 2, 3], max_list=10)
    assert out == [1, 2, 3]


def test_trims_long_string():
    out = trim("x" * 5000, max_str=100)
    assert out.endswith("...[truncated]")
    assert len(out) <= 120


def test_short_string_unchanged():
    out = trim("hello", max_str=100)
    assert out == "hello"


def test_nested_dict():
    out = trim({"items": list(range(50)), "name": "ok"}, max_list=5)
    assert len(out["items"]) == 6  # 5 items + marker
    assert out["name"] == "ok"


def test_passthrough_int():
    assert trim(42) == 42


def test_passthrough_none():
    assert trim(None) is None


def test_nested_list_of_dicts():
    data = [{"a": "x" * 5000}]
    out = trim(data, max_list=5, max_str=10)
    assert out[0]["a"].endswith("...[truncated]")
