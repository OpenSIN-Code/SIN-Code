# Provenance

The executable CEO Audit engine was migrated from
`OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/ceo-audit` into the canonical
SIN-Code repository on 2026-07-27.

- Source repository: `OpenSIN-Code/Infra-SIN-OpenCode-Stack`
- Source commit: `891867ac6f9bb997caed56a41e353f40d0b89b14`
- Destination: `skills/code-skills/skill-code-ceo-audit`
- License: MIT

The migration intentionally excluded the obsolete backup file
`templates/ceo-audit.yml.bak-20260604` and replaced a high-entropy synthetic
secret fixture with the explicit value `synthetic-test-key`.

Infra remains a private migration source; SIN-Code is the source of truth for
all future CEO Audit implementation and CI changes.
