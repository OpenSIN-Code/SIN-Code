---
name: skill-github-readme
description: "Transform ANY repository into a visually stunning, instantly understandable experience. Hero banner, SOTA badges, benefit-driven bullets, clean code blocks, social proof, star history. Based on research of 5 top GitHub repos (Anthropic, OpenAI, Vercel, tldraw, screenshot-to-code). Embeds sin-code image-graph charts for benchmark data."
license: MIT
compatibility:
  - opencode
  - sin-code
  - claude-code
  - codex
metadata:
  audience: all-engineering-levels
  mode: autonomous-visual-enhancement
  language: en
  coupled_with: skill-github-governance
  integrates_with: sin-code image-graph
  version: "5.0"
  last_updated: "2026-06-18"
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/visual-repo"
required_tools:
  - sin_write
  - sin_image_graph
lifecycle: external
---

> SIN-Code Bundled Skill v5.0 — SOTA README Standard. Based on deep research of 10 top GitHub repos (Anthropic SDK 3.6k, OpenAI Python 31k, Vercel Next.js 140k, tldraw 48k, screenshot-to-code 73k, Microsoft VS Code 186k, Rust 114k, LangChain 140k, Supabase 104k, LlamaIndex 50k). Macht JEDES Repo visuell verstaendlich, AI-discoverable, und professionell.

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

## DIE SOTA-README-BLUEPRINT

Based on deep research of 10 top GitHub repos (186k to 3.6k stars):

```
Hero Image (optional) -> Badges -> Tagline -> Quick Links -> Quick Start -> 
Features -> Stats/Charts -> Models/Details -> API/Usage -> 
Architecture -> Contributing -> License -> Footer
```

**Keine starren Regeln — orientiere dich an den Top-Repos, passe an dein Repo an.**

### Patterns — wann nutzen, wann nicht

Diese Patterns sind **Werkzeuge**, keine Anti-Patterns. Nutze sie bewusst:

| Pattern | Wann nutzen | Wann NICHT |
|---------|------------|-----------|
| Mermaid-Diagramm | Komplexe Architektur mit >5 Komponenten, Flows, Sequences | Bei einfachen 3-Komponenten-Setups — dann reicht Text |
| `<details>` sections | Lange Config-Beispiele, Troubleshooting, optionale Details | Für Kern-Features — die sollen sichtbar sein |
| Navigation-Links | Bei READMEs >200 Zeilen mit vielen Sektionen | Bei kurzen READMEs <100 Zeilen — dann clutter |
| HTML-Tabellen-Layout | 3-Spalten Quick-Start (Clone/Install/Run), Side-by-Side Vergleiche | Für normale Tabellen — Markdown-Tabelle ist sauberer |
| Emoji in Headings | Bei fun/playful Projekten (LlamaIndex nutzt 🦙) | Bei Enterprise/B2B Projekten — dort unprofessionell |
| 6+ Badges | Wenn sie ALLE relevant sind (PyPI + Downloads + CI + Discord + License + Version = LlamaIndex pattern) | Wenn es nur filler sind (Last-Commit, Topics) |
| Feature-Tabelle mit ✅ | Feature-Vergleich gegen Konkurrenten (✅ vs ❌) | Bei eigener Feature-Liste — Bullets sind besser |
| `back to top` Links | Bei READMEs >300 Zeilen | Bei kurzen READMEs — GitHub hat Scroll |
| Footer-Banner ("Powered by") | Wenn es ein echtes Branding gibt (Supabase "Made with Supabase" ist SOTA) | Wenn es nur Werbung ist ohne Mehrwert |
| Deutsch im README | Nur wenn das Repo explizit DACH-only ist | Default = English für internationalen Reach |

---

## SOTA-PATTERNS (von 10 Top-Repos gelernt)

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
- Downloads (PyPI stats) — **gut fuer Popularitaet** (LangChain nutzt es)
- CI status — **optional** wenn gruen
- Discord/Community — **optional** wenn aktiv
- Version — **gut** (LangChain, LlamaIndex nutzen es)

**LlamaIndex hat 7 Badges** (Downloads, Build, Contributors, Discord, Twitter, Reddit, Ask AI) — und das sieht gut aus weil sie ALLE relevant sind. 6+ ist ok wenn jeder Badge echten Mehrwert bringt. **Filler-Badges** (Last-Commit, Topics, Framework-logos ohne Kontext) = weg.

### 3. Tagline (Ein Satz, kursiv, zentriert)

```html
<p align="center">
  <em>Never hit a rate limit again. 484 keys, 10 proxies, 12 models, one URL.</em>
</p>
```

**Regel:** Ein Satz der den **Nutzen** sagt, nicht das Feature. Max 15 Worte.

### 4. Quick Links (Pipe-separated, Rust pattern)

```html
<p align="center">
  <a href="#quick-start">Quick Start</a> |
  <a href="#features">Features</a> |
  <a href="#models">Models</a> |
  <a href="#api">API</a> |
  <a href="#contributing">Contributing</a>
</p>
```

**Pipe `|` nicht dot `·`.** Rust (114k stars) nutzt `Website | Getting started | Learn | Documentation | Contributing`. Max 3-5 links.

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

### 8a. Chart Embedding — VOLLE BREITE (PFLICHT!)

**Charts und Bilder MUESSEN volle Breite nutzen.** Kein `width="640"` — das macht sie small und "lost".

**FALSCH (chart sieht lost aus):**
```html
<p align="center">
  <img src="./assets/pool-status.png" alt="Pool Status" width="640" />
</p>
```

**RICHTIG (volle breite, impactful):**
```markdown
![Pool Status](./assets/pool-status.png)
```

Oder fuer zentrierte Vollbreite ohne width:
```html
<p align="center">
  <img src="./assets/pool-status.png" alt="Pool Status" />
</p>
```

**Warum:** GitHub's content area is ~800px wide. `width="640"` schrumpft das Bild und laesst es "lost" aussehen. VS Code (186k stars), Supabase (104k stars), tldraw (48k stars) — alle nutzen VOLLBREITE Bilder ohne width-Attribut.

**Alternativ: Charts in Kontext einbetten** — nicht alleine floaten. Umgebe sie mit beschreibendem Text davor UND danach:

```markdown
The pool currently manages **484 keys** across 10 proxies:

![Pool Status](./assets/pool-status.png)

Most keys are suspended (431) due to Fireworks spending caps, while **43 remain available** for rotation.
```

**Chart-Qualitaet:** Charts muessen auf GitHub's Light-Mode lesbar sein. Wenn du dunkle Charts generierst (sin-code image-graph), generiere sie mit hellem Hintergrund oder teste mit `sin-brain_open_image_in_preview` vor commit.

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

### 10. SIN AI Banner System (PFLICHT für alle OpenSIN AI Repos)

Zwei Banner, zwei Zwecke:

| Position | Zweck | Individuell? | Dateien |
|---|---|---|---|
| **Header** (top of README) | Repo-Hero — zeigt was DIESSES Repo macht | **JA** — pro Repo einzigartig | `hero-banner.svg` + `hero-banner-light.svg` |
| **Footer** (bottom of README) | "Powered by OpenSIN AI" — generisches Branding | **NEIN** — gleiches Design für alle Repos | `sin-ai-banner.svg` + `sin-ai-banner-light.svg` |

Beide Banner sind **custom-designed SVGs** (NICHT shields.io badges). Sie nutzen dual-mode via `<picture>` + `prefers-color-scheme`.

#### 10a. Header Hero Banner (individuell pro Repo)

```html
<a name="readme-top"></a>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/hero-banner.svg" />
  <source media="(prefers-color-scheme: light)" srcset="./assets/hero-banner-light.svg" />
  <img src="./assets/hero-banner.svg" alt="{{REPO_NAME}}" />
</picture>
```

**Was das Header-Banner zeigt (pro Repo):**
- **Repo-Name** (gross, bold, gradient) + **Subtitle** (was es ist)
- **Tagline** (ein Satz Benefit)
- **3 Key-Metrics** als Visual Cards (z.B. `484 Keys`, `10 Proxies`, `12 Models`)
- **Mini-Architecture-Flow** (optional, wenn visuell sinnvoll)
- **Quick-start hint** (ein command, monospace)
- **Hex grid + glow effects + HUD corners** (Design Language)
- **Logo** embedded als base64 data URI (GitHub strips relative `<image href>`)

**Design-Spec Header:**
- Format: 1200×280 px (hero format, breit)
- Font: `ui-sans-serif, system-ui` (Stripe/Linear style, weight 500-600)
- Dark: `#0A0E14` bg, `#00D9FF`/`#7B3FE4` accents
- Light: `#FAFAFA` bg, `#00A8D8`/`#8B5CF6` accents
- Logo: 60×60px, base64 embedded (8KB), `feGaussianBlur` glow
- Metric Cards: dark cards mit accent top-bar, grossen Zahlen, kleinen Labels
- HUD corners: L-shaped accent lines in 4 Ecken
- Scanning line + pulse ring (subtle animation, GitHub-safe)

**WICHTIG — Logo als base64:**
GitHub sanitizes SVGs und strips `<image href="./logo.png">` mit relativen Pfaden.
Logo MUSS als `data:image/png;base64,...` direkt im SVG eingebettet sein.
Resize logo to 60×60 before encoding (~8KB base64).

#### 10b. Footer Branding Banner (generisch für alle Repos)

```html
---

<!-- OpenSIN AI BRANDING FOOTER -->
<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/sin-ai-banner.svg" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/sin-ai-banner-light.svg" />
    <img src="./assets/sin-ai-banner.svg" alt="OpenSIN AI" />
  </picture>
</p>
```

**Was das Footer-Banner zeigt (gleich für alle Repos):**
- **"OpenSIN AI"** (gradient text, `ui-sans-serif, system-ui`, weight 500)
- **Tagline**: "Enterprise AI agents that work autonomously"
- **Repo-specific benefits** (3 Items, pipe-separated, z.B. "484 free API keys · 10-proxy auto-failover · 12 Fireworks models")
- **OpenSIN logo** (embedded als base64 data URI, 100×100px)
- **Hex grid + neon glow + pulse ring + scanning line** (gleiche Design Language)

**Design-Spec Footer:**
- Format: 1200×200 px (schmaler als Header)
- Font: `ui-sans-serif, system-ui`, weight 500, letter-spacing 2
- Dark: `#0B1120` bg, neon glow filters
- Light: `#FAFAFA` bg, softer accents
- Logo: 100×100px, base64 embedded (~18KB)

**WARUM custom SVGs statt shields.io:**
- shields.io badges sind generisch und austauschbar — jedes GitHub Repo hat sie
- Custom SVGs sind einzigartig und wiederkennbar (wie tldraw, Anthropic)
- Full control über Layout, Typography, Color, Animation
- Dual-mode dark/light via `<picture>` + `prefers-color-scheme`
- GitHub cached SVGs — einmal committed, immer da
- Keine externen Dependencies (shields.io kann down sein)

---

## COMPLETE SOTA TEMPLATE

```markdown
<a name="readme-top"></a>

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

<!-- QUICK LINKS (pipe-separated, Rust pattern) -->
<p align="center">
  <a href="#quick-start">Quick Start</a> |
  <a href="#features">Features</a> |
  <a href="#documentation">Documentation</a>
</p>

<!-- HERO BANNER (custom SVG, repo-specific, dual-mode) -->
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="./assets/hero-banner.svg" />
  <source media="(prefers-color-scheme: light)" srcset="./assets/hero-banner-light.svg" />
  <img src="./assets/hero-banner.svg" alt="{{REPO_NAME}}" />
</picture>

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

---

<!-- OpenSIN AI BRANDING FOOTER (PFLICHT) -->
<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="./assets/sin-ai-banner.svg" />
    <source media="(prefers-color-scheme: light)" srcset="./assets/sin-ai-banner-light.svg" />
    <img src="./assets/sin-ai-banner.svg" alt="OpenSIN AI" />
  </picture>
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

- [ ] Hero Image (wenn visuelles Repo) — VOLLBREITE, kein width-Attribut
- [ ] Badges — nur relevante, kein Filler (6+ ok wenn alle Mehrwert bringen)
- [ ] Tagline — ein Satz Nutzen, kursiv zentriert
- [ ] Quick Links — pipe-separated `|`, 3-5 links (bei langen READMEs)
- [ ] Quick Start — 1-2 Code-Bloecke, max 5 Zeilen
- [ ] Features — Bullet-List mit `**Bold** — description`
- [ ] Charts VOLLBREITE — kein `width="640"`, in Kontext eingebettet
- [ ] Charts auf GitHub lesbar (heller Hintergrund fuer PNGs)
- [ ] Mermaid — nur bei komplexer Architektur (>5 Komponenten), nicht bei einfachen Setups
- [ ] `<details>` — nur fuer optionale Details/Troubleshooting, nicht fuer Kern-Features
- [ ] Emoji in Headings — nur bei fun/playful Projekten
- [ ] English default (Deutsch nur bei DACH-only repos)
- [ ] Keine hardcoded Zahlen (live geholt per curl/gh)
- [ ] Star History Chart (wenn 100+ stars)
- [ ] SIN AI Branding Footer (PFLICHT — 3-Tier: for-the-badge + flat-square links + sub text)
- [ ] llms.txt (optional)

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

## REFERENZ-REPOS (fuer Inspiration)

Deep research of 10 top GitHub repos:

| Repo | Stars | Key Pattern |
|------|-------|-------------|
| [microsoft/vscode](https://github.com/microsoft/vscode) | 186k | Full-width hero screenshot, 3 badges, no emoji headings |
| [rust-lang/rust](https://github.com/rust-lang/rust) | 114k | Hero SVG, pipe-separated links, "Why Rust?" bold bullets |
| [langchain-ai/langchain](https://github.com/langchain-ai/langchain) | 140k | Dark logo, 4 badges, Tip callouts, ecosystem bold links |
| [supabase/supabase](https://github.com/supabase/supabase) | 104k | Dual-mode hero (light/dark), full-width dashboard, SVG architecture |
| [vercel/next.js](https://github.com/vercel/next.js) | 140k | Hero logo, 4 badges, ultra-minimal |
| [openai/openai-python](https://github.com/openai/openai-python) | 31k | PyPI badge, code-heavy, clean sections |
| [tldraw/tldraw](https://github.com/tldraw/tldraw) | 48k | Hero image, 4 badges, bullet features, star history |
| [abi/screenshot-to-code](https://github.com/abi/screenshot-to-code) | 73k | GIF demos, no badges, example-driven |
| [run-llama/llama_index](https://github.com/run-llama/llama_index) | 50k | 6 badges, big docs blockquote, code-heavy examples |
| [anthropics/anthropic-sdk-python](https://github.com/anthropics/anthropic-sdk-python) | 3.6k | PyPI badge, ultra-clean, minimal |

**These are the gold standard. Study them before writing any README.**

### Key findings from deep research:

1. **Images = VOLLBREITE**: Kein einziges Top-Repo nutzt `width="640"`. VS Code, Supabase, tldraw — alle volle Breite.
2. **Architektur = SVG Datei**: Supabase speichert Architektur als `.svg` im Repo, nicht als Mermaid oder ASCII.
3. **Quick Links = Pipe-separated**: Rust nutzt `Website | Getting started | Learn | Documentation | Contributing`.
4. **"Why X?" Sektion**: Rust und LangChain nutzen beide eine "Why?" Sektion mit bold terms und em-dash.
5. **Keine Navigation-Anchor-Links**: Nirgends in Top-Repos.
6. **Dashboard-Screenshots**: Supabase und VS Code — volle Breite, klickbar.
7. **GitHub Alerts**: LangChain nutzt `> [!TIP]` callouts, Supabase nutzt `> [!NOTE]`.
8. **Dual-mode hero**: Supabase nutzt `#gh-light-mode-only` und `#gh-dark-mode-only` fuer dark/light Switch.
9. **Logo als SVG**: Rust und LangChain nutzen SVG logos im Repo, nicht PNG.
