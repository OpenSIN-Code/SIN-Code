# SPDX-License-Identifier: MIT
"""Tests for sin_code_bundle.tools.pypi_setup.

Docs: test_pypi_setup.doc.md

These tests cover the pure-Python parts of the module (payload building,
PEP 503 normalisation, arg parsing) and the HTTP layer via a fake
``urlopen`` — no real PyPI calls are made.
"""

from __future__ import annotations

import io
import json
from typing import Any, Dict
from unittest import mock

import pytest

from sin_code_bundle.tools import pypi_setup

# ── normalise_project_name ─────────────────────────────────────────────────


class TestNormaliseProjectName:
    """PEP 503 normalisation: lowercase + collapse [-_.]+ to single -."""

    def test_lowercase(self) -> None:
        assert pypi_setup.normalise_project_name("MyPackage") == "mypackage"

    def test_underscore_to_dash(self) -> None:
        assert pypi_setup.normalise_project_name("my_package") == "my-package"

    def test_dot_to_dash(self) -> None:
        assert pypi_setup.normalise_project_name("my.package") == "my-package"

    def test_mixed_separators_collapse(self) -> None:
        # Multiple separators in a row collapse to one dash.
        assert pypi_setup.normalise_project_name("my__-_---package") == "my-package"

    def test_already_normalised(self) -> None:
        assert pypi_setup.normalise_project_name("sin-code-bundle") == "sin-code-bundle"

    def test_empty_string(self) -> None:
        # Edge case — should not crash.
        assert pypi_setup.normalise_project_name("") == ""


# ── get_pending_publisher_payload ───────────────────────────────────────────


class TestPayloadBuilder:
    """The payload that gets POSTed to PyPI's _/v1/publisher endpoint."""

    def test_default_fields(self) -> None:
        p = pypi_setup.get_pending_publisher_payload(
            "sin-code-bundle",
            "OpenSIN-Code",
            "SIN-Code-Bundle",
            "release.yml",
            "pypi",
        )
        assert p == {
            "name": "sin-code-bundle",
            "owner": "OpenSIN-Code",
            "repository": "SIN-Code-Bundle",
            "workflow_filename": "release.yml",
            "environment": "pypi",
        }

    def test_uppercase_name_gets_normalised(self) -> None:
        # PyPI requires the normalised form.
        p = pypi_setup.get_pending_publisher_payload(
            "SIN-Code-Bundle",
            "OpenSIN-Code",
            "SIN-Code-Bundle",
            "release.yml",
            "pypi",
        )
        assert p["name"] == "sin-code-bundle"

    def test_owner_with_slash_takes_first_segment(self) -> None:
        # Some users mistakenly pass 'org/repo' as owner — strip the
        # extra segment to keep the payload schema-valid.
        p = pypi_setup.get_pending_publisher_payload(
            "p",
            "OpenSIN-Code/SIN-Code-Bundle",
            "SIN-Code-Bundle",
            "release.yml",
            "pypi",
        )
        assert p["owner"] == "OpenSIN-Code"

    def test_custom_environment(self) -> None:
        # `pypi-test` is the test-PyPI equivalent of `pypi`.
        p = pypi_setup.get_pending_publisher_payload(
            "sin-code-bundle",
            "OpenSIN-Code",
            "SIN-Code-Bundle",
            "release.yml",
            "pypi-test",
        )
        assert p["environment"] == "pypi-test"

    def test_payload_has_no_extra_keys(self) -> None:
        p = pypi_setup.get_pending_publisher_payload(
            "p",
            "o",
            "r",
            "w",
            "e",
        )
        # Lock the schema — a future maintainer who adds a new key must
        # update this test deliberately.
        assert set(p.keys()) == {
            "name",
            "owner",
            "repository",
            "workflow_filename",
            "environment",
        }


# ── build_argparser ────────────────────────────────────────────────────────


class TestArgParser:
    """CLI argument surface."""

    def test_required_flag_is_api_token(self) -> None:
        parser = pypi_setup.build_argparser()
        with pytest.raises(SystemExit):
            # No --api-token → argparse exits with error.
            parser.parse_args([])

    def test_all_defaults(self) -> None:
        parser = pypi_setup.build_argparser()
        args = parser.parse_args(["--api-token", "pypi-X"])
        assert args.project == "sin-code-bundle"
        assert args.owner == "OpenSIN-Code"
        assert args.repo == "SIN-Code-Bundle"
        assert args.workflow == "release.yml"
        assert args.environment == "pypi"
        assert args.api_token == "pypi-X"
        assert args.timeout == 15.0
        assert args.dry_run is False
        assert args.as_json is False

    def test_overrides(self) -> None:
        parser = pypi_setup.build_argparser()
        args = parser.parse_args(
            [
                "--project",
                "x",
                "--owner",
                "y",
                "--repo",
                "z",
                "--workflow",
                "ci.yml",
                "--environment",
                "test",
                "--api-token",
                "t",
                "--timeout",
                "30",
                "--dry-run",
                "--json",
            ]
        )
        assert args.project == "x"
        assert args.owner == "y"
        assert args.repo == "z"
        assert args.workflow == "ci.yml"
        assert args.environment == "test"
        assert args.api_token == "t"
        assert args.timeout == 30.0
        assert args.dry_run is True
        assert args.as_json is True


# ── add_pending_publisher (HTTP layer, mocked) ─────────────────────────────


class _FakeResponse:
    """Drop-in replacement for ``urllib.response.addinfourl``."""

    def __init__(self, status: int, body: str) -> None:
        self.status = status
        self._body = body.encode("utf-8")

    def read(self) -> bytes:
        return self._body

    def __enter__(self) -> "_FakeResponse":
        return self

    def __exit__(self, *args: Any) -> None:
        pass


class TestAddPendingPublisher:
    """HTTP behaviour: 201 success, 4xx/5xx errors, network errors, timeouts."""

    def test_201_is_success(self) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            return_value=_FakeResponse(201, '{"ok": true}'),
        ):
            ok, msg = pypi_setup.add_pending_publisher(
                "pypi-X",
                {
                    "name": "p",
                    "owner": "o",
                    "repository": "r",
                    "workflow_filename": "w",
                    "environment": "e",
                },
            )
        assert ok is True
        assert "Pending publisher created" in msg

    def test_400_is_failure_with_body(self) -> None:
        err_body = '{"errors": [{"code": "project_not_found"}]}'
        error = pypi_setup.urllib.error.HTTPError(
            "https://pypi.org/_/v1/publisher",
            400,
            "Bad Request",
            {"Content-Type": "application/json"},
            io.BytesIO(err_body.encode("utf-8")),
        )
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=error,
        ):
            ok, msg = pypi_setup.add_pending_publisher("t", {"k": "v"})
        assert ok is False
        assert "400" in msg
        assert "project_not_found" in msg

    def test_409_is_failure(self) -> None:
        err_body = '{"errors": [{"code": "already_exists"}]}'
        error = pypi_setup.urllib.error.HTTPError(
            "https://pypi.org/_/v1/publisher",
            409,
            "Conflict",
            {"Content-Type": "application/json"},
            io.BytesIO(err_body.encode("utf-8")),
        )
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=error,
        ):
            ok, _ = pypi_setup.add_pending_publisher("t", {"k": "v"})
        assert ok is False

    def test_401_auth_failure(self) -> None:
        err_body = '{"errors": [{"code": "invalid_token"}]}'
        error = pypi_setup.urllib.error.HTTPError(
            "https://pypi.org/_/v1/publisher",
            401,
            "Unauthorized",
            {"Content-Type": "application/json"},
            io.BytesIO(err_body.encode("utf-8")),
        )
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=error,
        ):
            ok, msg = pypi_setup.add_pending_publisher("t", {"k": "v"})
        assert ok is False
        assert "401" in msg

    def test_network_error(self) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=pypi_setup.urllib.error.URLError("DNS failure"),
        ):
            ok, msg = pypi_setup.add_pending_publisher("t", {"k": "v"})
        assert ok is False
        assert "Network error" in msg

    def test_unexpected_2xx_status(self) -> None:
        # PyPI should return 201; anything else is treated as a failure.
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            return_value=_FakeResponse(200, ""),
        ):
            ok, msg = pypi_setup.add_pending_publisher("t", {"k": "v"})
        assert ok is False
        assert "200" in msg

    def test_request_payload_format(self) -> None:
        # The body sent to PyPI must be JSON, the headers must include
        # the API token in the right scheme, and the method must be POST.
        captured: Dict[str, Any] = {}

        def fake_urlopen(req: Any, timeout: float = 0) -> _FakeResponse:
            captured["url"] = req.full_url
            captured["method"] = req.method
            captured["headers"] = dict(req.headers)
            captured["body"] = req.data.decode("utf-8")
            captured["timeout"] = timeout
            return _FakeResponse(201, "{}")

        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=fake_urlopen,
        ):
            pypi_setup.add_pending_publisher(
                "pypi-DEADBEEF",
                {"name": "p", "owner": "o"},
                timeout=5.0,
                base_url="https://example.test",
            )

        assert captured["url"] == "https://example.test/_/v1/publisher"
        assert captured["method"] == "POST"
        # urllib normalises header keys to title case ("Content-Type",
        # "Authorization"). Compare case-insensitively to be robust.
        lowered = {k.lower(): v for k, v in captured["headers"].items()}
        assert lowered["authorization"] == "Token pypi-DEADBEEF"
        assert lowered["content-type"] == "application/json"
        # Body must be valid JSON.
        body = json.loads(captured["body"])
        assert body == {"name": "p", "owner": "o"}
        assert captured["timeout"] == 5.0


# ── main() integration (no real HTTP) ───────────────────────────────────────


class TestMain:
    """End-to-end through ``main()`` with a mocked urlopen."""

    def test_dry_run_exits_zero_without_http(self, capsys: pytest.CaptureFixture) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
        ) as mocked:
            rc = pypi_setup.main(["--api-token", "pypi-X", "--dry-run"])
        # No HTTP call in dry-run.
        mocked.assert_not_called()
        assert rc == 0
        out = capsys.readouterr().out
        assert "sin-code-bundle" in out

    def test_dry_run_json_is_single_line(self, capsys: pytest.CaptureFixture) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
        ):
            rc = pypi_setup.main(
                [
                    "--api-token",
                    "pypi-X",
                    "--dry-run",
                    "--json",
                ]
            )
        assert rc == 0
        out = capsys.readouterr().out.strip()
        # Single JSON object on a single line.
        data = json.loads(out)
        assert data["dry_run"] is True
        assert data["payload"]["name"] == "sin-code-bundle"

    def test_success_prints_next_steps(self, capsys: pytest.CaptureFixture) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            return_value=_FakeResponse(201, "{}"),
        ):
            rc = pypi_setup.main(["--api-token", "pypi-X"])
        assert rc == 0
        out = capsys.readouterr().out
        assert "Pending publisher created" in out
        assert "OpenSIN-Code/SIN-Code-Bundle" in out or "OpenSIN-Code" in out
        # The next-step block mentions the email + magic link.
        assert "email" in out.lower() or "magic link" in out.lower()

    def test_success_json_contains_success_flag(self, capsys: pytest.CaptureFixture) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            return_value=_FakeResponse(201, "{}"),
        ):
            rc = pypi_setup.main(["--api-token", "pypi-X", "--json"])
        assert rc == 0
        data = json.loads(capsys.readouterr().out.strip())
        assert data["success"] is True
        assert "payload" in data

    def test_failure_exits_one_and_prints_fallback(self, capsys: pytest.CaptureFixture) -> None:
        err = pypi_setup.urllib.error.HTTPError(
            "https://pypi.org/_/v1/publisher",
            409,
            "Conflict",
            {},
            io.BytesIO(b"already exists"),
        )
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=err,
        ):
            rc = pypi_setup.main(["--api-token", "pypi-X"])
        assert rc == 1
        # Fallback URL must be in the error stream.
        err_out = capsys.readouterr().err
        assert "pypi.org/manage/account/publishing" in err_out
        assert "OpenSIN-Code" in err_out

    def test_failure_json_includes_success_false(self, capsys: pytest.CaptureFixture) -> None:
        err = pypi_setup.urllib.error.HTTPError(
            "https://pypi.org/_/v1/publisher",
            401,
            "Unauthorized",
            {},
            io.BytesIO(b"bad token"),
        )
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            side_effect=err,
        ):
            rc = pypi_setup.main(["--api-token", "pypi-X", "--json"])
        assert rc == 1
        data = json.loads(capsys.readouterr().out.strip())
        assert data["success"] is False
        assert "401" in data["message"]

    def test_custom_repo_and_owner_flow_through(self, capsys: pytest.CaptureFixture) -> None:
        with mock.patch.object(
            pypi_setup.urllib.request,
            "urlopen",
            return_value=_FakeResponse(201, "{}"),
        ):
            rc = pypi_setup.main(
                [
                    "--project",
                    "my-pkg",
                    "--owner",
                    "MyOrg",
                    "--repo",
                    "MyRepo",
                    "--workflow",
                    "ci.yml",
                    "--environment",
                    "pypi-test",
                    "--api-token",
                    "pypi-X",
                ]
            )
        assert rc == 0
        out = capsys.readouterr().out
        assert "my-pkg" in out
        assert "MyOrg" in out
        assert "MyRepo" in out
        assert "ci.yml" in out
        assert "pypi-test" in out
