# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- GitHub-flavored Markdown
- Mermaid diagrams (flowchart, not graph)
- Shields.io badges
- llms.txt / llms-full.txt standard
- sin-image-graph (optional, for benchmark charts)

## Standards

- README: 300-800 lines
- Max 5-7 badges in a row
- Max 1 Mermaid diagram in README (rest in docs/)
- All images must have alt-text (accessibility)
- All template variables ({REPO_NAME}, {REPO_SLUG}, etc.) must be replaced before commit
- Headings must NOT contain emojis (anchor link compatibility)

## Constraints

- No `<iframe>` or `<video>` tags (GitHub blocks them)
- No external image hosts (except shields.io)
- Use `flowchart` instead of `graph` in Mermaid
- No reserved keywords as classDef names
- OpenSIN-AI banner is MANDATORY at the bottom of every repo README
