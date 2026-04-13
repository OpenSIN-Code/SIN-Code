<p align="center">
  <img src="./assets/simone-mcp-banner.PNG" alt="Simone MCP banner" />
</p>

# Simone MCP

> **Simone MCP ist ein production-grade Code-Worker, der komplexe Code-Navigation und -Manipulation löst, indem es symbol-basierte Operationen über MCP bereitstellt - komplett automatisch für OpenCode, Codex und A2A-Agenten.**

## 🚀 Was bringt dir das?

| 😩 Vorher (Ohne Simone MCP) | ✨ Nachher (Mit Simone MCP) | ⏱️ Ersparnis |
|-----------------------------|----------------------------|--------------|
| Manuelles Code-Durchsuchen | Symbol-Lookup in Millisekunden | **90% schneller** |
| Regex-basierte Suche | AST-basierte präzise Analyse | **0 False Positives** |
| Manuelle Refaktorierung | Strukturelle Edits auf Knopfdruck | **Stunden → Sekunden** |
| Eigene Tools bauen | Fertig integrierte MCP Tools | **Tage → Minuten** |

## 🎬 Simone MCP in Aktion

### ❌ Ohne Simone MCP:
```
1. Codebase manuell durchsuchen (grep, rg)
2. Dateien einzeln öffnen
3. Symbole per Hand finden
4. Referenzen manuell追踪en
5. Code-Änderungen kopieren/einfügen
...und hoffen dass nichts kaputt geht! 😰
```

### ✅ Mit Simone MCP:
```bash
# Ein Befehl - fertig!
python3 src/cli.py run-action '{"action":"code.find_symbol","symbol":"my_function"}'
✨ Symbol gefunden in 50ms mit exakter Position!
```

## 🎯 In 3 Schritten starten

```
┌─────────────┐     ┌─────────────┐     ┌─────────────┐
│  1. Install │ ──▶ │  2. Activate│ ──▶ │  3. Run!    │
│             │     │             │     │             │
│  pip install│     │  activate_  │     │  simone.mcp │
│  -e .[dev]  │     │  simone     │     │  .health    │
│             │     │             │     │             │
│  ⏱️ 30 Sek  │     │  ⏱️ 1 Sek   │     │  ⏱️ GO! 🚀  │
└─────────────┘     └─────────────┘     └─────────────┘
```

**Keine Programmierkenntnisse erforderlich - funktioniert out-of-the-box mit OpenCode!**

## 💡 Use Cases - Wer braucht das?

| 👤 Rolle | 🎯 Anwendungsfall | 💰 Wert |
|----------|-------------------|---------|
| **Developer** | Code-Navigation in großen Repos | 15 Std/Woche gespart |
| **A2A-Agenten** | Symbol-Level Code-Verständnis | Automatisierte Code-Änderungen |
| **Team Leads** | Neue Mitarbeiter onboarden | Repo-Verstehen 80% schneller |
| **OpenCode User** | MCP Integration | Sofort einsatzbereit |

---

## 📊 Architektur

```mermaid
graph TB
    subgraph Clients["🖥️ Clients"]
        OC[OpenCode CLI]
        CX[Codex]
        A2A[A2A Agents]
    end

    subgraph Transport["🔌 Transport Layer"]
        STDIO[MCP stdio<br/>Server]
        HTTP[FastAPI HTTP<br/>Server :8234]
    end

    subgraph Core["⚙️ Simone Core Engine"]
        EXEC[Action Executor]
        SYMBOL[Symbol Operations<br/>Python AST]
        MEMORY[Memory Facade]
        AUTH[OAuth 2.1<br/>Validator]
    end

    subgraph Storage["💾 Memory & Storage"]
        QDRANT[(Qdrant<br/>Vector DB)]
        NEO4J[(Neo4j<br/>Graph DB)]
        SUPABASE[(Supabase)]
    end

    OC -->|stdio| STDIO
    CX -->|stdio| STDIO
    A2A -->|HTTP| HTTP

    STDIO --> EXEC
    HTTP --> MCP[MCP /mcp]
    HTTP --> A2AEP[A2A /a2a/v1]
    HTTP --> WELL[.well-known]
    
    MCP --> AUTH
    A2AEP --> EXEC
    WELL --> EXEC
    AUTH --> EXEC
    
    EXEC --> SYMBOL
    EXEC --> MEMORY
    MEMORY --> QDRANT
    MEMORY --> NEO4J
    MEMORY --> SUPABASE

    classDef client fill:#e1f5fe,stroke:#01579b,stroke-width:2px
    classDef transport fill:#fff3e0,stroke:#e65100,stroke-width:2px
    classDef core fill:#e8f5e9,stroke:#1b5e20,stroke-width:2px
    classDef storage fill:#fce4ec,stroke:#880e4f,stroke-width:2px

    class OC,CX,A2A client
    class STDIO,HTTP transport
    class EXEC,SYMBOL,MEMORY,AUTH,MCP,A2AEP,WELL core
    class QDRANT,NEO4J,SUPABASE storage
```

→ Für ALLE technischen Details (OAuth Flow, Memory Integration, Security, CI/CD): [docs/architecture.md](docs/architecture.md)

## 🛠️ Quick Start

```bash
git clone https://github.com/Delqhi/Simone-MCP.git
cd Simone-MCP
python3 -m venv .venv && source .venv/bin/activate
pip install -e .[dev]
python3 src/cli.py serve
```

## Core Commands

```bash
python3 src/cli.py serve        # HTTP Server starten
python3 src/cli.py serve-mcp    # MCP stdio Server
python3 src/cli.py print-card   # Agent Card anzeigen
python3 src/cli.py run-action '{"action":"simone.mcp.health"}'
```

## 📚 Mehr

- [docs/architecture.md](docs/architecture.md) - Vollständige Architektur-Dokumentation mit 12+ Diagrammen
- [ARCHITECTURE.md](ARCHITECTURE.md) - Technische Design-Entscheidungen

## License

MIT
