from sin_code_efsm.mock_generator import MockServer, StatefulMock
from sin_code_efsm.orchestrator import EphemeralOrchestrator
from sin_code_efsm.sandbox import DockerSandbox, docker_available


def test_stateful_mock_post_then_get():
    mock = StatefulMock(name="github", base_path="/github")
    created = mock.respond("POST", "/github/repos", {"name": "demo"})
    assert created["status"] == "created"
    listing = mock.respond("GET", "/github/repos")
    assert listing["data"][0]["name"] == "demo"


def test_mock_server_dispatch_routing():
    server = MockServer()
    server.add_mock(StatefulMock(name="stripe", base_path="/stripe"))
    server.add_mock(StatefulMock(name="github", base_path="/github"))
    res = server.dispatch("POST", "/stripe/charges", {"amount": 100})
    assert res["status"] == "created"
    miss = server.dispatch("GET", "/unknown/x")
    assert miss.get("error") == "no mock matched"


def test_mock_server_scenario_override():
    mock = StatefulMock(
        name="api",
        base_path="/api",
        scenarios={"GET:/api/health": {"status": "degraded"}},
    )
    server = MockServer()
    server.add_mock(mock)
    assert server.dispatch("GET", "/api/health")["status"] == "degraded"


def test_orchestrator_configures_env_vars():
    orch = EphemeralOrchestrator()
    env = orch.configure_from_task(
        {"name": "t", "external_apis": ["stripe"], "requires_db": True}
    )
    assert "STRIPE_BASE_URL" in env.env_vars
    assert env.db_dsn == "sqlite:///:memory:"
    assert env.sandbox_backend in ("docker", "subprocess")


def test_sandbox_runs_command_via_fallback():
    # Funktioniert mit Docker ODER Subprozess-Fallback.
    sandbox = DockerSandbox()
    result = sandbox.run_command("echo hello-sin", timeout=30)
    assert result.exit_code == 0
    assert "hello-sin" in result.stdout
    assert result.backend in ("docker", "subprocess")


def test_docker_available_returns_bool():
    assert isinstance(docker_available(), bool)
