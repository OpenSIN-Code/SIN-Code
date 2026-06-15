# CEO AUDIT REPORT 2: Competitor Landscape Research

> Detaillierte Analyse der SOTA Coding CLIs und deren Features
> Datum: 2026-06-15 | Researcher: CEO-Detektiv-Mode

---

## 1. OpenAI Codex CLI

### 1.1 Overview
- **GitHub:** https://github.com/openai/codex
- **Stars:** 91.2k
- **Sprache:** Rust (96.1%)
- **Lizenz:** Apache-2.0
- **Release:** v0.139.0 (Jun 9, 2026)

### 1.2 Kernfeatures

**Agent Loop:**
- Vollintegrierter Agent Loop (PLAN → ACT → VERIFY → DONE)
- Streaming token output
- Automatic retry mit backoff
- Automatic context selection
- Automatic commit nach edits

**Tools:**
- `file_read` — Read file mit automatic context
- `file_write` — Write file mit diff-based editing
- `edit_file` — Edit file mit unified diff
- `shell` — Shell commands mit sandboxing (Docker/VM)
- `code_search` — Semantic code search mit LSP
- `browser` — Web browsing für documentation

**Editing:**
- **Diff-based editing** (unified diff format)
- **Multi-file editing** (koordinierte changes über mehrere Dateien)
- **Automatic context selection** (LLM wählt relevante files aus)
- **Automatic commit** (nach jeder Änderung)

**Code Navigation:**
- **LSP-Integration** (go-to-definition, find-references, hover)
- **Semantic search** (embedding-basiert)
- **Cross-reference navigation** (call graph, dependency graph)

**Testing:**
- **Automatic test execution** (nach jedem edit)
- **Automatic error fixing** (lint errors, type errors)
- **Sandboxed test execution** (in Docker/VM)

**Safety:**
- **Sandboxing** (Docker/VM für alle shell commands)
- **Permission system** (allow/deny/ask)
- **Human-in-the-loop** (für destructive operations)

**Integration:**
- **IDE-Integration** (VS Code, Cursor, Windsurf)
- **Desktop App** (standalone application)
- **Web Interface** (chatgpt.com/codex)

### 1.3 SOTA-Features die SIN-Code fehlen

1. **LSP-Integration**
   - Go-to-definition
   - Find-references
   - Hover information
   - Diagnostic messages
   - **Impact:** Ohne LSP kann SIN-Code keine echte code navigation

2. **Sandboxed Execution**
   - Docker/VM sandboxing für alle shell commands
   - Isolated test execution
   - **Impact:** SIN-Code hat nur pattern-basierte safety checks

3. **Automatic Context Selection**
   - LLM-basierte file selection
   - Smart context window management
   - **Impact:** SIN-Code erfordert manuelle file selection

4. **Streaming Output**
   - Token-by-token streaming
   - Real-time tool output
   - **Impact:** SIN-Code hat kein streaming (wartet auf komplette response)

5. **Multi-File Editing**
   - Koordinierte changes über mehrere Dateien
   - Automatic diff application
   - **Impact:** SIN-Code muss jede Datei einzeln editieren

---

## 2. Claude Code (Anthropic)

### 2.1 Overview
- **Website:** https://claude.ai/code
- **Docs:** https://docs.anthropic.com/en/docs/claude-code
- **Plattformen:** Terminal, VS Code, JetBrains, Desktop App, Web
- **Modelle:** Claude Opus, Sonnet, Haiku

### 2.2 Kernfeatures

**Agent Loop:**
- Vollintegrierter Agent Loop
- Streaming token output
- Automatic retry
- Automatic context selection
- Automatic commit

**Tools:**
- `read_file` — Read file
- `write_file` — Write file mit diff
- `edit_file` — Edit file mit unified diff
- `bash` — Shell commands mit permission system
- `codebase_search` — Semantic code search (embeddings)
- `file_search` — File search
- `list_dir` — List directory
- `web_fetch` — Fetch URLs mit HTML→Markdown

**Editing:**
- **Unified diff editing** (mit automatic context)
- **Multi-file editing** (koordinierte changes)
- **Automatic commit** (nach jeder Änderung)
- **Checkpoint system** (undo/redo)

**Code Navigation:**
- **Semantic search** (embedding-basiert)
- **Codebase search** (natural language queries)
- **Cross-reference navigation**

**Testing:**
- **Automatic test execution**
- **Automatic error fixing**
- **Lint integration**

**Safety:**
- **Permission system** (allow/deny/ask)
- **Human-in-the-loop**
- **Checkpoint system** (rollback)

**Memory:**
- **CLAUDE.md** (project instructions)
- **Auto memory** (automatische learnings)
- **Session history** (resumable sessions)

**Multi-Agent:**
- **Sub-agents** (parallel task execution)
- **Background agents** (long-running tasks)
- **Agent SDK** (custom agents)

**Integration:**
- **VS Code Extension**
- **JetBrains Plugin**
- **Desktop App**
- **Web Interface**
- **GitHub Actions**
- **GitLab CI/CD**
- **Slack Integration**

### 2.3 SOTA-Features die SIN-Code fehlen

1. **Semantic Search (Embeddings)**
   - Embedding-basierte code search
   - Natural language queries
   - **Impact:** SIN-Code hat nur regex/substring search

2. **Sub-Agents**
   - Parallel task execution
   - Multi-agent coordination
   - **Impact:** SIN-Code hat nur single-agent loop

3. **Memory System**
   - CLAUDE.md für project instructions
   - Auto memory für learnings
   - **Impact:** SIN-Code hat nur primitives learning loop

4. **Checkpoint System**
   - Undo/redo für alle changes
   - Rollback zu jedem checkpoint
   - **Impact:** SIN-Code hat kein checkpoint system

5. **HTML→Markdown Conversion**
   - Automatische conversion von web content
   - **Impact:** SIN-Code harvest hat kein HTML parsing

6. **GitHub Actions Integration**
   - Automatische PR reviews
   - CI/CD integration
   - **Impact:** SIN-Code hat keine CI/CD integration

---

## 3. Aider

### 3.1 Overview
- **GitHub:** https://github.com/Aider-AI/aider
- **Stars:** 46.3k
- **Sprache:** Python (80%)
- **Lizenz:** Apache-2.0
- **Release:** v0.86.0 (Aug 9, 2025)

### 3.2 Kernfeatures

**Agent Loop:**
- Pair programming paradigm
- Automatic context selection via repo-map
- Automatic commit nach edits
- Automatic lint + test loop

**Editing:**
- **Diff-based editing** (unified diff format)
- **Whole-file editing** (für kleinere changes)
- **Multi-file editing** (koordinierte changes)
- **Automatic commit** (nach jeder Änderung)

**Code Navigation:**
- **Repo-map** (tree-sitter-basierte code maps)
- **Symbol tracking** (über alle Dateien)
- **Dependency graph** (import-based)

**Testing:**
- **Automatic test execution** (nach jedem edit)
- **Automatic lint checking**
- **Automatic error fixing** (lint errors, test failures)

**LLM Support:**
- **Multi-model support** (Claude, GPT-4, DeepSeek, local models)
- **Model switching** (während der session)
- **API key management**

**Features:**
- **Voice-to-code** (speech-to-text für prompts)
- **Image support** (screenshots, diagrams)
- **Web page ingestion** (URLs als context)
- **IDE integration** (watch mode für file changes)

### 3.3 SOTA-Features die SIN-Code fehlen

1. **Repo-Map (Tree-Sitter)**
   - Tree-sitter-basierte code maps
   - Symbol tracking über alle Dateien
   - **Impact:** SIN-Code hat nur oberflächliche dependency extraction

2. **Automatic Lint + Test Loop**
   - Automatic lint checking nach edits
   - Automatic test execution
   - Automatic error fixing
   - **Impact:** SIN-Code hat kein automatic testing

3. **Voice-to-Code**
   - Speech-to-text für prompts
   - **Impact:** SIN-Code hat kein voice input

4. **Image Support**
   - Screenshots als context
   - Diagrams als context
   - **Impact:** SIN-Code hat kein image support

5. **Watch Mode**
   - Automatische re-execution bei file changes
   - **Impact:** SIN-Code hat kein watch mode

---

## 4. Cline

### 4.1 Overview
- **GitHub:** https://github.com/cline/cline
- **Stars:** 63.3k
- **Sprache:** TypeScript (97.9%)
- **Lizenz:** Apache-2.0
- **Release:** CLI v3.0.24 (Jun 11, 2026)

### 4.2 Kernfeatures

**Agent Loop:**
- Vollintegrierter Agent Loop
- Plan mode + Act mode
- Automatic context selection
- Automatic commit

**Editing:**
- **Diff-based editing** (unified diff)
- **Multi-file editing**
- **Automatic commit**
- **Checkpoint system** (undo/redo)

**Code Navigation:**
- **Project structure understanding**
- **File relationship tracking**
- **Linter/compiler error monitoring**

**Multi-Agent:**
- **Multi-agent teams** (coordinator + specialists)
- **Parallel task execution**
- **Team state persistence**

**Kanban:**
- **Web-based task board**
- **Multi-agent parallel execution**
- **Auto-commit pro task**
- **Dependency chains**

**Scheduled Agents:**
- **Cron-based scheduling**
- **Recurring automations**
- **Persistent schedules**

**Integration:**
- **VS Code Extension**
- **JetBrains Plugin**
- **CLI** (interactive + headless)
- **SDK** (custom agents)
- **Slack/Telegram/Discord** (messaging integration)

**MCP Support:**
- **MCP server integration**
- **Custom tool creation**
- **Community MCP servers**

### 4.3 SOTA-Features die SIN-Code fehlen

1. **Kanban Board**
   - Web-based multi-agent task board
   - Parallel execution mit auto-commit
   - **Impact:** SIN-Code hat nur primitives task management

2. **Multi-Agent Teams**
   - Coordinator + specialist agents
   - Parallel task execution
   - **Impact:** SIN-Code hat nur single-agent loop

3. **Scheduled Agents**
   - Cron-based scheduling
   - Recurring automations
   - **Impact:** SIN-Code hat scheduler skill aber keine agent integration

4. **Messaging Integration**
   - Slack/Telegram/Discord integration
   - **Impact:** SIN-Code hat kein messaging integration

5. **SDK**
   - Programmatic agent API
   - Custom tool creation
   - **Impact:** SIN-Code hat kein SDK

---

## 5. Roo Code (Archived)

### 5.1 Overview
- **GitHub:** https://github.com/RooCodeInc/Roo-Code
- **Stars:** 24.2k
- **Status:** **ARCHIVED** (May 15, 2026)
- **Nachfolger:** ZooCode (community fork)

### 5.2 Kernfeatures

**Modes:**
- **Code Mode** (everyday coding)
- **Architect Mode** (planning, specs)
- **Ask Mode** (questions, explanations)
- **Debug Mode** (tracing, logging)
- **Custom Modes** (user-defined)

**Editing:**
- **Diff-based editing**
- **Multi-file editing**
- **Automatic commit**

**Code Navigation:**
- **Project structure understanding**
- **File relationship tracking**

**MCP Support:**
- **MCP server integration**
- **Custom tool creation**

### 5.3 Lessons Learned
- **Multi-mode approach** ist nützlich für unterschiedliche workflows
- **Custom modes** ermöglichen team-spezifische workflows
- **Archivierung** zeigt dass der Markt sich schnell konsolidiert

---

## 6. Vergleichsmatrix

| Feature | Codex CLI | Claude Code | Aider | Cline | SIN-Code |
|---------|-----------|-------------|-------|-------|----------|
| **Agent Loop** | ✅ Voll | ✅ Voll | ✅ Pair | ✅ Voll | ⚠️ Primitiv |
| **Streaming** | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Diff Editing** | ✅ Unified | ✅ Unified | ✅ Unified | ✅ Unified | ❌ String Replace |
| **Multi-File Edit** | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Auto Context** | ✅ LLM | ✅ LLM | ✅ Repo-Map | ✅ | ❌ |
| **Auto Commit** | ✅ | ✅ | ✅ | ✅ | ❌ |
| **Auto Test** | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **Auto Lint** | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **Auto Fix** | ✅ | ✅ | ✅ | ⚠️ | ❌ |
| **LSP Integration** | ✅ | ⚠️ Internal | ❌ | ❌ | ❌ |
| **Semantic Search** | ✅ | ✅ Embeddings | ❌ | ❌ | ❌ |
| **Sandboxing** | ✅ Docker/VM | ⚠️ Permission | ❌ | ❌ | ❌ |
| **Sub-Agents** | ✅ | ✅ | ❌ | ✅ | ❌ |
| **Multi-Agent** | ✅ | ✅ | ❌ | ✅ Teams | ❌ |
| **Memory** | ✅ | ✅ CLAUDE.md | ❌ | ✅ | ⚠️ Primitiv |
| **Checkpoint** | ✅ | ✅ | ✅ Auto | ✅ | ❌ |
| **IDE Integration** | ✅ VS Code | ✅ VS Code + JB | ⚠️ Watch | ✅ VS Code + JB | ❌ |
| **Desktop App** | ✅ | ✅ | ❌ | ❌ | ❌ |
| **Web Interface** | ✅ | ✅ | ❌ | ❌ | ⚠️ WebUI |
| **CLI** | ✅ | ✅ | ✅ | ✅ | ✅ |
| **SDK** | ✅ | ✅ | ❌ | ✅ | ❌ |
| **MCP Support** | ✅ | ✅ | ❌ | ✅ | ✅ |
| **Verification Gate** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |
| **Arch Debt Detection** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |
| **Intent-Based Diff** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |
| **Proof-of-Correctness** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |
| **Oracle Verification** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |
| **EFM** | ❌ | ❌ | ❌ | ❌ | ✅ Einzigartig |

---

## 7. SOTA Best Practices 2025-2026

### 7.1 Agent Loop Architecture
- **Streaming output** (token-by-token)
- **Automatic retry** mit exponential backoff
- **Automatic context selection** (LLM-basiert)
- **Automatic commit** nach jeder Änderung
- **Checkpoint system** für undo/redo
- **Human-in-the-loop** für destructive operations

### 7.2 Code Editing
- **Diff-based editing** (unified diff format)
- **Multi-file editing** (koordinierte changes)
- **Automatic context selection** (surrounding code)
- **Syntax validation** vor commit
- **Automatic formatting** nach edit

### 7.3 Code Navigation
- **LSP integration** (go-to-definition, find-references)
- **Semantic search** (embedding-basiert)
- **Repo-map** (tree-sitter-basierte code maps)
- **Cross-reference navigation** (call graph)
- **Incremental indexing** (persistent index)

### 7.4 Testing & Quality
- **Automatic test execution** nach edits
- **Automatic lint checking**
- **Automatic error fixing**
- **Coverage tracking**
- **Performance monitoring**

### 7.5 Safety & Security
- **Sandboxing** (Docker/VM für shell commands)
- **Permission system** (allow/deny/ask)
- **Secret redaction** (API keys, tokens)
- **Audit trail** (alle actions loggen)
- **Rollback capability** (checkpoint system)

### 7.6 Integration
- **IDE integration** (VS Code, JetBrains)
- **CI/CD integration** (GitHub Actions, GitLab CI)
- **Messaging integration** (Slack, Telegram)
- **MCP support** (external tools)
- **SDK** (custom agents)

### 7.7 Memory & Learning
- **Project instructions** (CLAUDE.md, AGENTS.md)
- **Auto memory** (automatische learnings)
- **Session history** (resumable sessions)
- **Learning loop** (from failures)
- **Knowledge graph** (codebase understanding)

---

## 8. Fazit

**Die SOTA Coding CLIs haben sich stark konvergiert:**

1. **Diff-based editing** ist Standard (alle außer SIN-Code)
2. **Automatic context selection** ist Standard (alle außer SIN-Code)
3. **Automatic test + lint loop** ist Standard (alle außer SIN-Code)
4. **LSP integration** ist becoming standard (Codex CLI, Claude Code internal)
5. **Semantic search** ist becoming standard (Codex CLI, Claude Code)
6. **Multi-agent coordination** ist emerging standard (Claude Code, Cline)

**SIN-Code ist meilenweit entfernt von SOTA.**

**Einzige Alleinstellungsmerkmale:**
- Verification Gate
- Architectural Debt Watchdogs
- Intent-Based Diffing
- Proof-of-Correctness
- Oracle Verification
- Ephemeral Full-Stack Mocking

**Aber diese USP sind nur nützlich wenn die Basis stimmt:**
- Diff-based editing
- Automatic context selection
- Automatic test + lint loop
- LSP integration
- Semantic search

**Ohne diese Basis-Features sind die USP wertlos.**

---

## 9. Empfehlungen

### 9.1 Kurzfristig (1-2 Monate)
1. **Diff-based editing implementieren** (unified diff format)
2. **Streaming output hinzufügen** (token-by-token)
3. **Automatic test execution** (nach jedem edit)
4. **Automatic lint checking** (nach jedem edit)

### 9.2 Mittelfristig (3-6 Monate)
5. **LSP integration** (go-to-definition, find-references)
6. **Semantic search** (embedding-basiert)
7. **Automatic context selection** (LLM-basiert)
8. **Multi-file editing** (koordinierte changes)

### 9.3 Langfristig (6-12 Monate)
9. **Sub-agents** (parallel task execution)
10. **Checkpoint system** (undo/redo)
11. **IDE integration** (VS Code extension)
12. **CI/CD integration** (GitHub Actions)

### 9.4 USP-Verbesserung
13. **Verification Gate verbessern** (LLM-basiertes verification)
14. **ADW verbessern** (custom rules, trend tracking)
15. **IBD verbessern** (LLM-basiertes intent understanding)
16. **PoC verbessern** (behavioral verification)
17. **Oracle verbessern** (actual test execution)

---

## 10. Quellen

- OpenAI Codex CLI: https://github.com/openai/codex
- Claude Code: https://docs.anthropic.com/en/docs/claude-code
- Aider: https://github.com/Aider-AI/aider
- Cline: https://github.com/cline/cline
- Roo Code: https://github.com/RooCodeInc/Roo-Code (archived)
- ZooCode: https://github.com/Zoo-Code-Org/Zoo-Code/
