# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Detect modality from extension + MIME + magic bytes.
- [ ] Pick exactly one `analyse__*` tool.
- [ ] Tool is on the `allow` list (always true for `analyse__*`).

## Execution

- [ ] Call the chosen `analyse__*` tool with the file path.
- [ ] Read the structured output carefully (counts, tables, segments).
- [ ] Quote the relevant portion verbatim in the answer.
- [ ] Cite the tool call so the user can reproduce.

## Verification

- [ ] Original file is unchanged (stat mtime + size match).
- [ ] Quote matches the tool output exactly.
- [ ] No `read` / `cat` / `od` ever invoked on the file.
- [ ] No naive LLM guessing about image/PDF/audio content.
