# PLAN: README Verbesserung für alle 7 Tools

**Ziel:** Aus dünnen 39-60 Zeilen README → ausführliche Dokumentation mit Installation, Beispielen, Troubleshooting.

**Status:** 🟡 WICHTIG
**Aufwand:** ~15 Minuten pro Tool = ~2 Stunden total

---

## README Struktur (Template)

Jedes README sollte enthalten:

```markdown
# SIN-Code-Discover-Tool

> **Deep file discovery with relevance scoring for opencode CLI**

## ✨ Features

- 🎯 **Relevance Scoring** — Dateien werden nach Wichtigkeit sortiert
- 📊 **Multi-Format Output** — JSON, YAML, text
- 🔍 **Pattern Matching** — Glob patterns für flexible Suche
- ⚡ **Fast** — Durchsucht 10.000+ Dateien in <1s
- 🛡️ **Safe** — Keine Code Execution, nur Read-Only

## 📦 Installation

### Via sin bundle (Recommended)
\`\`\`bash
sin sin-code run discover
\`\`\`

### Manual
\`\`\`bash
git clone https://github.com/OpenSIN-Code/SIN-Code-Discover-Tool.git
cd SIN-Code-Discover-Tool
go build -o ~/.local/bin/discover cmd/discover/main.go
\`\`\`

## 🚀 Usage

### Basic
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT
\`\`\`

### With Pattern
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -pattern "**/*.py"
\`\`\`

### JSON Output
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -format json
\`\`\`

### Sort by Relevance
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -sort_by relevance -max_results 10
\`\`\`

## 📋 Parameters

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `-path` | string | `.` | Directory to search |
| `-pattern` | string | `**/*.go` | Glob pattern |
| `-sort_by` | string | `relevance` | `relevance`, `name`, `modified`, `size`, `lines` |
| `-max_results` | int | `100` | Maximum number of results |
| `-format` | string | `text` | `text`, `json`, `yaml` |

## 🔌 MCP Server

\`\`\`bash
discover --mcp
\`\`\`

## 🐛 Troubleshooting

### "Permission denied"
- Ensure you have read access to the directory
- Try with `-path` pointing to a different location

### "No files found"
- Check your glob pattern
- Use `-pattern "**/*"` to match all files

## 📚 Examples

### Find all Python files
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -pattern "**/*.py"
\`\`\`

### Find largest files
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -sort_by size -max_results 20
\`\`\`

### Find recently modified files
\`\`\`bash
discover -path /Users/jeremy/dev/PROJECT -sort_by modified -max_results 10
\`\`\`

## 🤝 Contributing

See [AGENTS.md](AGENTS.md) for development guidelines.

## 📄 License

MIT
```

---

## Per-Tool Anpassungen

### Discover-Tool
- Features: Relevance Scoring, Multi-Format
- Beispiele: Python files, Largest files, Recent files

### Execute-Tool
- Features: Secret Redaction, Timeout, Safety Checks
- Beispiele: Simple command, Long-running, With env vars

### Map-Tool
- Features: Module Mapping, Dependency Graph
- Beispiele: Map Python project, Map Go project, Find dead code

### Grasp-Tool
- Features: Deep Analysis, Related Files, Context
- Beispiele: Analyze Go file, Analyze Python file, Find usages

### Scout-Tool
- Features: Regex, Semantic, Symbol Search
- Beispiele: Find function, Find class, Find imports

### Harvest-Tool
- Features: Caching, Auth, Structure Extraction
- Beispiele: GET request, POST with body, With auth

### Orchestrate-Tool
- Features: Task Management, Dependencies, Rollback
- Beispiele: Add task, List tasks, With dependencies

---

## Geschätzte Zeit

- Pro Tool: 15 Minuten
- Total: ~2 Stunden
