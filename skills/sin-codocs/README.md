# sin-codocs skill

The canonical CoDocs agent skill ships **inside the package** at
[`src/sin_code_bundle/data/codocs/SKILL.md`](../../src/sin_code_bundle/data/codocs/SKILL.md)
so it is available from both editable and wheel installs.

## Install into your agent

```bash
sin codocs install-skill                 # Hermes + OpenCode
sin codocs install-skill --agent hermes  # Hermes only
sin codocs install-skill --agent opencode
```

This copies `SKILL.md` to:

| Agent    | Path |
|----------|------|
| Hermes   | `~/.hermes/skills/sin-codocs/SKILL.md` |
| OpenCode | `~/.config/opencode/skills/sin-codocs/SKILL.md` |

See [docs/CODOCS.md](../../docs/CODOCS.md) for the full standard.
