# Baseline Skills — Purpose Notes

**Datum:** 2026-06-06
**Quelle:** `~/.config/opencode/opencode.json` → `skills.baseline`
**Zweck:** 1-2 Sätze pro Skill, distinct from X, Y, Z.
**Bezug:** Skill Creation Charter (Test 1: One-sentence purpose)

---

## Die 17 Baseline-Skills

### 1. `git-immortal-commit`
> Macht Conventional Commits + annotated tag + push to main, distinct from
> `gitnexus-cli` (nur Read/Query) und built-in git tools (kein Charter-Workflow),
> weil ein "unsterblicher" Commit nie via reset/checkout verloren gehen darf.

**Status:** ✅ Solide.

### 2. `sin-codocs`
> Definiert und validiert den CoDocs-Standard (.doc.md Companions + Inline-Comments),
> distinct from `sin-doc-coauthoring` (schreibt Doc-Inhalte) und
> `sin-codocs-sprint` (jetzt konsolidiert als Subcommand), weil CoDocs
> ein **Struktur-Standard** ist, kein Doc-Inhalt.

**Status:** ✅ Konsolidiert (Issue #29, Konsolidierung 1 DONE).

### 3. ~~`sin-codocs-sprint`~~ → konsolidiert in `sin-codocs`
> War: Bulk-Generierung von .doc.md Drafts, distinct from `sin-codocs` (Validator).
> **Konsolidiert 2026-06-06** als `sin-codocs sprint|generate|repair` Subcommands.

**Status:** ✅ In `sin-codocs` aufgegangen.

### 4. `ceo-audit`
> 47 Quality-Gates Audit (Security, Performance, Code-Qualität, Dependencies,
> Tests, Docs, Compliance), distinct from `gitnexus-impact-analysis` (Code-Graph)
> und `sin-codocs` (nur Doc-Struktur), weil ceo-audit **multi-dimensional** ist
> und externe Tools (bandit, mypy, ruff, gosec) orchestriert.

**Status:** ✅ Solide.

### 5. `gitnexus-impact-analysis`
> Blast-Radius-Analyse vor Code-Changes via Code Knowledge Graph,
> distinct from `ceo-audit` (multi-dimensional) und `sin-context-bridge`
> (multi-source Wrapper), weil GitNexus **spezifisch Graph-basiert** ist
> und CALLS/IMPORTS-Traversal liefert.

**Status:** ✅ Solide.

### 6. `sin-context-bridge`
> Unified Context-Lookup über SCKG + sin-brain + GitNexus + local SQLite
> in 1 MCP-Call, distinct from `gitnexus-impact-analysis` (nur GitNexus)
> und `claude_mem_search` (nur Honcho), weil bridge **mehrere Quellen mergt**
> mit graceful degradation.

**Status:** ⚠️ Teilweise Überschneidung mit #5 — siehe Audit-Matrix Konsolidierung 5.

### 7. `sin-honcho`
> Behavioral-Memory-Layer (User-Präferenzen, Session-Kontext über Sessions hinweg),
> distinct from `sin-goal-mode` (explizite Goals) und `sin-brain` (rules),
> weil Honcho **implizite User-Signale** speichert (PREFERENCES, PEER-MODELS).

**Status:** ✅ Solide.

### 8. `sin-infisical`
> Zentrales Secret-Management (API-Keys, Tokens, Credentials) via Infisical CLI,
> distinct from `sin-honcho` (User-Memory) und `.env` Files (kein Rotation),
> weil Secrets **mandatory-rotierbar** sind und nie in Git landen dürfen.

**Status:** ✅ Solide.

### 9. `sin-websearch`
> Multi-Key SerpAPI Pool mit Cache+History (LRU + 2-Tier Cooldown),
> distinct from opencode built-in websearch (kein Pool, single Key) und
> `context7` (Library-Docs, nicht Web), weil wir >100 req/day Kapazität brauchen.

**Status:** ✅ Solide.

### 10. `sin-scheduler`
> Cron + Interval Job-Scheduling mit Timeout + Audit-Log,
> distinct from `sin-orchestrate` (Task-Management, nicht Zeit-basiert) und
> built-in LaunchAgents (kein Agent-Style), weil Scheduler **persistent + replayable** ist.

**Status:** ✅ Solide.

### 11. `sin-marketplace`
> Skill Discovery + Install aus zentralem Katalog (sync mit Infra-SIN-OpenCode-Stack),
> distinct from `sin-slash` (Command-Registry) und `sin-codocs` (Doc-Standard),
> weil Marketplace **das Meta-Verzeichnis** aller Skills ist.

**Status:** ⚠️ Bundle-Kandidat (Audit-Matrix Konsolidierung 4).

### 12. `sin-slash`
> Custom Slash-Commands registrieren + dispatchen (mit History + Built-in/Custom),
> distinct from opencode built-in `command` (kein History, keine Built-ins) und
> `sin-marketplace` (Skill-Registry), weil Slash-Commands **User-spezifische Workflows** sind.

**Status:** ⚠️ Built-in-Kandidat (Audit-Matrix Konsolidierung 2).

### 13. `sin-goal-mode`
> Goal-Tracking mit Subtasks, Checkpoints, Rollback und Reports,
> distinct from `sin-honcho` (User-Memory, nicht Goal-State) und
> `sin-orchestrate` (einmalige Tasks, nicht persistent), weil Goal-Mode
> **lifecycle-aware** ist (start → checkpoint → complete/rollback).

**Status:** ✅ Solide.

### 14. `sin-frontend-design`
> SOTA Frontend Design System (Tokens, Komponenten, WCAG 2.2 AA, optional v0-Generation),
> distinct from `sin-codocs` (Doc-Struktur) und `sin-mcp-server-builder` (Backend-Tooling),
> weil Frontend-Design **visuelle Systeme** braucht (Typography, Color, Motion).

**Status:** ✅ Solide.

### 15. `sin-doc-coauthoring`
> Strukturierte Doc-Co-Authoring-Sessions (READMEs, ADRs, Specs, RFCs, API-Docs) via MCP,
> distinct from `sin-codocs` (Struktur-Standard, kein Inhalt) und
> `sin-codocs-sprint` (Bulk, nicht session-basiert), weil Doc-Co-Authoring
> **iterativ + sokratisch** ist (Fragen, Drafts, Review).

**Status:** ✅ Solide (orthogonal zu `sin-codocs`).

### 16. `sin-mcp-server-builder`
> MCP-Server-Scaffolding (8 Tools: scaffold, template_list, add_tool, test, register, validate, publish, audit),
> distinct from `sin-code-bundle` (generische Meta-CLI) und
> `sin-frontend-design` (Frontend, nicht Server), weil MCP-Builder **spezifisch für MCP** ist.

**Status:** ⚠️ Bundle-Kandidat (Audit-Matrix Konsolidierung 3) — sollte als
`sin mcp-server` Subcommand ins Bundle.

### 17. `context7`
> Library-Docs-Lookup (versionierte, aktuelle Dokumentation via Context7 MCP),
> distinct from `sin-websearch` (generisches Web) und built-in code-intel
> (kein Doc-Layer), weil context7 **versioniert + lib-spezifisch** ist.

**Status:** ✅ Solide.

### 18. `sin-grill-me`
> Adversarial Design-Review-Interview (relentless questioning, hidden assumptions),
> distinct from `ceo-audit` (Code-Audit, nicht Design) und `claude_mem_search`
> (Memory-Lookup, nicht Interview), weil Grill-Mode **sokratisch + stateful** ist.

**Status:** ✅ Solide.

---

## Deprecation-Kandidaten

Basierend auf Audit-Matrix (Stand 2026-06-06):

| Skill | Grund | Empfehlung |
|---|---|---|
| ~~`sin-codocs-sprint`~~ | Duplikat zu `sin-codocs` | ✅ DONE — konsolidiert |
| `sin-slash` | opencode built-in `command` reicht evtl. | ⚠️ PRÜFEN (Kons. 2) |
| `sin-mcp-server-builder` | Gehört als `sin mcp-server` ins Bundle | ⚠️ PRÜFEN (Kons. 3) |
| `sin-marketplace` | Gehört als `sin marketplace` ins Bundle | ⚠️ PRÜFEN (Kons. 4) |
| `sin-context-bridge` | gitnexus-impact könnte bridge subsumieren | ⚠️ PRÜFEN (Kons. 5) |

**Ziel-Kapazität:** target 14, hard cap 16 (siehe Charter).

---

## Wartung

- Bei neuem Skill: Diese Datei **muss** aktualisiert werden (Charter Test 2).
- Bei Deprecation: Skill aus `opencode.json` baseline entfernen, hier als
  "DONE — konsolidiert in X" markieren.
- Quartalsweise Review: alle 17 Einträge prüfen, Redundanzen markieren.
