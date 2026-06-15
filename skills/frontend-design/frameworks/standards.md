# Frameworks: Standards & Constraints

Docs: ../SKILL.md

## Technology Stack

- SOTA design system tokens (typography, color, spacing, motion).
- Component generators for button, input, card, modal.
- Page scaffolds for landing, pricing, docs, blog.
- WCAG 2.2 AA checker.
- v0-pool at `http://localhost:27401` (optional fallback).

## Standards

- Use 4px grid spacing.
- Use token ramps (50–900).
- Provide all states: default, hover, focus, active, disabled.
- WCAG 2.2 AA compliance minimum.

## Constraints

- No arbitrary values.
- No decoration without function.
- v0-pool fallback to templates if offline.

## Quality Gates

- Design system loaded.
- Tokens applied consistently.
- A11y check passed.
- States defined.
