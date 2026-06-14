# CodeGraph Integration (issue #126)

`sin-code codegraph` bridges [CodeGraph](https://github.com/codegraph-ai/codegraph),
an external multi-language static-analysis engine that builds a graph of
symbols (functions, types, modules) and their relationships (calls, imports,
implements, references). It gives the agent code-aware navigation across
languages without SIN-Code having to ship per-language parsers.

## Design: Bridged-External-Contract (Option A)

CodeGraph is **never vendored**. SIN-Code shells out to the user's installed
`codegraph` binary, exactly like the `gh`, `vane`, `dox`, and `rtk` bridges:

- Discovery order: `PATH`, then `/usr/local/bin`, `/opt/homebrew/bin`,
  `~/.cargo/bin`, `~/.local/bin`.
- When the binary is missing, every command fails with a clear install hint
  (`ErrNotInstalled`) instead of a cryptic exec error.
- The agent can always fall back to its built-in `grasp`/`map`/`index`
  commands when CodeGraph is unavailable.

## Installation

```sh
# install script
curl -fsSL https://raw.githubusercontent.com/codegraph-ai/codegraph/main/install.sh | sh
# or via cargo
cargo install codegraph
```

Verify:

```sh
sin-code codegraph doctor
# codegraph: OK
#   path:    /usr/local/bin/codegraph
#   version: codegraph 0.x.y
```

## Usage

```sh
# human-readable summary
sin-code codegraph analyze .
# CodeGraph: /path/to/repo
#   nodes: 1423
#   edges: 5210

# raw JSON graph (for MCP / downstream tooling)
sin-code codegraph analyze --json . > graph.json
```

`analyze` runs `codegraph analyze --json .` under the hood and decodes the
envelope into a typed `Graph{ Root, Nodes[], Edges[] }`.

## MCP exposure

The typed `Graph` is the contract used to surface CodeGraph as an MCP tool:
the JSON envelope from `analyze --json` is forwarded verbatim, so an MCP
client receives a stable `{root, nodes, edges}` shape. Because parsing
(`ParseGraph`) is separated from execution (`Run`/`Analyze`), the envelope
can be validated and unit-tested without the external binary present.

## Graph schema

| Field        | Meaning                                              |
|--------------|------------------------------------------------------|
| `node.id`    | stable unique identifier                             |
| `node.kind`  | `function` \| `type` \| `method` \| `module` \| ...  |
| `node.name`  | symbol name                                          |
| `node.file`  | source file (relative to root)                       |
| `node.line`  | 1-based declaration line                             |
| `node.lang`  | source language                                      |
| `edge.from`  | source node id                                       |
| `edge.to`    | target node id                                       |
| `edge.kind`  | `calls` \| `imports` \| `implements` \| `references` |
