# External Upstream Tools

The bundle does not reinvent solved problems. Where a best-in-class open-source
tool already exists, we **bridge** to it: the upstream project stays the source
of truth (installed and updated independently), and SIN-Code only wires it into
every coder agent so no agent operates without it. No upstream source is
vendored or copied, which keeps the bundle MIT-licensed.

Three external tools are bridged today:

| Tool | Purpose | Mechanism | License |
| --- | --- | --- | --- |
| [GitNexus](https://github.com/abhigyanpatwari/GitNexus) | Code knowledge graph / impact context | MCP server (`npx gitnexus mcp`) | PolyForm Noncommercial |
| [MarkItDown](https://github.com/microsoft/markitdown) | Convert PDF / Office / images to Markdown | MCP server (`uvx markitdown-mcp`) | MIT |
| [RTK](https://github.com/rtk-ai/rtk) | Compress command output, 60-90% fewer tokens | Per-agent hook via `rtk init` | Apache-2.0 |

All three target the same coder agents: **OpenCode**, **Codex**, **Hermes**.

---

## MarkItDown — document context

MarkItDown turns binary/office documents into LLM-friendly Markdown, so agents
can reason over PDFs, specs, spreadsheets, and screenshots instead of being
limited to plain-text source.

### Install (upstream)

```bash
pip install "markitdown[all]"      # CLI + library
pip install markitdown-mcp         # MCP server  (or: uv tool install markitdown-mcp)
```

### Wire into agents

```bash
sin markitdown setup                       # all three agents
sin markitdown setup --agents opencode     # a subset
sin markitdown doctor                      # availability check
```

This writes a `markitdown` MCP server entry into each agent's own config,
preferring upstream's recommended `uvx markitdown-mcp` invocation:

- **OpenCode** → `~/.config/opencode/opencode.json` (`mcp.markitdown`)
- **Codex** → `~/.codex/config.toml` (`[mcp_servers.markitdown]`)
- **Hermes** → `~/.hermes/mcp.json` (`mcpServers.markitdown`)

Existing entries are preserved; the operation is idempotent.

### Use directly

```bash
sin markitdown convert report.pdf > report.md
```

`sin serve` also exposes a `markitdown_convert(path)` tool through the unified
MCP endpoint.

---

## RTK — token-saving command proxy

RTK is a single Rust binary that filters and compresses the output of common dev
commands (`ls`, `grep`, `git`, `pytest`, `cargo test`, ...) before it reaches the
model — typically cutting 60-90% of the tokens those commands would otherwise
consume. It is **not** an MCP server; it installs an interception hook/plugin
per agent.

### Install (upstream)

```bash
brew install rtk
# or: curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh
# or: cargo install --git https://github.com/rtk-ai/rtk
```

### Wire into agents

```bash
sin rtk setup                      # runs `rtk init` for all three agents
sin rtk setup --agents hermes      # a subset
sin rtk doctor                     # is the binary installed?
sin rtk gain                       # token-savings stats (JSON)
```

`sin rtk setup` drives the upstream installer for each agent:

| Agent | Command run |
| --- | --- |
| OpenCode | `rtk init -g --opencode` |
| Codex | `rtk init -g --codex` |
| Hermes | `rtk init --agent hermes` |

After setup, **restart the agent** so the hook/plugin loads.

---

## Graceful degradation

Every bridge checks for its upstream tool at runtime. If the tool is missing,
the command exits with a clear install hint instead of crashing, and `sin status`
reports each external tool's availability:

```bash
sin status
# {
#   ...
#   "GitNexus (graph context, external)": true,
#   "MarkItDown (doc->markdown, external)": false,
#   "RTK (token-saving proxy, external)": true
# }
```

## Recommended one-time setup per machine

```bash
sin gitnexus setup
sin markitdown setup
sin rtk setup
# then, per repository, before an agent task:
sin preflight
```
