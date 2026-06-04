"""Purpose: DAP runtime bridge for SIN-Code — attach debuggers, store runtime facts.

Docs: dap_bridge.doc.md
"""

from __future__ import annotations

import subprocess
from pathlib import Path
from typing import Optional


class DAPSession:
    """Manages a single DAP debugging session."""

    def __init__(self, language: str, target: str, repo_root: Path):
        self.language = language
        self.target = target
        self.repo_root = repo_root
        self.process: Optional[subprocess.Popen] = None
        self.port: Optional[int] = None

    def start(self) -> dict:
        try:
            if self.language == "python":
                self.port = 5678  # debugpy default port (https://github.com/microsoft/debugpy)
                self.process = subprocess.Popen(
                    [
                        "python",
                        "-m",
                        "debugpy",
                        "--listen",
                        str(self.port),
                        "--wait-for-client",
                        self.target,
                    ],
                    cwd=self.repo_root,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )
            elif self.language == "go":
                self.port = 2345  # delve default headless port
                self.process = subprocess.Popen(
                    [
                        "dlv",
                        "debug",
                        "--headless",
                        "--listen",
                        f":{self.port}",
                        "--api-version=2",
                        self.target,
                    ],
                    cwd=self.repo_root,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )
            elif self.language in ("node", "javascript", "typescript"):
                self.port = 9229  # node --inspect default port
                self.process = subprocess.Popen(
                    ["node", f"--inspect-brk={self.port}", self.target],
                    cwd=self.repo_root,
                    stdout=subprocess.PIPE,
                    stderr=subprocess.PIPE,
                )
            else:
                return {"error": f"Unsupported language for DAP: {self.language}"}
            return {
                "success": True,
                "port": self.port,
                "message": f"Debugger attached on port {self.port}",
            }
        except FileNotFoundError:
            return {"error": f"Debugger for {self.language} not found (install debugpy/dlv/node)."}
        except Exception as e:
            return {"error": str(e)}

    def stop(self) -> None:
        if self.process:
            try:
                self.process.terminate()
            except Exception:
                pass
            self.process = None


# ── SINRuntimeTrace: High-level Orchestrator ───────────────────────────────
class SINRuntimeTrace:
    """High-level runtime tracing orchestrator."""

    def __init__(self, repo_root: Optional[Path] = None):
        self.repo_root = repo_root or Path.cwd()
        self.sessions: dict[str, DAPSession] = {}

    def trace_function(
        self,
        file_path: str,
        function_name: str,
        language: str = "python",
        store_in_memory: bool = True,
    ) -> dict:
        session_id = f"{language}_{function_name}"
        session = DAPSession(language, file_path, self.repo_root)
        result = session.start()
        if not result.get("success"):
            return result
        self.sessions[session_id] = session
        if store_in_memory:
            try:
                from sin_code_bundle import memory

                memory.remember(
                    f"Runtime trace initiated for {function_name} in {file_path} on port {result['port']}",
                    kind="runtime",
                    scope="repo",
                )
            except Exception:
                pass
        return {
            "success": True,
            "session_id": session_id,
            "port": result["port"],
            "message": f"Attach DAP client to localhost:{result['port']} to inspect {function_name}",
        }

    def get_session_status(self, session_id: str) -> dict:
        if session_id in self.sessions:
            return {"active": True, "port": self.sessions[session_id].port}
        return {"active": False, "error": "Session not found"}

    def stop_trace(self, session_id: str) -> dict:
        if session_id in self.sessions:
            self.sessions[session_id].stop()
            del self.sessions[session_id]
            return {"success": True, "message": "Session terminated"}
        return {"error": "Session not found"}
