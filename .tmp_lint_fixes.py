#!/usr/bin/env python3
"""Batch fixes for ruff lint errors."""

from pathlib import Path

REPLACEMENTS = [
    # (path, old, new)
    (
        "src/sin_code_bundle/agent_engine/__init__.py",
        "    _heuristic_rule,\n)\nfrom .distiller import (\n    _signature as _rule_signature,\n)",
        "    _heuristic_rule,  # noqa: F401\n)\nfrom .distiller import (\n    _signature as _rule_signature,  # noqa: F401\n)",
    ),
    (
        "src/sin_code_bundle/agent_engine/delegate.py",
        '{"delegation_id": l.delegation_id, "goal": l.goal[:80],\n             "depth": l.depth,\n             "age_s": round(time.monotonic() - l.started_at, 1)}\n            for l in self._leases.values()',
        '{"delegation_id": lease.delegation_id, "goal": lease.goal[:80],\n             "depth": lease.depth,\n             "age_s": round(time.monotonic() - lease.started_at, 1)}\n            for lease in self._leases.values()',
    ),
    (
        "src/sin_code_bundle/agent_engine/distiller.py",
        'f"- {l}" for l in lessons[:8]',
        'f"- {lesson}" for lesson in lessons[:8]',
    ),
    (
        "src/sin_code_bundle/agent_engine/loop.py",
        "from .types import AgentTask, StepResult, Verdict",
        "from .types import AgentTask, StepResult, Verdict, VerdictKind",
    ),
    (
        "src/sin_code_bundle/agent_engine/synthesizer.py",
        "from .planner import Planner\nfrom .types import AgentTask",
        "from .planner import Planner\nfrom .types import AgentTask\nfrom .distiller import KnowledgeDistiller  # noqa: F401",
    ),
    (
        "src/sin_code_bundle/cli.py",
        "import yaml\n            runner.export_docker_compose(output)",
        "import yaml  # noqa: F401\n            runner.export_docker_compose(output)",
    ),
    (
        "src/sin_code_bundle/cli.py",
        "    files = [l for l in out.splitlines() if l][:max_files * 4]",
        "    files = [line for line in out.splitlines() if line][:max_files * 4]",
    ),
    (
        "src/sin_code_bundle/cli.py",
        "dirty = [l for l in out.splitlines() if l.strip()]",
        "dirty = [line for line in out.splitlines() if line.strip()]",
    ),
    (
        "src/sin_delegate/doctor.py",
        "dirty = [l for l in out.splitlines() if l.strip()]",
        "dirty = [line for line in out.splitlines() if line.strip()]",
    ),
    (
        "src/sin_delegate/planner.py",
        "    files = [l for l in out.splitlines() if l][:max_files * 4]",
        "    files = [line for line in out.splitlines() if line][:max_files * 4]",
    ),
    (
        "tests/agent_engine/test_delegate_v2.py",
        'assert all(len(l) <= 300 for l in clean["lessons"])',
        'assert all(len(lesson) <= 300 for lesson in clean["lessons"])',
    ),
    (
        "tests/agent_engine/test_tracing_and_distiller.py",
        "recs = [json.loads(l) for l in log.read_text().splitlines()]",
        "recs = [json.loads(line) for line in log.read_text().splitlines()]",
    ),
    (
        "tests/agent_engine/test_tracing_and_distiller.py",
        'assert any("fail_lint" in l for l in lessons)',
        'assert any("fail_lint" in lesson for lesson in lessons)',
    ),
]

root = Path("/Users/jeremy/dev/SIN-Code-Bundle")

for rel, old, new in REPLACEMENTS:
    path = root / rel
    text = path.read_text()
    if old not in text:
        print(f"SKIP (not found): {rel}")
        continue
    text = text.replace(old, new)
    path.write_text(text)
    print(f"FIXED: {rel}")

# Path import in test_intelligence_multirepo.py
path = root / "tests/test_intelligence_multirepo.py"
text = path.read_text()
if "from pathlib import Path" not in text:
    text = text.replace(
        "import time\n\nimport pytest", "import time\nfrom pathlib import Path\n\nimport pytest"
    )
    path.write_text(text)
    print("FIXED: tests/test_intelligence_multirepo.py (Path import)")
else:
    print("SKIP: tests/test_intelligence_multirepo.py (Path import already present)")

# slash __init__.py: add to __all__ or use noqa
path = root / "src/sin_code_bundle/tools/slash/__init__.py"
text = path.read_text()
# Use noqa approach on each import line
if "# noqa: F401" not in text:
    text = text.replace(
        "from .commands import BUILTIN_COMMANDS, get_command_help",
        "from .commands import BUILTIN_COMMANDS, get_command_help  # noqa: F401",
    )
    text = text.replace(
        "from .dispatcher import CommandDispatcher, DispatchResult",
        "from .dispatcher import CommandDispatcher, DispatchResult  # noqa: F401",
    )
    text = text.replace(
        "from .executor import CommandExecutor",
        "from .executor import CommandExecutor  # noqa: F401",
    )
    text = text.replace(
        "from .parser import ParsedCommand, SlashParser",
        "from .parser import ParsedCommand, SlashParser  # noqa: F401",
    )
    text = text.replace(
        "from .registry import CommandRegistry, CustomCommand",
        "from .registry import CommandRegistry, CustomCommand  # noqa: F401",
    )
    path.write_text(text)
    print("FIXED: src/sin_code_bundle/tools/slash/__init__.py")
else:
    print("SKIP: src/sin_code_bundle/tools/slash/__init__.py")

# cli.py: except ImportError as exc and exc=exc defaults
path = root / "src/sin_code_bundle/cli.py"
text = path.read_text()
# There are two blocks; replace all occurrences
for old, new in [
    (
        'except ImportError:\n    @app.command("slash")\n    def slash_missing() -> None:',
        'except ImportError as exc:\n    @app.command("slash")\n    def slash_missing(exc=exc) -> None:',
    ),
    (
        'except ImportError:\n    @app.command("mcp-server")\n    def mcp_server_missing() -> None:',
        'except ImportError as exc:\n    @app.command("mcp-server")\n    def mcp_server_missing(exc=exc) -> None:',
    ),
    (
        'except ImportError:\n    @app.command("marketplace")\n    def marketplace_missing() -> None:',
        'except ImportError as exc:\n    @app.command("marketplace")\n    def marketplace_missing(exc=exc) -> None:',
    ),
]:
    if old in text:
        text = text.replace(old, new)
        print(f"FIXED: {old.splitlines()[0]}")
    else:
        print(f"SKIP: {old.splitlines()[0]}")
# Remove duplicate commands
for block in [
    '@app.command()\ndef ibd():\n    """Intent-Based Diffing (IBD) — thin wrapper around the `ibd` binary."""\n    _forward_to_binary("ibd", _NEW_TOOL_BINARIES["ibd"][0])\n\n',
    '@app.command()\ndef poc():\n    """Proof-of-Correctness (POC) — thin wrapper around the `poc` binary."""\n    _forward_to_binary("poc", _NEW_TOOL_BINARIES["poc"][0])\n\n',
    '@app.command()\ndef adw():\n    """Architectural Debt Watchdogs (ADW) — thin wrapper around the `adw` binary."""\n    _forward_to_binary("adw", _NEW_TOOL_BINARIES["adw"][0])\n\n',
    '@app.command()\ndef oracle():\n    """Verification Oracle — thin wrapper around the `oracle` binary."""\n    _forward_to_binary("oracle", _NEW_TOOL_BINARIES["oracle"][0])\n\n',
    '@app.command()\ndef efm():\n    """Ephemeral Full-Stack Mocking (EFM) — thin wrapper around the `efm` binary."""\n    _forward_to_binary("efm", _NEW_TOOL_BINARIES["efm"][0])\n\n',
]:
    if text.count(block) > 1:
        text = text.replace(block, "", 1)
        print("FIXED: removed duplicate command block")
    else:
        print("SKIP: duplicate command block (only one)")
path.write_text(text)

print("done")
