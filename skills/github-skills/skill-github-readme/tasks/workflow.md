# Tasks: Workflow

Docs: ../SKILL.md

## Pre-flight

- [ ] Determine repo type (library, web-app, cli, api, agent, infrastructure, monorepo)
- [ ] Collect repo metadata (name, slug, description, tagline)
- [ ] Check for existing README and assets/

## Execution

- [ ] Task 1: Generate README.md with enterprise visual standard
  - Acceptance: Banner, badges, navigation, quick start, features, architecture, use cases
  - Verify: README renders on GitHub without broken images or Mermaid errors
- [ ] Task 2: Create llms.txt and llms-full.txt
  - Acceptance: Both files in repo root with absolute URLs
  - Verify: Files parse as valid Markdown
- [ ] Task 3: Add CONTRIBUTING.md, SECURITY.md, SUPPORT.md
  - Acceptance: All three files present and customized
  - Verify: Files exist and reference correct repo URLs
- [ ] Task 4: Add CI badge + workflow if missing
  - Acceptance: CI badge in README shows real status
  - Verify: Badge URL resolves
- [ ] Task 5: Optimize GitHub About Section
  - Acceptance: Description, website, and 5-10 topics set
  - Verify: `gh repo view` shows all fields
- [ ] Task 6: Generate social preview image
  - Acceptance: 1280x640 PNG uploaded to GitHub Settings
  - Verify: Social preview visible in repo settings
- [ ] Task 7: Embed sin-image-graph charts (if benchmark data available)
  - Acceptance: Charts generated and embedded in README
  - Verify: `sin-code image-graph` output referenced in README

## Post-flight

- [ ] Validate all anchor links work
- [ ] Validate all Mermaid diagrams render
- [ ] Validate all template variables are replaced
- [ ] Validate OpenSIN-AI banner is present at bottom
