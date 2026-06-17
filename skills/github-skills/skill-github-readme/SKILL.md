---
name: skill-github-readme
description: "Transform ANY repository into a visually stunning, instantly understandable experience. Hero banner, SOTA badges, benefit-driven bullets, clean code blocks, social proof, star history. Based on research of 5 top GitHub repos (Anthropic, OpenAI, Vercel, tldraw, screenshot-to-code). Embeds sin-code image-graph charts for benchmark data."
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  audience: all-engineering-levels
  mode: autonomous-visual-enhancement
  language: en
  coupled_with: skill-github-governance
  integrates_with: sin-code image-graph
  version: "4.0"
  last_updated: "2026-06-17"
  lifecycle: external
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/visual-repo"
---

> SIN-Code Bundled Skill v4.0 — SOTA README Standard. Based on research of 5 top GitHub repos (Anthropic SDK 3.6k stars, OpenAI Python 31k stars, Vercel Next.js 140k stars, tldraw 48k stars, screenshot-to-code 73k stars). Macht JEDES Repo visuell verstaendlich, AI-discoverable, und professionell.

# skill-github-readme (SOTA README Standard)

**Transformiere jedes Repository in ein visuelles Meisterwerk das auf den ersten Blick verstaendlich, AI-discoverable, und professionell ist.**

---

## TRIGGER

**Keywords:** "readme", "readme verbessern", "repo visualisieren", "visual repo", "github readme", "skill-github-readme", "optisch aufwerten", "profis readme", "sota readme", "anthropic style readme", "openai style readme"

**PFLICHT-AUSLOESER:**

- `skill-github-governance` -> IMMER `skill-github-readme` mitverwenden!
- Neues Repo erstellt
- Bestehendes Repo mit schlechter README
- "Ich verstehe nicht was das macht"
- Professionalisierung needed

---

## SKILL-KOPPLUNG

```
skill-github-governance  ->  "Was muss gemacht werden?" (Issues, Roadmaps)
        ↓ coupled
skill-github-readme      ->  "Wie sieht es aus?" (README, Badges, Hero)
        ↓ can embed
sin-code image-graph     ->  "Daten visualisieren" (sin-code image-graph)
```

---

## DIE SOTA-README-FORMEL

Based on research of the 5 most successful open-source repos on GitHub:

```
Hero Image -> 3-4 Badges -> Tagline -> 3 Quick Links -> Quick Start -> 
Features (bullets) -> Examples/Proof -> Docs Link -> Community -> Contributing -> License -> Star History
```

### Anti-Patterns (VERBOTTEN)

| Pattern | Warum schlecht | Sota-Alternative |
|---------|---------------|-----------------|
| Mermaid-Diagramme im README | Kein Top-Repo nutzt sie; GitHub rendert sie unzuverlaessig | Link zu `/docs/architecture.md` |
| `<details>` collapsible sections | Versteckt Inhalt = schlechte UX | Alles sichtbar, kurze Sektionen |
| Navigation-Anchor-Links | Kein Top-Repo nutzt sie; GitHub hat schon TOC | Weglassen |
| HTML-Tabellen fuer Layout | Sieht aus wie 2015 | Markdown-Code-Bloecke |
| Emoji in Headings | Unprofessionell; kein Top-Repo macht das | Klare Text-Headings |
| 6+ Badges | Cluttered; wirkt verzweifelt | Max 3-4 relevante Badges |
| Feature-Tabelle mit ✅ | Wirtsdlich; schwer lesbar | Bullet-List mit Bold-Termen |
| Deutsch im README | International = English | Immer English |
| `back to top` Links | Veraltet; GitHub hat Scroll | Weglassen |
| Footer-Banner ("Powered by") | Wird als Werbung wahrgenommen | Subtle `<sub>` Text |

---

## DIE 10 GEBOTE DES SOTA README

### 1. Hero Image (PFlicht bei visuellen Repos)

Ein einzelnes PNG-Bild oben, das das Projekt oder sein Ergebnis zeigt.

```html
<p align="center">
  <img src="./assets/hero.png" alt="Project Name" width="800" />
</p>
```

**Regeln:**
- Eine Datei: `assets/hero.png` (1280x400 oder 1920x400)
- Zeigt das Produkt, die UI, oder das Kern-Ergebnis
- Wenn keine UI existiert (CLI/Library): weglassen oder generische Architektur-Grafik
- tldraw nutzt `assets/github-hero-light.png`, Next.js nutzt Vercel-CDN

### 2. Badges (Max 3-4)

```html
<p align="center">
  <a href="https://pypi.org/project/yourpackage/">
    <img src="https://img.shields.io/pypi/v/yourpackage" alt="PyPI" />
  </a>
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License" />
  </a>
  <a href="https://github.com/org/repo/stargazers">
    <img src="https://img.shields.io/github/stars/org/repo?style=social" alt="Stars" />
  </a>
</p>
```

**Welche Badges?**
- Package registry (PyPI/npm/crates.io) — **immer** wenn published
- License — **immer**
- Stars (social) — **optional** ab 100+ stars
- CI status — **optional** wenn gruen
- Discord/Community — **optional** wenn aktiv

**NIEMALS:** Last-Commit, Downloads-total, Topics, Python-version, Framework-logos. Das ist Clutter.

### 3. Tagline (Ein Satz, kursiv, zentriert)

```html
<p align="center">
  <em>Never hit a rate limit again. 484 keys, 10 proxies, 12 models, one URL.</em>
</p>
```

**Regel:** Ein Satz der den **Nutzen** sagt, nicht das Feature. Max 15 Worte.

### 4. Quick Links (3 Links, punkt-getrennt)

```html
<p align="center">
  <a href="#quick-start">Quick Start</a> ·
  <a href="#features">Features</a> ·
  <a href="#documentation">Documentation</a>
</p>
```

**Max 3-5 Links.** tldraw nutzt: `Docs . Examples . Starter kits`. Mehr als 5 = Clutter.

### 5. Quick Start (1-2 Code-Bloecke, maximal 5 Zeilen)

```markdown
## Quick Start

```bash
pip install yourpackage
```

```python
from yourpackage import Client
client = Client()
result = client.do_thing()
print(result)
```
```

**Regeln:**
- Ein `install` Befehl + ein `usage` Snippet
- Max 5 Zeilen pro Block
- Kein `git clone` + `cd` + `pip install` + `config` + `run` — das ist kein Quick Start, das ist eine Anleitung
- Wenn Setup komplex ist: Link zu `/docs/setup.md`

### 6. Features (Bullet-List, keine Tabelle)

```markdown
## Features

- **Automated Key Generation** — GMX alias rotation, OTP verification, API key extraction
- **10-Proxy Auto-Failover** — Router distributes across 10 proxies with automatic switch
- **Silent Key Swap** — On 429/401/403, proxy swaps key without client noticing
- **OpenAI-Compatible** — One URL works with any OpenAI-compatible client
```

**Format:** `**Term** — Description` (em-dash, nicht colon)

**WARUM keine Tabelle:** Tabellen sind schwer lesbar auf Mobile, force column widths, und sehen wirtdlich aus. Bullets sind scannbar, natuerlich, und was alle Top-Repos nutzen.

### 7. Social Proof (Wer nutzt es?)

```markdown
## Used By

Trusted by teams at **Google**, **Shopify**, **BlackRock**, **Autodesk**, **ClickUp**, **Replit**, and many more.
```

**Regeln:**
- Text-Logos, keine Bild-Logos (tldraw pattern)
- Wenn keine bekannten Nutzer: weglassen
- Wenn Open Source mit Stars: Star History Chart stattdessen

### 8. Examples / Visual Proof (Wenn anwendbar)

```markdown
## Examples

![Demo](./assets/demo.gif)
```

**Regeln:**
- GIF/Video fuer UI-Tools (screenshot-to-code pattern)
- Screenshot fuer CLI-Tools
- Vorher/Nachher-Vergleich wenn moeglich
- **Keine** dunklen Charts die auf GitHub nicht lesbar sind

### 9. Star History (Optional, am Ende)

```html
## Star History

<a href="https://star-history.com/#org/repo&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=org/repo&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=org/repo&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=org/repo&type=Date" />
  </picture>
</a>
```

**tldraw pattern** — dual-theme SVG, funktioniert in Light und Dark mode.

### 10. Footer (Minimal, subtil)

```html
<p align="center">
  <sub>Built by <a href="https://example.com">YourOrg</a>. MIT Licensed.</sub>
</p>
```

**Keine for-the-badge Banner.** Keine Emoji-Links. Subtil ist professionell.

---

## COMPLETE SOTA TEMPLATE

```markdown
<a name="readme-top"></a>

<!-- HERO IMAGE (only if visual product) -->
<p align="center">
  <img src="./assets/hero.png" alt="{{REPO_NAME}}" width="800" />
</p>

<!-- BADGES (max 3-4) -->
<p align="center">
  <a href="https://pypi.org/project/{{PACKAGE}}/">
    <img src="https://img.shields.io/pypi/v/{{PACKAGE}}" alt="PyPI" />
  </a>
  <a href="./LICENSE">
    <img src="https://img.shields.io/badge/license-MIT-blue.svg" alt="License" />
  </a>
  <a href="https://github.com/{{ORG}}/{{REPO}}/stargazers">
    <img src="https://img.shields.io/github/stars/{{ORG}}/{{REPO}}?style=social" alt="Stars" />
  </a>
</p>

<!-- TAGLINE -->
<p align="center">
  <em>{{ONE_SENTENCE_BENEFIT}}</em>
</p>

<!-- QUICK LINKS (max 3-5) -->
<p align="center">
  <a href="#quick-start">Quick Start</a> &middot;
  <a href="#features">Features</a> &middot;
  <a href="#documentation">Documentation</a>
</p>

---

# {{REPO_NAME}}

{{TWO_SENTENCE_DESCRIPTION}}

## Quick Start

```bash
{{INSTALL_COMMAND}}
```

```python
{{USAGE_SNIPPET_5_LINES}}
```

> [!NOTE]
> For full setup instructions, see [docs/setup.md](docs/setup.md).

## Features

- **{{FEATURE_1}}** — {{BENEFIT_1}}
- **{{FEATURE_2}}** — {{BENEFIT_2}}
- **{{FEATURE_3}}** — {{BENEFIT_3}}
- **{{FEATURE_4}}** — {{BENEFIT_4}}
- **{{FEATURE_5}}** — {{BENEFIT_5}}

## Used By

{{SOCIAL_PROOF_OR_REMOVE_SECTION}}

## Documentation

Full documentation at **[{{DOCS_URL}}]({{DOCS_URL}})**.

## Community

- [Discord](https://discord.gg/...) — questions and discussion
- [Twitter/X](https://twitter.com/...) — news and updates
- [Issues](https://github.com/{{ORG}}/{{REPO}}/issues) — bug reports and feature requests

## Contributing

1. Fork the repository
2. Create your branch (`git checkout -b feature/amazing-feature`)
3. Test your changes
4. Commit and push
5. Open a Pull Request

See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

## License

Distributed under the **MIT License**. See [LICENSE](LICENSE) for details.

<!-- STAR HISTORY (optional) -->
## Star History

<a href="https://star-history.com/#{{ORG}}/{{REPO}}&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos={{ORG}}/{{REPO}}&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos={{ORG}}/{{REPO}}&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos={{ORG}}/{{REPO}}&type=Date" />
  </picture>
</a>

<p align="center">
  <sub>Built by <a href="https://{{ORG_URL}}">{{ORG_NAME}}</a>. MIT Licensed.</sub>
</p>
```

---

## MANDATORY WORKFLOW

### Step 1: Repo analysieren

```bash
git log --oneline -20                    # Letzte Commits
gh repo view --json name,description     # Repo metadata
ls -la                                   # File structure
cat package.json 2>/dev/null || cat pyproject.toml 2>/dev/null  # Tech stack
```

### Step 2: Live-Metriken holen (PFLICHT)

**NIEMALS hardcoded Zahlen! IMMER live abfragen:**

```bash
# GitHub stats
gh repo view --json stargazerCount,forkCount,description,createdAt,updatedAt

# API stats (falls vorhanden)
curl -s http://localhost:PORT/api/stats | python3 -m json.tool

# Package stats
curl -s https://pypistats.org/api/packages/PACKAGE/recent | python3 -m json.tool
```

### Step 3: Charts generieren (PFLICHT bei Daten)

```bash
# Bar chart
echo '{"title":"...","categories":[...],"series":[{"name":"...","values":[...]}]}' | \
  sin-code image-graph --type bar --output assets/chart-name

# Pie chart  
echo '{"title":"...","items":[{"label":"...","value":N},...]}' | \
  sin-code image-graph --type pie --output assets/chart-name
```

**Charts MUESSEN auf GitHub lesbar sein.** Teste mit `sin-brain_open_image_in_preview` vor commit.

### Step 4: README schreiben

Befolge die 10 Gebote. Nutze das Template. English only.

### Step 5: llms.txt (Optional)

```
# {{REPO_NAME}}

> {{ONE_SENTENCE_DESCRIPTION}}

> {{TWO_SENTENCE_DETAIL}}

## Quick Start
{{INSTALL_COMMAND}}

## Documentation
{{DOCS_URL}}
```

---

## REPO-TYP ANPASSUNGEN

| Repo-Typ | Hero Image | Quick Start | Examples | Social Proof |
|----------|-----------|-------------|----------|-------------|
| **Library/Package** | Nein | `pip install` + usage | Code snippets | Star history |
| **CLI Tool** | Nein | Install + 1 command | Terminal screenshot | Star history |
| **Web App** | Ja (UI screenshot) | `npm run dev` | GIF demo | Star history |
| **API/Service** | Nein | `docker up` + curl | curl example | Star history |
| **AI/Agent** | Ja (output example) | Install + prompt | Output screenshot | Used by |

---

## QUALITY CHECKLIST (vor commit)

- [ ] Hero Image (wenn visuelles Repo) — eine PNG, max 1920x400
- [ ] Max 3-4 Badges — PyPI/npm + License + Stars
- [ ] Ein-Zeiliger Tagline (kursiv, zentriert)
- [ ] 3-5 Quick Links (punkt-getrennt)
- [ ] Quick Start: 1-2 Code-Bloecke, max 5 Zeilen
- [ ] Features als Bullet-List (keine Tabelle)
- [ ] Kein Mermaid-Diagramm im README
- [ ] Keine `<details>` collapsible sections
- [ ] Keine Navigation-Anchor-Links (ausser Quick Links oben)
- [ ] Kein Emoji in Headings
- [ ] English only (kein Deutsch)
- [ ] Keine hardcoded Zahlen (live geholt)
- [ ] Charts auf GitHub lesbar (heller Hintergrund fuer PNGs)
- [ ] Star History Chart (wenn 100+ stars)
- [ ] Minimaler Footer (`<sub>` Text, kein for-the-badge Banner)
- [ ] llms.txt (optional, wenn AI-discoverability gewuenscht)

---

## CHART INTEGRATION (sin-code image-graph)

Wenn die README Daten enthaelt (benchmarks, stats, model comparison):

```bash
# Bar chart for comparisons
echo '{"title":"Model Context Windows","yLabel":"K tokens","categories":["Model A","Model B"],"series":[{"name":"Context","values":[128,256]}]}' | \
  sin-code image-graph --type bar --output assets/context-comparison

# Pie chart for distributions
echo '{"title":"Pool Status","items":[{"label":"Available","value":41},{"label":"Suspended","value":431}]}' | \
  sin-code image-graph --type pie --output assets/pool-status
```

**WICHTIG:** Charts muessen auf GitHub (Light-Mode!) lesbar sein. Wenn du dunkle Charts generierst, teste sie vorher mit `sin-brain_open_image_in_preview`. GitHub's README-Viewer ist Light-Mode by default.

**Embed in README:**
```html
<p align="center">
  <img src="./assets/chart-name.png" alt="Chart Title" width="640" />
</p>
```

---

## REFERENZ-REPOS ( fuer Inspiration)

| Repo | Stars | Key Pattern |
|------|-------|-------------|
| [vercel/next.js](https://github.com/vercel/next.js) | 140k | Hero logo, 4 badges, ultra-minimal |
| [openai/openai-python](https://github.com/openai/openai-python) | 31k | PyPI badge, code-heavy, clean sections |
| [tldraw/tldraw](https://github.com/tldraw/tldraw) | 48k | Hero image, 4 badges, bullet features, star history |
| [abi/screenshot-to-code](https://github.com/abi/screenshot-to-code) | 73k | GIF demos, no badges, example-driven |
| [anthropics/anthropic-sdk-python](https://github.com/anthropics/anthropic-sdk-python) | 3.6k | PyPI badge, ultra-clean, minimal |

**These are the gold standard. Study them before writing any README.**
