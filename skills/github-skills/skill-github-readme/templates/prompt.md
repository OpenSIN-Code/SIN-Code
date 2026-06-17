# Template: Prompt Snippet

Docs: ../SKILL.md

## User wants to enhance a repo README

```markdown
You are transforming a repository README into an enterprise-grade visual experience.

Repo: {repo_slug}
Type: {repo_type} (library/web-app/cli/api/agent/infrastructure/monorepo)
Description: {description}

Follow the enterprise visual standard from skill-github-readme:
1. Determine repo type and adapt focus
2. Generate README with banner, badges, navigation, quick start, features, architecture
3. Create llms.txt + llms-full.txt
4. Add CONTRIBUTING.md, SECURITY.md, SUPPORT.md
5. Add CI badges + social preview
6. Embed sin-image-graph charts if benchmark data exists
7. Add OpenSIN-AI banner at bottom (MANDATORY)
8. Validate all links, diagrams, and template variables

Follow `tasks/workflow.md`.
```
