# PyPI Trusted Publisher — Ausführliche Erklärung

## Was ist PyPI?

PyPI = Python Package Index. Wenn du `pip install sin-code-bundle` machst,
lädt pip die Dateien von https://pypi.org/. Jedes Python-Package der Welt
ist dort.

## Was ist ein "Trusted Publisher"?

Traditionell muss man auf PyPI:
1. Account erstellen
2. API-Token generieren (langer zufälliger String)
3. Den Token irgendwo speichern (als GitHub Secret, in .env, etc.)
4. Bei jedem Release den Token eintippen, damit das Package hochgeladen werden kann

Das ist unsicher (Token kann leaken) und unbequem (muss man pflegen).

**Trusted Publisher** ist PyPIs neue Methode (seit 2023):
- Man registriert EINMAL: "GitHub Action `OpenSIN-Code/SIN-Code-Bundle/.github/workflows/release.yml` darf mich publizieren"
- PyPI vertraut dann GitHub Actions, das mit OIDC-Tokens kurzlebige Credentials bekommt
- Kein API-Token mehr, kein Secret-Rotation, keine Leaks

## Wie funktioniert es technisch?

1. **GitHub Actions OIDC:** `release.yml` hat `permissions: id-token: write`.
   Das erlaubt dem Workflow, OIDC-Tokens von GitHub zu minten (kurzlebig, 15 min).
2. **PyPI vertraut dem Workflow:** Bei PyPI ist `release.yml` als "Trusted Publisher" registriert.
3. **Auto-Publish:** Wenn du `git tag v0.8.0 && git push origin v0.8.0` machst, läuft release.yml,
   bekommt einen OIDC-Token, geht damit zu PyPI und published. Kein API-Token involviert.

## Warum schlägt release.yml fehl?

PyPI weiß noch nichts von unserem Workflow. Wir haben `id-token: write` in der YAML,
aber PyPI würde sagen: "Ich kenne OpenSIN-Code/SIN-Code-Bundle:release.yml nicht, kein Token."

## Wie fixen wir das? (1× manuelle Aktion)

**1× manuell**, danach auto-forever:

1. **PyPI API-Token erstellen** auf https://pypi.org/manage/account/token/
   (nur einmalig, zur Authentifizierung beim Setup)
2. **Tool ausführen:** `python -m sin_code_bundle.tools.pypi_setup --api-token pypi-...`
3. **Tool POSTed** zu `https://pypi.org/_/v1/publisher` mit:
   ```json
   {
     "project": "sin-code-bundle",
     "repository_name": "SIN-Code-Bundle",
     "repository_owner": "OpenSIN-Code",
     "workflow_filename": "release.yml",
     "environment_name": "pypi"
   }
   ```
4. **PyPI schickt Email** an den Maintainer mit Magic Link
5. **Maintainer klickt Link** → Publisher ist registriert
6. **Ab jetzt:** Jeder `git push origin v*` auto-published in 30s

## Was wir gerade haben

- ✅ `release.yml` ist korrekt konfiguriert (id-token: write, environment: pypi)
- ✅ Wir haben den Publisher noch NICHT registriert (manuell nötig)
- ✅ Wir haben das Setup-Tool fertig (v0.8.1) das die Registrierung automatisiert
- ❌ Release-Workflow schlägt noch fehl, bis Maintainer das Tool einmal ausführt

## Warum es nicht von CI aus geht

- PyPI Trusted Publisher Setup braucht:
  1. PyPI-Login (Mensch + Password + 2FA-Code)
  2. Email-Bestätigung (Magic Link)
  3. Beide können nicht von CI automatisiert werden
- Die Maintainer-Tools (PyPI API-Token) sind absichtlich out-of-band

## Was passiert NACH der 1× Aktion

```
git tag v0.9.0
git push origin v0.9.0
   ↓
GitHub: detect tag push → trigger release.yml
   ↓
release.yml: build sdist+wheel → PyPI via OIDC token (auto, no human)
   ↓
PyPI: validates OIDC token against trusted publisher registry
   ↓
PyPI: accepts → package live auf pypi.org in 30s
```

## TL;DR

Trusted Publisher = PyPI vertraut GitHub Actions. Einmal registrieren = nie wieder
API-Tokens pflegen. Wir haben das Tool, es fehlt nur der 1× Klick.
