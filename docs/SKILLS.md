# SIN-Code Skills Integration

## Übersicht
SIN-Code unterstützt nun **Agent Skills** gemäß dem [agent-skills](https://github.com/addyosmani/agent-skills) Standard.  
Skills sind strukturierte Workflows, die AI-Agenten diszipliniert ausführen – inklusive Qualitätsgates, Verifikationsschritten und Anti-Rationalisierungen.

## Installation eines Skills
```bash
sin skill install ~/mein-skill-verzeichnis   # lokaler Pfad
sin skill install https://github.com/benutzer/awesome-skill   # Git-Repo (bald)
```

## Verfügbare Skills auflisten
```bash
sin skill list
```

## Skill ausführen
```bash
sin skill run spec --verbose
```

## Eigene Skills schreiben
Erstellen Sie ein Verzeichnis mit einer `SKILL.md` Datei. Aufbau:
```markdown
# skill-name
## Overview
Kurzbeschreibung

## Steps
1. Erster Schritt
2. Zweiter Schritt

## Verification
- [ ] Prüfpunkt

## Anti-Rationalization
| Ausrede | Entkräftung |
| "..." | "..." |
```

Die Steps werden nacheinander durch den SIN-Code-Agenten ausgeführt. Jeder Step kann auf MCP-Tools zugreifen.

## Integration mit Multi-Agent-System
- **Governor**: Budget- und Sicherheitsgrenzen.
- **Critic**: Prüft jeden Step vor Ausführung.
- **Adversary**: Verifiziert die Ausgabe jedes Steps.

## Best Practices
- Halten Sie Skills fokussiert (max. 10 Steps).
- Nutzen Sie die Anti-Rationalization-Tabelle, um typische Agenten-Ausreden zu blockieren.
- Lagern Sie komplexe Logik in separate MCP-Tools aus.

## Fehlersuche
- `sin skill run --verbose` zeigt jeden Step an.
- Logs in `~/.sin/logs/skill-<name>.log`.

## Chains – Verkettete Skill-Ausführung
Skills können in Chains verkettet werden für komplexe Workflows:
```bash
sin skill chain execute ./chains/sin-full-lifecycle.chain.json
```

Chains unterstützen:
- **Retry-Logik**: Automatische Wiederholung bei Fehlern
- **Fallback-Skills**: Alternative Skills bei Fehler
- **Shared State**: Daten zwischen Skills austauschen
- **Loop-Detection**: Endlosschleifen verhindern
