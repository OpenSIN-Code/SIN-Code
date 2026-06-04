# Usage — EFSM

The package installs the `efsm` command.

## `efsm setup <name>`

Prepare an ephemeral environment and run a test command against it.

```bash
efsm setup checkout-task \
  --api stripe \
  --api github \
  --db \
  --test-cmd "pytest -q"
```

Flags:

| Flag | Repeatable | Description |
|------|-----------|-------------|
| `--api <name>` | yes | Register a stateful mock for an external API. Sets `<NAME>_BASE_URL`. |
| `--db` | no | Provide an in-memory database DSN via `DATABASE_URL`. |
| `--test-cmd <cmd>` | no | Command to run inside the environment (default `pytest`). |

## Python API

```python
from sin_code_efsm import EphemeralOrchestrator, StatefulMock

orch = EphemeralOrchestrator()
env = orch.configure_from_task({
    "name": "checkout",
    "external_apis": ["stripe"],
    "requires_db": True,
    "test_command": "pytest",
})
print(env.env_vars)   # STRIPE_BASE_URL, DATABASE_URL, ...
result = orch.run_tests("pytest -q")
print(result["success"])
```

### Stateful mock behavior

```python
mock = StatefulMock(name="stripe", base_path="/stripe")
mock.respond("POST", "/stripe/charges", {"amount": 100})
# -> {"status": "created", "id": 1, "data": {"amount": 100}}
mock.respond("GET", "/stripe/charges")
# -> {"data": [{"amount": 100}]}
```

Use the `scenarios` dict to script exact responses for specific
`METHOD:path` keys.

## Running the mock server standalone

```python
from sin_code_efsm import MockServer, StatefulMock

server = MockServer()
server.add_mock(StatefulMock(name="github", base_path="/github"))
server.run(host="127.0.0.1", port=8787)
```
