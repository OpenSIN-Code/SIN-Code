# Configuration — EFSM

EFSM is configured per task (via the CLI or the orchestrator API) rather than
through a global config file.

## Task context fields

`EphemeralOrchestrator.configure_from_task(ctx)` accepts:

| Key | Type | Description |
|-----|------|-------------|
| `name` | str | Task name (labeling only). |
| `external_apis` | list[str] | One stateful mock per entry. Each sets `<NAME>_BASE_URL`. |
| `requires_db` | bool | If true, sets `DATABASE_URL` to an in-memory SQLite DSN. |
| `test_command` | str | Command executed inside the environment. |

## Generated environment variables

| Variable | When | Value |
|----------|------|-------|
| `<API>_BASE_URL` | per `external_apis` entry | URL pointing at the mock server route. |
| `DATABASE_URL` | when `requires_db` | `sqlite:///:memory:` |

## Execution backend

`DockerSandbox` is used when the Docker SDK and a running daemon are available.
Resource limits applied to containers:

| Limit | Value |
|-------|-------|
| memory | 512m |
| cpu_quota | 50000 |
| network (isolated runs) | `none` |

When Docker is unavailable, EFSM transparently falls back to a **subprocess**
runner with a timeout. This is convenient but is **not** a security sandbox for
untrusted code — use Docker for that.

## Mock server bind address

`MockServer.run(host, port)` defaults to `127.0.0.1:8787`. Change these
arguments to bind elsewhere.
