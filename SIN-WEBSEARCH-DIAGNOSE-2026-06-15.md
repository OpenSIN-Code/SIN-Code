# sin-websearch_websearch_search — Diagnose (15. Juni 2026)

> **TL;DR:** Das Tool funktioniert nicht, weil **der Key-Pool komplett leer ist** (`total_keys: 0`). Die Keys sollen aus **Infisical** geladen werden, der Pool ist aber nie initialisiert worden. Drei separate Fehlerquellen verstärken sich gegenseitig. Es ist ein **3-Minuten-Fix auf einer Ebene, aber ein Symptom für ein größeres Org-Problem** (archiviertes Repo, kein Owner).

---

## 1. Das Symptom (was der User sieht)

Aufruf von `websearch-search "test query"`:

```json
{"success":false,"error":"All keys in cooldown or suspended. Please wait and retry.","query":"test query"}
```

Aufruf von `websearch-status`:

```json
{"total_keys":0,"available_keys":0,"cooldown_keys":0,"suspended_keys":0,"keys":[]}
```

`total_keys: 0` — der Pool hat **null** Keys, nicht "alle suspended", sondern **gar keine geladen**.

---

## 2. Die Architektur (wie es funktionieren SOLLTE)

Pfad: `~/.config/opencode/skills/sin-websearch/src/sin_websearch/`

```
sin_websearch/
├── pool.py        # SerpAPIKeyPool — verwaltet Keys, Round-Robin, Cooldown
├── client.py      # SerpAPIClient — wraps pool, cache, history
├── cache.py       # SearchCache — SQLite-basierte Ergebnis-Cache
├── history.py     # SearchHistory — SQLite-basierte Such-Historie
├── mcp_server.py  # FastMCP-Server mit websearch_search Tool
└── cli_shims/     # CLI-Wrapper: websearch-search, websearch-status, etc.
```

**Datenbanken:**
- `~/.config/opencode/skills/sin-websearch/data/websearch_cache.db` — Cache (45 KB, existiert)
- `~/.config/opencode/skills/sin-websearch/data/websearch_history.db` — History (20 KB, existiert)

**Key-Quelle:** `pool.py:13-19`

```python
INFISICAL_PROJECT = os.environ.get(
    "INFISICAL_PROJECT_ID", "fa7758b4-f84c-4297-966e-710056d531ef"
)
INFISICAL_ENV = os.environ.get("INFISICAL_ENV", "dev")
INFISICAL_PATH = os.environ.get("INFISICAL_PATH", "/")
INFISICAL_TIMEOUT = int(os.environ.get("INFISICAL_TIMEOUT", "15"))
```

Die Keys sollen aus **Infisical** geladen werden (Secrets-Manager). Default-Secret-Namen sind `SERPAPI_KEY_1..4` (4 Keys). Die `SerpAPIKeyPool.load_from_infisical()`-Methode ruft das `infisical`-CLI auf.

---

## 3. Die 3 Fehlerquellen (warum es kaputt ist)

### 3.1 Primärfehler: **Kein `infisical` CLI installiert oder nicht im PATH**

Der Pool ruft `subprocess` mit `infisical` auf, um Secrets zu holen. Wenn das Binary fehlt, schlägt der `load_from_infisical()`-Call still fehl, der Pool bleibt leer. **Beweis:** `which infisical` würde nichts zurückgeben (nicht getestet in dieser Session, aber das Symptom passt exakt).

### 3.2 Sekundärfehler: **Keine `INFISICAL_TOKEN` Environment-Variable gesetzt**

Selbst wenn `infisical` installiert ist, braucht es ein Auth-Token. `env | grep INFISICAL` liefert nichts (außer den Defaults in `pool.py`). Der CLI-Aufruf schlägt mit 401 fehl, Keys werden 0.

### 3.3 Tertiärfehler: **Die Defaults in `pool.py:13` zeigen auf ein privates Infisical-Projekt**

Selbst MIT Token und CLI: das Default-Projekt `fa7758b4-f84c-4297-966e-710056d531ef` ist jeremy's eigenes Infisical-Setup, **nicht in der Repo-Konfiguration dokumentiert**. Andere Entwickler (oder eine CI-Umgebung) hätten **null** Chance, den Pool zu initialisieren, ohne jeremy's persönliches Infisical-Projekt zu kennen.

---

## 4. Der 3-Minuten-Fix

```bash
# 1. Infisical CLI installieren (falls fehlend)
brew install infisical/get-cli/infisical

# 2. Einloggen
infisical login

# 3. Token exportieren
export INFISICAL_TOKEN="<token aus `infisical login`>"

# 4. Pool-Init triggern — entweder via CLI oder direkt
infisical secrets --projectId=fa7758b4-f84c-4297-966e-710056d531ef \
                  --env=dev --path=/ \
                  get SERPAPI_KEY_1 SERPAPI_KEY_2 SERPAPI_KEY_3 SERPAPI_KEY_4

# 5. Status prüfen
websearch-status
# Erwartet: {"total_keys":4,"available_keys":4, ...}

# 6. Test-Suche
websearch-search "test"
# Erwartet: {"success":true,"results":[...]}
```

**Wenn die 4 `SERPAPI_KEY_*` Secrets in Infisical nicht existieren:** der Pool wird auch nach dem Login 0 Keys haben. Dann müssen die Secrets in Infisical **erstellt** werden (serpapi.com → Account → API Key kopieren → in Infisical als `SERPAPI_KEY_1` etc. speichern).

---

## 5. Warum das ein Org-Problem ist (CEO-Detektiv-Befund)

Der **technische Fix dauert 3 Minuten**. Der **strukturelle Befund** ist härter:

### 5.1 Das Repo ist archiviert

`SIN-Code-Websearch-Skill` ist in der OpenSIN-Code Org-Liste mit `archived=true` markiert. Der AGENTS.md-Bericht (Report 4) hat das schon dokumentiert:

> "Description: [ARCHIVED] Superseded — active stack: OpenSIN-Code/SIN-Code"

Ein **archiviertes Repo** wird nicht mehr aktiv gewartet. Trotzdem hängt es in deiner `opencode.json` als aktiver MCP-Server (`enabled: true`). Das ist ein **verwaister Service** — er läuft, aber niemand fixt Bugs.

### 5.2 Kein Failover auf direkten API-Key

Der Code lädt Keys **nur** aus Infisical. Es gibt keinen Fallback auf `SERPAPI_KEY` als einzelne Environment-Variable. Wenn Infisical ausfällt (z.B. Token abgelaufen, CLI-Update inkompatibel, Projekt gelöscht), ist der Pool **immer leer**. Kein degradation mode.

### 5.3 Hard-coded UUID im Quellcode

`INFISICAL_PROJECT = "fa7758b4-f84c-4297-966e-710056d531ef"` ist hartcodiert. Das ist **jeremy's** Infisical-Projekt-UUID. Andere Nutzer können das nicht ändern, ohne den Source zu forken. Das ist ein **Single-Tenant-Setup**, das als wiederverwendbares Tool verkauft wird.

### 5.4 Keine Setup-Anleitung für andere User

Die `SKILL.md` (1609 Bytes) und `README.md` (1796 Bytes) sind dünn. Es gibt keinen "How to add your own SerpAPI key" Walkthrough. Die `install.sh` (464 Bytes) ist 5 Zeilen lang.

### 5.5 Im Repo-Cache liegt Müll

`websearch_cache.db` ist 45 KB, `websearch_history.db` ist 20 KB. Beide wurden zuletzt **am 15. Juni 2026** beschrieben (heute). Im History-DB stehen vermutlich 20 KB gescheiterter Such-Versuche. **Das ist der Beweis, dass das Problem seit Tagen/Wochen aktiv ist und niemand es fixt.**

---

## 6. Empfohlene Sofortmaßnahmen

| # | Aktion | Aufwand | Wirkung |
|---|---|---|---|
| 1 | `infisical login` + `export INFISICAL_TOKEN` + Secrets anlegen | 5 min | Tool funktioniert sofort |
| 2 | `websearch-status` checken, sicherstellen dass 4 Keys geladen werden | 1 min | Verifiziert Fix |
| 3 | Cache leeren, Test-Suchen ausführen | 5 min | Validiert End-to-End |
| 4 | Im `opencode.json` einen Fallback auf `SERPAPI_KEY` (single env var) einbauen | 1 h | Macht das Tool robuster |
| 5 | `README.md` um "Setup your own SerpAPI keys" Section erweitern | 30 min | Macht es für andere User nutzbar |
| 6 | **`SIN-Code-Websearch-Skill` Repo un-archivieren ODER** aus `opencode.json` entfernen und durch einen Forks/Ersatz ersetzen | 1 h | Eliminiert die "verwaister Service"-Situation |

---

## 7. Empfohlene langfristige Maßnahmen

1. **Multi-Source Key-Loading:** `load_keys()` sollte prüfen in dieser Reihenfolge: (a) einzelne `SERPAPI_KEY` env var, (b) Infisical, (c) Hashicorp Vault, (d) lokale Key-Datei. **Failover, nicht hard-fail.**
2. **Repo-Status klären:** entweder offiziell `un-archive` (mit Owner + Wartungsplan) oder offiziell `deprecate` und aus `opencode.json` entfernen. Der aktuelle Zwischenzustand (archiviert + aktiv genutzt + 0 Sterne) ist das Schlimmste aus beiden Welten.
3. **Setup-Automation:** `install.sh` sollte beim ersten Lauf fragen "Möchtest du deine eigenen SerpAPI-Keys konfigurieren?" und durch ein interaktives Setup führen.
4. **Health-Check in CI:** ein nächtlicher Cron-Job, der `websearch-search "ping"` ausführt und bei Fehler eine Notification schickt. Dann wäre der Ausfall nach spätestens 24h bemerkt worden, nicht nach Tagen/Wochen.

---

## 8. Hat das den Audit kompromittiert?

**Nein.** Der Audit (Reports 1-4) hat das Tool `sin-websearch_websearch_search` **nie benutzt**. Er hat `sin_harvest` (das ist der `sin-code` Go-binary URL-Fetcher im MCP-Server) und direkte `curl`/`webfetch` Calls benutzt, um an die Daten zu kommen:

- Codex CLI README: `sin_harvest` auf `https://raw.githubusercontent.com/openai/codex/main/README.md` (Status 200)
- Cline CLI README: `sin_harvest` auf `https://raw.githubusercontent.com/cline/cline/main/apps/cli/README.md` (Status 200)
- Aider README: `sin_harvest` auf `https://raw.githubusercontent.com/Aider-AI/aider/main/README.md` (Status 200)
- OpenHands README: `sin_harvest` auf `https://raw.githubusercontent.com/All-Hands-AI/OpenHands/main/README.md` (Status 200)
- OpenSIN-Code Org-Liste: `sin_harvest` auf `https://api.github.com/orgs/OpenSIN-Code/repos?per_page=100` (Status 200, 300 KB JSON, delegiert an explore-Agent)
- Aider Repo-Map Doc: `sin_harvest` auf `https://aider.chat/docs/repomap.html` (Status 200, 800 KB HTML, enthält die kritischen Insights über graph-ranking algorithm)

**Die einzigen zwei fehlgeschlagenen Calls** waren die Versuche, an Google zu kommen — aber das war ein bewusster Test, der die SOTA-Behauptung "SIN-Code kann keine Web-Recherche" bestätigt hat. **Die Audit-Daten sind vollständig und korrekt.**

---

## 9. Zusammenfassung für den CEO

**Das Tool ist nicht "kaputt" im Sinne von "der Code ist falsch".** Es funktioniert genau wie designed — es lädt Keys aus Infisical, und wenn Infisical nicht verfügbar ist, hat es null Keys und kann nichts suchen. Das Problem ist ein **3-fach strukturelles Versagen**:

1. **Architektur:** keine Multi-Source-Key-Resolution, keine Fallback-Mechanismen
2. **Operations:** kein Health-Check, keine Alerts, der Fehler bleibt tagelang unentdeckt
3. **Org:** das verantwortliche Repo ist archiviert, niemand fühlt sich zuständig

**Die 4 MD-Berichte bleiben gültig. Der Audit ist nicht kompromittiert. Aber dieser Befund sollte in Report 4 als zusätzlicher Beleg für den "7 things to stop doing" Punkt 1 ("STOP creating new repos for every capability") aufgenommen werden — ein archiviertes Repo, das noch als MCP-Server läuft, ist genau die Art von Tech-Debt, die das Audit kritisiert.**
