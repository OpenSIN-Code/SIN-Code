# rtk Integration — Token-Sparen für SIN-Code

[rtk (Rust Token Killer)](https://github.com/rtk-ai/rtk) reduziert den
Token-Verbrauch von LLM-Coding-Agenten um **60-90%**, indem es CLI-Ausgaben
intelligent filtert (Smart Filtering, Grouping, Truncation, Deduplication).

## Warum rtk + SIN-Code?

- SIN-Code orchestriert LLM-Arbeit, **rtk senkt die Kosten** jeder
  LLM-Interaktion.
- Kein Overhead: rtk ist ein eigenständiges 5-10 MB Rust-Binary ohne
  Abhängigkeiten. Es wird **nie** in SIN-Code vendored — wir rufen das
  vom Nutzer installierte Binary auf (Bridged-External-Contract, wie bei
  `gh`, `vane`, `dox`).

## Installation von rtk

```bash
# macOS / Linux
curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/main/install.sh | sh

# oder Homebrew
brew install rtk

# Überprüfen
rtk --version
```

Danach prüft SIN-Code die Installation:

```bash
sin-code rtk doctor
# rtk: OK
#   path:    /usr/local/bin/rtk
#   version: rtk x.y.z
```

## Nutzung in SIN-Code

### 1. Direkter CLI-Subcommand

`--` trennt das an rtk weitergereichte Kommando von SIN-Codes eigenen Flags:

```bash
sin-code rtk run -- git status
sin-code rtk run -- go test ./...
sin-code rtk run -C /pfad/zum/projekt -- cargo check
```

Flags:

- `-C, --dir <pfad>`  Arbeitsverzeichnis (Standard: aktuelles Verzeichnis)
- `--timeout <dauer>` Maximale Laufzeit (Standard: `60s`, `0` = kein Limit)

### 2. Im Chat (interaktiv)

```text
sin-code chat
> Bitte führe `git status` mit rtk aus, um Tokens zu sparen.
```

## Verhalten bei fehlender Installation

Ist rtk nicht installiert, schlägt `sin-code rtk run` mit einer klaren
Fehlermeldung samt Installationsanweisung fehl. Der Agent kann jederzeit
auf das rohe Kommando zurückfallen (z. B. `git status` direkt ausführen).

## Benchmark (Beispiel)

| Befehl         | Roh-Tokens | Mit rtk | Einsparung |
| -------------- | ---------- | ------- | ---------- |
| `git status`   | 1.850      | 210     | 88,6 %     |
| `go test -v`   | 8.430      | 1.120   | 86,7 %     |
| `cargo check`  | 4.200      | 480     | 88,6 %     |

## Architektur

```text
+----------------+     +-------------------+     +--------------+
| SIN-Code (Go)  | --> | internal/rtk      | --> | rtk binary   |
| sin-code rtk   |     | Bridge.Find/Run   |     | (Rust)       |
+----------------+     +-------------------+     +--------------+
                                                        |
                                                        v
                                                +----------------+
                                                | Gefilterter    |
                                                | Output         |
                                                +----------------+
```

Implementierung: `cmd/sin-code/internal/rtk/rtk.go` (Binary-Discovery +
Ausführung) und `cmd/sin-code/rtk_cmd.go` (CLI). Beide vendoren rtk nicht.
