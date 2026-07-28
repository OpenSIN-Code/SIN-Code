# SPDX-License-Identifier: MIT
"""Tests for sin_code_bundle.config — layered config store.

Covers: TOML loading, JSON (opencode) loading, env-var overrides,
redaction, dotted-key resolution, merged view, set/unset round-trips,
and formatting helpers.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from sin_code_bundle.config import (
    MISSING,
    REDACTED_PLACEHOLDER,
    ConfigSource,
    ConfigView,
    Missing,
    _dig,
    _is_sensitive,
    _read_env,
    _read_opencode_json,
    _read_sources,
    _read_toml,
    all_paths,
    format_path,
    format_show,
    get,
    merged,
    redact,
)

# tomli_w is an optional write-time dependency; skip set/unset tests
# when it is not installed.
tomli_w = pytest.importorskip("tomli_w", reason="tomli_w not installed")
from sin_code_bundle.config import set_value, unset_value  # noqa: E402

# ════════════════════════════════════════════════════════════════════════
# Sentinel
# ════════════════════════════════════════════════════════════════════════


class TestMissingSentinel:
    def test_missing_is_singleton(self):
        assert Missing() is MISSING
        assert Missing() is Missing()

    def test_missing_repr(self):
        assert "MISSING" in repr(MISSING())

    def test_missing_distinct_from_none(self):
        assert MISSING is not None
        assert MISSING != "unset"


# ════════════════════════════════════════════════════════════════════════
# TOML loading
# ════════════════════════════════════════════════════════════════════════


class TestReadToml:
    def test_nonexistent_returns_empty(self, tmp_path: Path):
        assert _read_toml(tmp_path / "nope.toml") == {}

    def test_valid_toml(self, sample_toml: Path):
        data = _read_toml(sample_toml)
        assert data["tui"]["theme"] == "dark"
        assert data["opencode"]["model"] == "gpt-4o"

    def test_corrupted_toml_returns_empty(self, tmp_path: Path):
        p = tmp_path / "bad.toml"
        p.write_text("this is = = not valid toml [[[", encoding="utf-8")
        assert _read_toml(p) == {}

    def test_empty_toml_returns_empty(self, tmp_path: Path):
        p = tmp_path / "empty.toml"
        p.write_text("", encoding="utf-8")
        assert _read_toml(p) == {}


# ════════════════════════════════════════════════════════════════════════
# OpenCode JSON loading
# ════════════════════════════════════════════════════════════════════════


class TestReadOpencodeJson:
    def test_nonexistent_returns_empty(self, tmp_path: Path):
        assert _read_opencode_json(tmp_path / "nope.json") == {}

    def test_extracts_sin_subobject(self, sample_opencode_json: Path):
        data = _read_opencode_json(sample_opencode_json)
        assert data == {"model": "claude-3.5-sonnet"}

    def test_no_sin_key_returns_empty(self, tmp_path: Path):
        p = tmp_path / "no_sin.json"
        p.write_text('{"other": {"foo": "bar"}}', encoding="utf-8")
        assert _read_opencode_json(p) == {}

    def test_sin_not_dict_returns_empty(self, tmp_path: Path):
        p = tmp_path / "sin_scalar.json"
        p.write_text('{"sin": "not-a-dict"}', encoding="utf-8")
        assert _read_opencode_json(p) == {}

    def test_corrupted_json_returns_empty(self, tmp_path: Path):
        p = tmp_path / "bad.json"
        p.write_text("{not valid json", encoding="utf-8")
        assert _read_opencode_json(p) == {}


# ════════════════════════════════════════════════════════════════════════
# Environment variable parsing
# ════════════════════════════════════════════════════════════════════════


class TestReadEnv:
    def test_no_env_returns_empty(self, env_clean):
        assert _read_env() == {}

    def test_simple_env_var(self, env_clean):
        env_clean.setenv("SIN_THEME", "dark")
        assert _read_env() == {"theme": "dark"}

    def test_doubledash_section_separator(self, env_clean):
        env_clean.setenv("SIN_TUI__THEME", "dark")
        result = _read_env()
        assert result == {"tui": {"theme": "dark"}}

    def test_multiple_env_vars(self, env_clean):
        env_clean.setenv("SIN_TUI__THEME", "dark")
        env_clean.setenv("SIN_TUI__HISTORY_SIZE", "1000")
        env_clean.setenv("SIN_OPENCODE__MODEL", "gpt-4o")
        result = _read_env()
        assert result["tui"]["theme"] == "dark"
        assert result["tui"]["history_size"] == "1000"
        assert result["opencode"]["model"] == "gpt-4o"

    def test_non_sin_env_ignored(self, env_clean):
        env_clean.setenv("PATH", "/usr/bin")
        env_clean.setenv("HOME", "/tmp")
        env_clean.setenv("SIN_THEME", "dark")
        result = _read_env()
        assert "PATH" not in result
        assert "HOME" not in result
        assert result == {"theme": "dark"}

    def test_env_values_are_strings(self, env_clean):
        env_clean.setenv("SIN_TUI__HISTORY_SIZE", "1000")
        result = _read_env()
        assert isinstance(result["tui"]["history_size"], str)


# ════════════════════════════════════════════════════════════════════════
# Dotted-key resolution
# ════════════════════════════════════════════════════════════════════════


class TestDig:
    def test_top_level_scalar(self):
        assert _dig({"theme": "dark"}, "theme") == "dark"

    def test_nested_key(self):
        assert _dig({"tui": {"theme": "dark"}}, "tui.theme") == "dark"

    def test_missing_key(self):
        assert _dig({}, "tui.theme") is MISSING

    def test_missing_section(self):
        assert _dig({"other": 1}, "tui.theme") is MISSING

    def test_section_returns_dict(self):
        result = _dig({"tui": {"theme": "dark"}}, "tui")
        assert result == {"theme": "dark"}

    def test_scalar_key_conflict(self):
        # When the key exists as a scalar at top level, return it
        assert _dig({"theme": "dark"}, "theme") == "dark"


# ════════════════════════════════════════════════════════════════════════
# get() — the public resolution API
# ════════════════════════════════════════════════════════════════════════


class TestGet:
    def test_missing_key_returns_missing(self, env_clean, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        view = get("tui.theme", global_path=empty, opencode_path=empty, project_path=empty)
        assert view.value is MISSING
        assert view.source is None

    def test_global_source_resolved(self, env_clean, sample_toml: Path, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        view = get(
            "tui.theme",
            global_path=sample_toml,
            opencode_path=empty,
            project_path=empty,
        )
        assert view.value == "dark"
        assert view.source.label == "global"

    def test_project_overrides_global(self, env_clean, tmp_path: Path):
        global_toml = tmp_path / "global.toml"
        global_toml.write_text('[tui]\ntheme = "dark"\n', encoding="utf-8")
        project_toml = tmp_path / "project.toml"
        project_toml.write_text('[tui]\ntheme = "light"\n', encoding="utf-8")
        empty = tmp_path / "empty.json"
        view = get(
            "tui.theme",
            global_path=global_toml,
            opencode_path=empty,
            project_path=project_toml,
        )
        assert view.value == "light"
        assert view.source.label == "project"

    def test_env_overrides_all(self, env_clean, sample_toml: Path, tmp_path: Path):
        empty = tmp_path / "empty.json"
        env_clean.setenv("SIN_TUI__THEME", "solarized")
        view = get(
            "tui.theme",
            global_path=sample_toml,
            opencode_path=empty,
            project_path=sample_toml,
        )
        assert view.value == "solarized"
        assert view.source.label == "env"

    def test_opencode_source(self, env_clean, sample_opencode_json: Path, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        view = get(
            "model",
            global_path=empty,
            opencode_path=sample_opencode_json,
            project_path=empty,
        )
        assert view.value == "claude-3.5-sonnet"
        assert view.source.label == "opencode"

    def test_returns_config_view(self, env_clean, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        empty_json = tmp_path / "empty.json"
        view = get("x.y", global_path=empty, opencode_path=empty_json, project_path=empty)
        assert isinstance(view, ConfigView)
        assert view.key == "x.y"


# ════════════════════════════════════════════════════════════════════════
# Redaction
# ════════════════════════════════════════════════════════════════════════


class TestRedaction:
    def test_is_sensitive_api_key(self):
        assert _is_sensitive("opencode.api_key") is True

    def test_is_sensitive_token(self):
        assert _is_sensitive("auth.token") is True

    def test_is_sensitive_password(self):
        assert _is_sensitive("db.password") is True

    def test_is_sensitive_secret(self):
        assert _is_sensitive("app.secret") is True

    def test_is_sensitive_private_key(self):
        assert _is_sensitive("ssh.private_key") is True

    def test_is_not_sensitive_model(self):
        assert _is_sensitive("opencode.model") is False

    def test_is_not_sensitive_theme(self):
        assert _is_sensitive("tui.theme") is False

    def test_redact_dict(self):
        payload = {"api_key": "sk-123", "model": "gpt-4o"}
        result = redact(payload)
        assert result["api_key"] == REDACTED_PLACEHOLDER
        assert result["model"] == "gpt-4o"

    def test_redact_nested_dict(self):
        payload = {"opencode": {"api_key": "sk-123", "model": "gpt-4o"}}
        result = redact(payload)
        assert result["opencode"]["api_key"] == REDACTED_PLACEHOLDER
        assert result["opencode"]["model"] == "gpt-4o"

    def test_redact_list(self):
        payload = [{"api_key": "sk-1"}, {"model": "x"}]
        result = redact(payload)
        assert result[0]["api_key"] == REDACTED_PLACEHOLDER
        assert result[1]["model"] == "x"

    def test_redact_scalar_passthrough(self):
        assert redact("hello") == "hello"
        assert redact(42) == 42
        assert redact(None) is None

    def test_redact_empty_dict(self):
        assert redact({}) == {}

    def test_redact_case_insensitive(self):
        payload = {"API_KEY": "sk-123"}
        result = redact(payload)
        assert result["API_KEY"] == REDACTED_PLACEHOLDER


# ════════════════════════════════════════════════════════════════════════
# set_value / unset_value round-trips (require tomli_w)
# ════════════════════════════════════════════════════════════════════════


class TestSetValue:
    def test_set_and_read_back(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        assert tmp_toml.exists()
        view = get("tui.theme", project_path=tmp_toml)
        assert view.value == "dark"

    def test_set_creates_parent_dirs(self, env_clean, tmp_path: Path):
        nested = tmp_path / "a" / "b" / "config.toml"
        set_value("tui.theme", "light", project_path=nested)
        assert nested.exists()
        assert get("tui.theme", project_path=nested).value == "light"

    def test_set_overwrites_existing(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        set_value("tui.theme", "light", project_path=tmp_toml)
        assert get("tui.theme", project_path=tmp_toml).value == "light"

    def test_set_multiple_keys_same_section(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        set_value("tui.history_size", "500", project_path=tmp_toml)
        assert get("tui.theme", project_path=tmp_toml).value == "dark"
        assert get("tui.history_size", project_path=tmp_toml).value == "500"

    def test_set_empty_key_raises(self, tmp_toml: Path):
        with pytest.raises(ValueError, match="non-empty"):
            set_value("", "x", project_path=tmp_toml)

    def test_set_top_level_key_raises(self, tmp_toml: Path):
        with pytest.raises(ValueError, match="top-level"):
            set_value("tui", "x", project_path=tmp_toml)


class TestUnsetValue:
    def test_unset_existing_key(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        assert unset_value("tui.theme", project_path=tmp_toml) is True
        assert get("tui.theme", project_path=tmp_toml).value is MISSING

    def test_unset_missing_key_returns_false(self, env_clean, tmp_toml: Path):
        assert unset_value("tui.theme", project_path=tmp_toml) is False

    def test_unset_nonexistent_file_returns_false(self, env_clean, tmp_path: Path):
        assert unset_value("tui.theme", project_path=tmp_path / "nope.toml") is False

    def test_unset_top_level_returns_false(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        assert unset_value("tui", project_path=tmp_toml) is False

    def test_unset_cleans_empty_section(self, env_clean, tmp_toml: Path):
        set_value("tui.theme", "dark", project_path=tmp_toml)
        unset_value("tui.theme", project_path=tmp_toml)
        data = _read_toml(tmp_toml)
        assert "tui" not in data


# ════════════════════════════════════════════════════════════════════════
# merged() — full layering
# ════════════════════════════════════════════════════════════════════════


class TestMerged:
    def test_empty_sources(self, env_clean, tmp_path: Path):
        empty_t = tmp_path / "empty.toml"
        empty_j = tmp_path / "empty.json"
        payload, origins = merged(
            global_path=empty_t,
            opencode_path=empty_j,
            project_path=empty_t,
        )
        assert payload == {}
        assert origins == {}

    def test_merge_layering(self, env_clean, tmp_path: Path):
        global_t = tmp_path / "global.toml"
        global_t.write_text('[tui]\ntheme = "dark"\nsize = 10\n', encoding="utf-8")
        project_t = tmp_path / "project.toml"
        project_t.write_text('[tui]\ntheme = "light"\n', encoding="utf-8")
        empty_j = tmp_path / "empty.json"
        payload, origins = merged(
            global_path=global_t,
            opencode_path=empty_j,
            project_path=project_t,
            redact_secrets=False,
        )
        assert payload["tui"]["theme"] == "light"  # project wins
        assert payload["tui"]["size"] == 10  # from global
        assert origins["tui.theme"].label == "project"
        assert origins["tui.size"].label == "global"

    def test_merged_redacts_by_default(self, env_clean, tmp_path: Path):
        global_t = tmp_path / "global.toml"
        global_t.write_text(
            '[opencode]\napi_key = "sk-secret"\nmodel = "gpt-4o"\n',
            encoding="utf-8",
        )
        empty_j = tmp_path / "empty.json"
        payload, _ = merged(
            global_path=global_t,
            opencode_path=empty_j,
            project_path=global_t,
        )
        assert payload["opencode"]["api_key"] == REDACTED_PLACEHOLDER
        assert payload["opencode"]["model"] == "gpt-4o"

    def test_merged_no_redact_when_disabled(self, env_clean, tmp_path: Path):
        global_t = tmp_path / "global.toml"
        global_t.write_text(
            '[opencode]\napi_key = "sk-secret"\n',
            encoding="utf-8",
        )
        empty_j = tmp_path / "empty.json"
        payload, _ = merged(
            global_path=global_t,
            opencode_path=empty_j,
            project_path=global_t,
            redact_secrets=False,
        )
        assert payload["opencode"]["api_key"] == "sk-secret"


# ════════════════════════════════════════════════════════════════════════
# all_paths()
# ════════════════════════════════════════════════════════════════════════


class TestAllPaths:
    def test_returns_four_sources(self, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        sources = all_paths(
            global_path=empty,
            opencode_path=empty,
            project_path=empty,
        )
        assert len(sources) == 4
        labels = [s.label for s in sources]
        assert labels == ["global", "opencode", "project", "env"]

    def test_priorities_ascending(self, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        sources = all_paths(
            global_path=empty,
            opencode_path=empty,
            project_path=empty,
        )
        priorities = [s.priority for s in sources]
        assert priorities == [0, 1, 2, 3]

    def test_env_source_always_exists(self, tmp_path: Path):
        empty = tmp_path / "empty.toml"
        sources = all_paths(
            global_path=empty,
            opencode_path=empty,
            project_path=empty,
        )
        env_src = sources[3]
        assert env_src.exists is True
        assert env_src.label == "env"


# ════════════════════════════════════════════════════════════════════════
# Formatting helpers
# ════════════════════════════════════════════════════════════════════════


class TestFormatShow:
    def test_empty_payload(self):
        assert format_show({}, {}) == "(no config set)"

    def test_dotted_keys_with_origin(self):
        payload = {"tui": {"theme": "dark"}}
        origins = {
            "tui.theme": ConfigSource(
                path=Path("x"),
                exists=True,
                priority=2,
                label="project",
            )
        }
        result = format_show(payload, origins)
        assert "tui.theme" in result
        assert "'dark'" in result
        assert "[project]" in result

    def test_scalar_top_level(self):
        payload = {"version": "1.0"}
        origins = {
            "version": ConfigSource(
                path=Path("x"),
                exists=True,
                priority=0,
                label="global",
            )
        }
        result = format_show(payload, origins)
        assert "version" in result
        assert "'1.0'" in result
        assert "[global]" in result


class TestFormatPath:
    def test_renders_all_sources(self):
        sources = [
            ConfigSource(path=Path("/a.toml"), exists=True, priority=0, label="global"),
            ConfigSource(path=Path("/b.json"), exists=False, priority=1, label="opencode"),
        ]
        result = format_path(sources)
        assert "[global ]" in result
        assert "/a.toml" in result
        assert "EXISTS" in result
        assert "[opencode]" in result
        assert "/b.json" in result
        assert "absent" in result


# ════════════════════════════════════════════════════════════════════════
# _read_sources() — internal ordering
# ════════════════════════════════════════════════════════════════════════


class TestReadSources:
    def test_four_sources_ascending_priority(self, env_clean, tmp_path: Path):
        empty_t = tmp_path / "empty.toml"
        empty_j = tmp_path / "empty.json"
        sources = _read_sources(
            global_path=empty_t,
            opencode_path=empty_j,
            project_path=empty_t,
        )
        assert len(sources) == 4
        priorities = [src.priority for src, _ in sources]
        assert priorities == [0, 1, 2, 3]

    def test_env_source_payload_populated(self, env_clean):
        env_clean.setenv("SIN_TUI__THEME", "dark")
        sources = _read_sources()
        env_payload = sources[3][1]
        assert env_payload == {"tui": {"theme": "dark"}}
