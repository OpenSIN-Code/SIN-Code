# v0.5.0-ceo-audit-v3 Stale Tags — Ausführliche Erklärung

## Was ist passiert?

Am 2026-06-04 haben wir die `ceo-audit.yml` Workflow-Datei (mit 47-Gate SOTA Audit)
per Bulk-Deploy auf 30 OpenSIN-Code Repos ausgerollt. Damit jeder Commit
und jeder PR automatisch auditiert wird.

Aus diesem Bulk-Deploy haben wir auf jedem der 30 Repos einen Git-Tag erstellt:
`v0.5.0-ceo-audit-v3`. Das ist:
- **KEIN** Versions-Tag (wie `v0.1.0` oder `v1.0.0`)
- **EIN Deployment-Marker**: "Dieses Repo hat ceo-audit v3 (Workflow-Version 3) deployt"

## Warum dieser Tag da ist

Bei `git clone --tags` würde man sonst nur die version-spezifischen Tags
sehen (`v0.1.0`, `v0.1.1`, etc.). Aber wenn jemand explizit "den ceo-audit
Workflow v3 checkout" will, kann er `git checkout v0.5.0-ceo-audit-v3` machen.

Es ist also ein **Marker** für den Bulk-Deploy, nicht eine Version.

## Welche Repos haben diesen Tag?

19 von 30 OpenSIN-Code Repos. Die anderen 11 haben entweder:
- Ihren eigenen Versionierungs-Schema (z.B. SIN-Code-Discover-Tool hat `v1.0.1-codocs`)
- Wurden später erstellt (nach dem Bulk-Deploy) und haben den Tag nicht bekommen
- Haben gar keine Tags

## Das "Problem" — ist es wirklich eins?

**Nein, es ist nur Hygiene.** Der Tag schadet nicht:
- Er zeigt auf einen alten Commit (den Bulk-Deploy-Commit)
- Er ist nicht die "aktuelle Version" (das ist z.B. `v0.8.1` für das Bundle)
- Er wird nicht von PyPI oder CI referenziert

**ABER:** Es ist verwirrend wenn jemand `git tag` macht und 3-4 Tags sieht
(`v0.5.0-ceo-audit-v3`, `v0.1.0`, `v0.1.1`, `v0.2.0`...). Welches ist die
"richtige" Version?

## Was wir gemacht haben

Wir haben das in der `ceo-audit/SKILL.md` dokumentiert:
```markdown
## Tag naming convention

`v0.5.0-ceo-audit-v3` ist ein bulk-deploy marker, NOT a project version.
Project versions use a different scheme (e.g. `v0.1.0`, `v0.7.0`).

To find the project's actual version:
  git describe --tags --exclude 'v*-ceo-audit-v*'
```

## Warum wir den Tag NICHT gelöscht haben

- Andere Tools/Scripts könnten den Tag-Namen hardcoden
- `git log v0.5.0-ceo-audit-v3` wäre plötzlich broken
- Konsumenten (z.B. CI, Mirror) könnten ihn gecached haben
- **Doku ist sicherer als Delete**

## TL;DR

19 Repos haben einen historischen Bulk-Deploy-Tag. Er ist harmlos, gut
dokumentiert, und nicht der Anlass für eine Aktion. Falls jemand den
Versions-Tag eines Repos finden will: `git describe --tags --exclude 'v*-ceo-audit-v*'`.
