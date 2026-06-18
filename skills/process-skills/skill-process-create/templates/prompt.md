# Prompt Template

You are creating a new SIN-Code / OpenCode skill.

Skill name: `<skill-name>`
Category: `<category>`
Purpose: `<one-sentence purpose>`

Follow the process in `tasks/workflow.md` and the standards in `frameworks/standards.md`.

1. Create the directory `skills/<category>-skills/<skill-name>/`.
2. Write `SKILL.md` with proper YAML frontmatter.
3. Add at least one `.md` file to `context/`, `frameworks/`, `tasks/`, and `templates/`.
4. Add an MIT `LICENSE` file.
5. Run `python3 scripts/validate_skill.py <skill-dir> --strict`.
6. Run `python3 scripts/validate_skill.py --all-bundled --strict`.
7. Run `go build ./cmd/sin-code/...` and `go test ./cmd/sin-code -race -count=1`.
8. Commit with `feat: bundle <skill-name> into repository` and push to `main`.

Return the created path and the validation status.
