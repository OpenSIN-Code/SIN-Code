# Skill-Audit-Matrix — OpenSIN-Code Ökosystem

**Datum:** 2026-06-06
**Zweck:** Identifikation von Konsolidierungs-Kandidaten unter den 17 Baseline-Skills
**Methode:** 1-Satz-Zweck + Überschneidungs-Check + ROI-Bewertung

---

## Inventur: 17 Baseline Skills

| # | Skill | Zweck (1 Satz) | Read/Write | Eigener Code? | Status |
|---|---|---|---|---|---|
| 1 | `git-immortal-commit` | Conventional Commits + annotated tag + push to main | Write (git) | Nein (Git-Wrapper) | ✅ Solide |
| 2 | `sin-codocs` | CoDocs Standard definieren + validieren | Read-only | Ja (Python lib + scripts) | ⚠️ Duplikat mit #3 |
| 3 | `sin-codocs-sprint` | CoDocs Bulk-Sprint (generate, repair) | Write | Ja (Python lib + scripts) | ⚠️ Duplikat mit #2 |
| 4 | `ceo-audit` | 47 Quality-Gates Audit (security, performance, ...) | Read-only | Nein (orchestriert andere) | ✅ Solide |
| 5 | `gitnexus-impact-analysis` | Blast-Radius Analyse vor Code-Changes | Read-only | Nein (GitNexus-Wrapper) | ✅ Solide |
| 6 | `sin-context-bridge` | Unified Context über SCKG + sin-brain + GitNexus + local | Read-only | Ja (Python) | ⚠️ Teilweise duplikat mit #5 |
| 7 | `sin-honcho` | Behavioral Memory Layer (User-Präferenzen) | Read+Write | Ja (Python) | ✅ Solide |
| 8 | `sin-infisical` | Secret Management (API-Keys, Tokens) | Read+Write | Ja (Python) | ✅ Solide |
| 9 | `sin-websearch` | Multi-Key SerpAPI Pool | Read+Write (cache) | Ja (Python) | ✅ Solide |
| 10 | `sin-scheduler` | Cron + Interval Job Scheduling | Read+Write (jobs) | Ja (Python) | ✅ Solide |
| 11 | `sin-marketplace` | Skill Discovery + Install | Read+Write | Ja (Python) | ⚠️ Gehört in sin-code-bundle? |
| 12 | `sin-slash` | Custom Slash-Commands | Read+Write | Ja (Python) | ⚠️ opencode hat built-in |
| 13 | `sin-goal-mode` | Goal-Tracking + Subtasks | Read+Write | Ja (Python) | ⚠️ Überschneidung mit sin-honcho? |
| 14 | `sin-frontend-design` | Design Tokens + Components | Read+Write | Ja (Python) | ✅ Solide |
| 15 | `sin-doc-coauthoring` | Doc-Strukturierung (READMEs, ADRs) | Read+Write | Ja (Python) | ⚠️ Überschneidung mit sin-codocs |
| 16 | `sin-mcp-server-builder` | MCP-Server Scaffolding | Write (Filesystem) | Ja (Python) | ⚠️ Tool, kein Skill |
| 17 | `context7` | Library Docs Lookup | Read-only | Nein (npm-Wrapper) | ✅ Solide |
| 18 | `sin-grill-me` | Adversarial Design-Review Interview | Read+Write (state) | Ja (Python) | ✅ Solide |

---

## Konsolidierungs-Kandidaten (Priorisiert nach ROI)

### ✅ Konsolidierung 1 (DONE 2026-06-06): sin-codocs + sin-codocs-sprint → 1 Skill
- **Warum**: 2 Skills, 1 Domain. Validator + Mutator = 1 CLI mit Subcommands
- **Überschneidung**: Beide haben `check`/`scan`, Library-Code, Templates
- **Aufwand**: 1-2h (Repos mergen, Tests deduplizieren, SKILL.md neu)
- **Risiko**: Mittel (sin-code-bundle exposed `sin codocs` CLI)
- **Status**: ✅ **ABGESCHLOSSEN** — v1.0.0 in  live. Alte Repos deprecated. CLI: `sin-codocs check|sprint|generate|repair`.

### 🥈 Konsolidierung 2: sin-slash → opencode built-in
- **Warum**: opencode hat `command` in config — Custom-Slash-Commands gehen ohne Skill
- **Beweis**: opencode.json hat `command` Top-Level key, identische Funktionalität
- **Aufwand**: 30 min (Skill deprecaten, Commands in opencode.json migrieren)
- **Risiko**: Niedrig (User-facing, leicht testbar)
- **Empfehlung**: ⚠️ **PRÜFEN** — wenn opencode built-in reicht, skill deprecaten

### 🥉 Konsolidierung 3: sin-mcp-server-builder → sin-code-bundle CLI
- **Warum**: 8 Tools für MCP-Scaffolding. Gehört als `sin mcp-server` Subcommand ins Bundle
- **Überschneidung**: sin-code-bundle ist die Meta-CLI, sollte alle Meta-Operations bündeln
- **Aufwand**: 1-2h (Logic ins Bundle portieren, Skill deprecaten)
- **Risiko**: Niedrig
- **Empfehlung**: ⚠️ **PRÜFEN** — guter Kandidat für Bundle

### 4️⃣ Konsolidierung 4: sin-marketplace → sin-code-bundle CLI
- **Warum**: `sin marketplace` als Subcommand im Bundle ist naheliegender als separater Skill
- **Überschneidung**: Bundle IST die Meta-CLI, marketplace IST eine Meta-Operation
- **Aufwand**: 1h
- **Risiko**: Niedrig
- **Empfehlung**: ⚠️ **PRÜFEN** — auch ein Bundle-Kandidat

### 5️⃣ Konsolidierung 5: sin-context-bridge ↔ gitnexus-impact-analysis
- **Warum**: Beide fragen Knowledge-Graph ab, gitnexus = spezifisch, bridge = generisch
- **Überschneidung**: bridge kann gitnexus nutzen, redundante Funktionalität
- **Aufwand**: 1h
- **Risiko**: Niedrig
- **Empfehlung**: ⚠️ **PRÜFEN** — bridge ist Wrapper, könnte gitnexus + SCKG direkt subsumieren

### 6️⃣ Prüfung: sin-goal-mode ↔ sin-honcho
- **Warum**: Beide speichern State über Sessions
- **Unterschied**: honcho = Memory, goal-mode = explizite Goals
- **Empfehlung**: ✅ **BEHALTEN** — verschiedene Konzepte (Memory vs. Goal-Tracking)

### 7️⃣ Prüfung: sin-doc-coauthoring ↔ sin-codocs
- **Warum**: Beide machen Docs
- **Unterschied**: doc-coauthoring = Doc-INHALT, codocs = Doc-STRUKTUR (Companion-Files)
- **Empfehlung**: ✅ **BEHALTEN** — orthogonal, nicht redundant

---

## Nach Konsolidierung: 17 → 12 Skills

| Vorher | Nachher | Typ |
|---|---|---|
| sin-codocs + sin-codocs-sprint | **sin-codocs** (mit sprint/repair subcommands) | Merge |
| sin-slash | (in opencode.json `command`) | Deprecate |
| sin-mcp-server-builder | (in sin-code-bundle) | Bundle-ify |
| sin-marketplace | (in sin-code-bundle) | Bundle-ify |
| sin-context-bridge | (bleibt, ggf. gitnexus subsumieren) | Klären |
| Rest 12 Skills | unverändert | ✅ |

**Reduktion: 17 → 12-13 Skills** (28% weniger Komplexität)

---

## Governance-Policy (Phase 4)

> **Ab sofort:** Jeder neue Skill braucht schriftliche Begründung:
> 1. **Zweck** (1 Satz, klar abgegrenzt von existierenden Skills)
> 2. **Beweis** dass kein existierender Skill die Funktion abdeckt
> 3. **Owner** mit Zeitbudget für Maintenance
>
> Sonst: kein neuer Skill. Ausnahmen nur mit CEO-Approval.

---

## Empfohlene Reihenfolge

1. **Heute Abend**: Konsolidierung 1 (CoDocs) — direkt umsetzbar, größter Schmerzpunkt
2. **Diese Woche**: Konsolidierungen 2-3 (slash → built-in, mcp-builder → bundle)
3. **Nächste Woche**: Konsolidierungen 4-5 (marketplace, context-bridge)
4. **Danach**: Governance-Policy durchsetzen
