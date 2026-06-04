"""Architektur- und Komplexitaets-Analyse via radon."""
from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import Iterable


@dataclass
class FileReport:
    path: str
    lines: int
    cyclomatic_avg: float
    maintainability: float
    hotspots: list[dict] = field(default_factory=list)


class ComplexityAnalyzer:
    """Misst technische Schulden quantitativ."""

    def analyze(self, root: str, exclude: Iterable[str] = ()) -> list[FileReport]:
        from radon.complexity import cc_visit
        from radon.metrics import mi_visit

        reports: list[FileReport] = []
        root_path = Path(root)
        exclude_set = set(exclude)
        for py in root_path.rglob("*.py"):
            rel = py.relative_to(root_path).as_posix()
            if any(part in exclude_set for part in Path(rel).parts):
                continue
            try:
                src = py.read_text(encoding="utf-8", errors="replace")
            except Exception:
                continue
            try:
                blocks = cc_visit(src)
                avg_cc = sum(b.complexity for b in blocks) / max(len(blocks), 1)
                try:
                    mi = mi_visit(src, True)
                except Exception:
                    mi = 0.0
                hotspots = [
                    {"name": b.name, "complexity": b.complexity, "line": b.lineno}
                    for b in blocks
                    if b.complexity > 10
                ]
                reports.append(
                    FileReport(
                        path=rel,
                        lines=len(src.splitlines()),
                        cyclomatic_avg=round(avg_cc, 2),
                        maintainability=round(mi, 2),
                        hotspots=hotspots,
                    )
                )
            except Exception:
                reports.append(
                    FileReport(path=rel, lines=0, cyclomatic_avg=0.0, maintainability=0.0)
                )
        return reports

    def debt_score(self, reports: list[FileReport]) -> dict:
        """Gesamt-Schulden-Score (0-100)."""
        if not reports:
            return {"score": 0.0, "level": "none", "total_hotspots": 0, "avg_cc": 0.0}
        total_cc = sum(r.cyclomatic_avg for r in reports) / len(reports)
        hotspots = sum(len(r.hotspots) for r in reports)
        lines = sum(r.lines for r in reports)
        score = min(100.0, total_cc * 5 + hotspots * 2 + (lines / 10000) * 10)
        if score < 20:
            level = "healthy"
        elif score < 50:
            level = "manageable"
        elif score < 80:
            level = "warning"
        else:
            level = "critical"
        return {
            "score": round(score, 2),
            "level": level,
            "total_hotspots": hotspots,
            "avg_cc": round(total_cc, 2),
        }
