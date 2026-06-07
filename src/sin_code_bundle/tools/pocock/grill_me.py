# SPDX-License-Identifier: MIT
"""Socratic Alignment Tool (Grill Me) for Pocock Workflow.

This tool implements the Matt Pocock System-Design Paradigm's alignment phase,
forcing agents to ask clarifying questions and resolve design conflicts before
any code is generated. It produces a standardized Product Requirements Document (PRD.md).
"""

from __future__ import annotations

import os
import sys
import json
import argparse
from pathlib import Path
from typing import Optional
from dataclasses import dataclass, field, asdict


@dataclass
class GrillQuestion:
    """A single socratic question in the alignment process."""
    question: str
    category: str
    answer: Optional[str] = None


@dataclass
class GrillSession:
    """Complete grill session with questions, answers, and generated PRD."""
    target_goal: str
    questions: list[GrillQuestion] = field(default_factory=list)
    context: dict = field(default_factory=dict)
    prd_path: Optional[str] = None


DEFAULT_QUESTIONS = [
    GrillQuestion(
        question="Was ist das konkrete Problem, das gelöst werden soll? (Nicht die Lösung, das Problem)",
        category="problem_definition"
    ),
    GrillQuestion(
        question="Wer sind die primären Nutzer/Stakeholder dieses Features?",
        category="stakeholders"
    ),
    GrillQuestion(
        question="Was sind die harten Constraints (Budget, Zeit, Technik, Compliance)?",
        category="constraints"
    ),
    GrillQuestion(
        question="Welche Edge-Cases und Fehlerzustände müssen explizit behandelt werden?",
        category="edge_cases"
    ),
    GrillQuestion(
        question="Wie sieht der Migrationspfad aus, falls dieses Feature später ersetzt werden muss?",
        category="migration"
    ),
    GrillQuestion(
        question="Was sind die Systemgrenzen? Was gehört NICHT in den Scope?",
        category="boundaries"
    ),
    GrillQuestion(
        question="Wie wird Erfolg gemessen? (Konkrete Metriken, nicht 'es funktioniert')",
        category="success_metrics"
    ),
    GrillQuestion(
        question="Welche bestehenden Systeme/Module müssen integriert oder angepasst werden?",
        category="integration"
    ),
    GrillQuestion(
        question="Was ist der Rollback-Plan, wenn das Feature in Produktion Probleme macht?",
        category="rollback"
    ),
    GrillQuestion(
        question="Gibt es Abhängigkeiten zu externen APIs/Diensten? Wie ist deren SLA?",
        category="dependencies"
    ),
]


class GrillMe:
    """Socratic alignment engine that generates PRDs through structured questioning."""

    def __init__(self, target_goal: str, custom_questions: Optional[list[GrillQuestion]] = None):
        self.target_goal = target_goal
        self.session = GrillSession(target_goal=target_goal)
        self.session.questions = custom_questions or DEFAULT_QUESTIONS.copy()

    def run_interactive(self) -> GrillSession:
        """Run the interactive grill session, prompting for each question."""
        print("\n" + "=" * 70)
        print("🔥 SOKRATISCHER ABSTIMMUNGS-PROZESS (GRILL ME)")
        print("=" * 70)
        print(f"\nZiel: {self.target_goal}\n")
        print("Ich werde dich nun grillen, um Design-Konflikte im Keim zu ersticken.")
        print("Antworte präzise. Leere Antworten sind nicht erlaubt.\n")

        for i, q in enumerate(self.session.questions, 1):
            print(f"\n{'─' * 70}")
            print(f"❓ FRAGE {i}/{len(self.session.questions)} [{q.category.upper()}]")
            print(f"   {q.question}")
            
            while True:
                try:
                    ans = input("   👉 Deine Antwort: ").strip()
                    if ans:
                        q.answer = ans
                        break
                    print("   ❌ Ein leerer Kontext bricht das Alignment ab. Bitte antworte präzise.")
                except (EOFError, KeyboardInterrupt):
                    print("\n\n⚠️  Abgebrochen. Keine PRD generiert.")
                    sys.exit(1)

        self.generate_prd()
        return self.session

    def run_non_interactive(self, answers: dict[str, str]) -> GrillSession:
        """Run with pre-provided answers (for automation/CI)."""
        for q in self.session.questions:
            if q.category in answers:
                q.answer = answers[q.category]
            else:
                raise ValueError(f"Missing answer for category: {q.category}")
        self.generate_prd()
        return self.session

    def generate_prd(self, output_path: Optional[str] = None) -> str:
        """Generate the Product Requirements Document from the grill session."""
        prd_content = self._build_prd_content()
        
        if output_path is None:
            output_path = os.path.join(os.getcwd(), "PRD.md")
        
        self.session.prd_path = output_path
        
        with open(output_path, "w", encoding="utf-8") as f:
            f.write(prd_content)
        
        print(f"\n{'=' * 70}")
        print(f"🎉 PRD ERFOLGREICH GENERIERT: {output_path}")
        print(f"{'=' * 70}\n")
        
        return output_path

    def _build_prd_content(self) -> str:
        """Build the markdown content for the PRD."""
        lines = [
            "# Product Requirements Document (PRD)",
            "",
            "## Zielsetzung",
            self.target_goal,
            "",
            "## Sokratische Klärung (Grill-Protokoll)",
            "",
        ]

        for q in self.session.questions:
            lines.append(f"### {q.question}")
            lines.append(f"**Kategorie:** {q.category}")
            lines.append(f"**Antwort:** {q.answer or '*nicht beantwortet*'}")
            lines.append("")

        lines.extend([
            "## Technische Spezifikation",
            "",
            "### Vertikale Architekturschnitte (Tracer Bullets)",
            "- [ ] Slice 1: Datenbankschema & API-Schnittstelle",
            "- [ ] Slice 2: Integrations-Route & Validierungs-Logik",
            "- [ ] Slice 3: Frontend-Hook / CLI-Integration",
            "",
            "## Abnahmekriterien (Definition of Done)",
            "1. Alle Unittests laufen im TDD-Modus grün (Red-Green-Refactor).",
            "2. Typenprüfung über statische Analyse ist fehlerfrei.",
            "3. Code Review durch mindestens einen weiteren Agenten erfolgt.",
            "4. PRD-Fragen sind alle beantwortet und dokumentiert.",
            "",
            "## Risiken & Offene Fragen",
            "| Risiko | Wahrscheinlichkeit | Impact | Mitigation |",
            "|--------|-------------------|--------|------------|",
            "| TBD | TBD | TBD | TBD |",
            "",
            "---",
            f"*Generiert durch OpenSIN Grill-Me Tool am {self._get_timestamp()}*",
        ])

        return "\n".join(lines)

    def _get_timestamp(self) -> str:
        from datetime import datetime
        return datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    def to_json(self) -> str:
        """Serialize session to JSON."""
        return json.dumps({
            "target_goal": self.session.target_goal,
            "questions": [asdict(q) for q in self.session.questions],
            "prd_path": self.session.prd_path,
        }, indent=2, ensure_ascii=False)


def run_grill_me(goal: str, output: Optional[str] = None, non_interactive: bool = False, answers: Optional[dict] = None) -> str:
    """Convenience function to run grill-me and return PRD path."""
    grill = GrillMe(goal)
    if non_interactive and answers:
        grill.run_non_interactive(answers)
    else:
        grill.run_interactive()
    return grill.session.prd_path or ""


def main():
    """CLI entry point for grill-me."""
    parser = argparse.ArgumentParser(
        description="Socratic Alignment Tool - Grill Me (Pocock Workflow)",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  %(prog)s "Neue Authentifizierungs-API implementieren"
  %(prog)s "Payment-Integration" --output docs/PRD.md
  %(prog)s "Feature X" --non-interactive --answers '{"problem_definition": "...", "stakeholders": "..."}'
        """
    )
    parser.add_argument("goal", help="Das Entwicklungsziel / Feature-Beschreibung")
    parser.add_argument("-o", "--output", help="Pfad für die generierte PRD.md")
    parser.add_argument("--non-interactive", action="store_true", help="Nicht-interaktiver Modus (für CI/CD)")
    parser.add_argument("--answers", help="JSON-String mit Antworten für non-interactive Modus")
    parser.add_argument("--json", action="store_true", help="Session als JSON auf stdout ausgeben")

    args = parser.parse_args()

    grill = GrillMe(args.goal)

    if args.non_interactive:
        if not args.answers:
            print("❌ --non-interactive erfordert --answers JSON", file=sys.stderr)
            sys.exit(1)
        import json
        answers_dict = json.loads(args.answers)
        grill.run_non_interactive(answers_dict)
    else:
        grill.run_interactive()

    if args.output:
        grill.generate_prd(args.output)

    if args.json:
        print(grill.to_json())


if __name__ == "__main__":
    main()
