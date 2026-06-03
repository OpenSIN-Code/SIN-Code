# skills.py

Compiles portable SIN skills (a single source of truth: `skills/*.md`
with YAML frontmatter + prompt body) into each agent's native command /
skill format. One source → three targets.

## Dependencies

- stdlib: `re`, `dataclasses`
- optional: `pyyaml` (required to parse skill frontmatter)

## Touched by

- `cli.py` — `sin skills` command
- `hooks.py` — separate auto-installer for `.opencode/hooks/`
- The `skills/` directory in a target repo (the input)

## What it does

1. **`Skill.parse(path)`** — reads a `*.md` file, splits YAML frontmatter
   from body, returns a `Skill` dataclass.
2. **`render_skill(skill, target)`** — returns `(relative_path, content)`
   for the target agent:
   - `opencode` → `.opencode/command/<name>.md` (with `description` /
     `agent` frontmatter)
   - `codex` → `~/.codex/prompts/<name>.md` (plain prompt;
     `{{arg}}` → `$N` positional)
   - `claude` → `.claude/skills/<name>/SKILL.md` (with `name` /
     `description` frontmatter)
3. **`load_skills(source_dir)`** — globs `*.md` under `source_dir` and
   parses them.
4. **`compile_skills(target, source_dir, out_root, dry_run)`** — the
   main entry point. For `codex`, writes under `~/.codex/`; for the
   others, under `out_root` (usually the repo).

## Important config

- `Target` literal — `"opencode" | "codex" | "claude"`
- `SUPPORTED_TARGETS = ("opencode", "codex", "claude")`
- `_FRONTMATTER_RE` — splits `^---\n…\n---\n…$`

## Usage

```python
from pathlib import Path
from sin_code_bundle.skills import compile_skills

written = compile_skills("opencode", source_dir=Path("skills"),
                         out_root=Path("."), dry_run=False)
for p in written:
    print(p)
```

## Known caveats

- `codex` writes go directly into `~/.codex/prompts/`, NOT under
  `out_root` — there's no way to redirect them to a project-local
  location.
- `compile_skills` does **not** delete obsolete skills; removing a
  source file leaves the rendered file in place. Clean manually.
- The `claude` target produces a *single-file* SKILL.md per skill, not
  the multi-file `SKILL.md` + `references/` layout that Claude Code
  supports; full Claude-Code packaging is not in scope.
