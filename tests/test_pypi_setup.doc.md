# tests/test_pypi_setup.py

## What
Unit tests for `sin_code_bundle.tools.pypi_setup` — the one-click PyPI
Trusted Publisher registration CLI.

## Coverage (28 tests, 5 classes)
- `TestNormaliseProjectName` (6 tests) — PEP 503 project-name
  normalisation: lowercase, `_`/`.`/mixed-separator collapsing.
- `TestPayloadBuilder` (5 tests) — `get_pending_publisher_payload`
  builds the correct JSON for PyPI's `_/v1/publisher` schema,
  including the `owner` with-`/` corner case and a schema-lock
  test that prevents accidental key additions.
- `TestArgParser` (3 tests) — CLI flags, defaults, required-field
  behaviour.
- `TestAddPendingPublisher` (7 tests) — HTTP layer with mocked
  `urlopen`: 201 success, 400/401/409 failures with body parsing,
  `URLError` network error, unexpected 2xx, and a request-shape
  assertion (URL, method, `Authorization: Token ...` header,
  JSON body, timeout propagation).
- `TestMain` (7 tests) — End-to-end through `main()`: dry-run
  (no HTTP), dry-run + JSON, success path (human + JSON),
  failure path (human + JSON), and a parameter round-trip.

## No real PyPI calls
Every HTTP interaction is mocked. Running this suite does not
require network access and does not register a real pending
publisher.
