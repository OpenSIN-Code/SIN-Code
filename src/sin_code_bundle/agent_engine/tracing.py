# SPDX-License-Identifier: MIT
"""Delegation-Tracing — Span-Baum ueber den gesamten Delegationsbaum.

OpenTelemetry-inspiriert, zero-dependency, JSONL-nativ: jeder
Sub-Agent (und jeder Step) wird als Span mit trace_id/span_id/parent_span_id
erfasst. Der Trace laesst sich aus dem Telemetrie-Log vollstaendig
rekonstruieren — auch nach einem Crash, auch fuer historische Runs.

Propagation laeuft ueber das TraceContext-Objekt (pfad-lokal, parallel-
sicher — derselbe Mechanismus wie DelegationContext, bewusst KEIN
contextvars-Magic: explizit ist debugbarer als implizit).
"""

from __future__ import annotations

import json
import time
import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from .telemetry import Telemetry


@dataclass(slots=True)
class TraceContext:
    trace_id: str = field(default_factory=lambda: uuid.uuid4().hex[:16])
    span_id: str = field(default_factory=lambda: uuid.uuid4().hex[:12])
    parent_span_id: str | None = None

    def child(self) -> "TraceContext":
        return TraceContext(
            trace_id=self.trace_id,
            span_id=uuid.uuid4().hex[:12],
            parent_span_id=self.span_id,
        )


class SpanEmitter:
    """Duenne Schicht ueber Telemetry: span_start/span_end Events."""

    def __init__(self, telemetry: Telemetry) -> None:
        self.telemetry = telemetry

    def start(self, ctx: TraceContext, *, kind: str, name: str,
              **attrs: Any) -> float:
        self.telemetry.emit(
            "span_start", trace_id=ctx.trace_id, span_id=ctx.span_id,
            parent_span_id=ctx.parent_span_id, kind=kind,
            name=name[:120], **attrs,
        )
        return time.monotonic()

    def end(self, ctx: TraceContext, started: float, *,
            status: str = "ok", **attrs: Any) -> None:
        self.telemetry.emit(
            "span_end", trace_id=ctx.trace_id, span_id=ctx.span_id,
            duration_s=round(time.monotonic() - started, 3),
            status=status, **attrs,
        )


@dataclass(slots=True)
class Span:
    span_id: str
    parent_span_id: str | None
    kind: str
    name: str
    status: str = "running"
    duration_s: float | None = None
    started_ts: float = 0.0
    attrs: dict[str, Any] = field(default_factory=dict)
    children: list["Span"] = field(default_factory=list)


class TraceAssembler:
    """Rekonstruiert Span-Baeume aus dem JSONL-Telemetrie-Log."""

    _SKIP_KEYS = {"event", "ts", "rel_s", "trace_id", "span_id",
                 "parent_span_id", "kind", "name"}

    def __init__(self, log_path: str | None = None) -> None:
        self.log_path = Path(
            log_path or Path.home() / ".sin" / "agent-events.jsonl"
        )

    def _events(self) -> list[dict[str, Any]]:
        if not self.log_path.exists():
            return []
        out: list[dict[str, Any]] = []
        for line in self.log_path.read_text(encoding="utf-8").splitlines():
            try:
                rec = json.loads(line)
                if rec.get("event") in ("span_start", "span_end"):
                    out.append(rec)
            except json.JSONDecodeError:
                continue
        return out

    def assemble(self, trace_id: str | None = None) -> list[Span]:
        events = self._events()
        if not events:
            return []
        if trace_id is None:
            trace_id = events[-1].get("trace_id", "")

        spans: dict[str, Span] = {}
        for e in events:
            if e.get("trace_id") != trace_id:
                continue
            sid = e["span_id"]
            if e["event"] == "span_start":
                spans[sid] = Span(
                    span_id=sid,
                    parent_span_id=e.get("parent_span_id"),
                    kind=e.get("kind", "?"),
                    name=e.get("name", "?"),
                    started_ts=e.get("ts", 0.0),
                    attrs={k: v for k, v in e.items()
                           if k not in self._SKIP_KEYS},
                )
            elif e["event"] == "span_end" and sid in spans:
                spans[sid].status = e.get("status", "ok")
                spans[sid].duration_s = e.get("duration_s")

        roots: list[Span] = []
        for span in spans.values():
            parent = spans.get(span.parent_span_id or "")
            if parent is not None:
                parent.children.append(span)
            else:
                roots.append(span)
        for span in spans.values():
            span.children.sort(key=lambda s: s.started_ts)
        return roots

    @staticmethod
    def render_tree(roots: list[Span], *, color: bool = False) -> str:
        GREEN, RED, CYAN, DIM, RESET = (
            "\x1b[32m", "\x1b[31m", "\x1b[36m", "\x1b[2m", "\x1b[0m")

        def paint(code: str, text: str) -> str:
            return f"{code}{text}{RESET}" if color else text

        lines: list[str] = []

        def walk(span: Span, prefix: str, connector: str) -> None:
            dur = (f"{span.duration_s:.1f}s" if span.duration_s is not None
                   else "running")
            status_code = {"ok": GREEN, "running": CYAN}.get(span.status, RED)
            lines.append(
                prefix + connector
                + paint(status_code, f"[{span.status}]")
                + f" {span.kind}:{span.name} "
                + paint(DIM, f"({dur})")
            )
            ext = "    " if connector == "└─ " else "│   "
            for i, child in enumerate(span.children):
                walk(child, prefix + ext, "└─ " if i == len(span.children) - 1
                     else "├─ ")

        for i, root in enumerate(roots):
            walk(root, "", "└─ " if i == len(roots) - 1 else "├─ ")
        return "\n".join(lines)

    def to_chrome_trace(self, trace_id: str | None = None) -> str:
        """Chrome-Tracing-JSON (chrome://tracing / Perfetto-kompatibel)."""
        roots = self.assemble(trace_id)
        events: list[dict[str, Any]] = []

        def walk(span: Span, depth: int) -> None:
            events.append({
                "name": f"{span.kind}:{span.name}",
                "cat": span.kind,
                "ph": "X",
                "ts": int(span.started_ts * 1_000_000),
                "dur": int((span.duration_s or 0.0) * 1_000_000),
                "pid": 1,
                "tid": depth + 1,
                "args": {**span.attrs, "status": span.status},
            })
            for child in span.children:
                walk(child, depth + 1)

        for root in roots:
            walk(root, 0)
        return json.dumps({"traceEvents": events}, ensure_ascii=False)
