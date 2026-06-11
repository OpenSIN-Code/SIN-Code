# SPDX-License-Identifier: MIT
"""Structured JSONL telemetry + live counters."""

from __future__ import annotations

import json
import os
import threading
import time
from collections import Counter
from pathlib import Path
from typing import Any


class Telemetry:
    def __init__(self, log_path: str | None = None, *, echo: bool = False) -> None:
        env_path = os.environ.get("SIN_AGENT_LOG", "")
        self.log_path = Path(log_path or env_path
                            or Path.home() / ".sin" / "agent-events.jsonl")
        self.log_path.parent.mkdir(parents=True, exist_ok=True)
        self.echo = echo
        self.counters: Counter[str] = Counter()
        self._lock = threading.Lock()
        self._t0 = time.monotonic()

    def emit(self, event: str, **fields: Any) -> None:
        record = {
            "ts": round(time.time(), 3),
            "rel_s": round(time.monotonic() - self._t0, 3),
            "event": event,
            **fields,
        }
        line = json.dumps(record, default=str, ensure_ascii=False)
        with self._lock:
            self.counters[event] += 1
            with self.log_path.open("a", encoding="utf-8") as fh:
                fh.write(line + "\n")
        if self.echo:
            print(f"[sin-agent] {line}")

    def summary(self) -> dict[str, Any]:
        return {
            "elapsed_s": round(time.monotonic() - self._t0, 1),
            "events": dict(self.counters),
            "log": str(self.log_path),
        }
