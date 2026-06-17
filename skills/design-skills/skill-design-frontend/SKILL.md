---
name: skill-design-frontend
description: SOTA frontend design system and philosophy. Loads typography, color, spacing, motion tokens; generates button/input/card/modal specs; scaffolds pages; runs WCAG 2.2 AA checks. v0-pool integration when available.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
lifecycle: external
---

# skill-design-frontend

## Overview

Apply a SOTA design system for frontend work: tokens, component specs, page scaffolding, accessibility checks, and v0-pool code generation.

## When to Use

- Generate component specs (button, input, card, modal).
- Scaffold a full page (landing, pricing, docs, blog).
- Review existing UI for design-system consistency.
- Extract design tokens from CSS/SCSS/Tailwind/JSON/Figma.
- Run WCAG 2.2 AA accessibility checks.
- Generate responsive breakpoints.
- Export tokens to Figma Tokens JSON.

## When NOT to Use

- Backend or API design (use `skill-code-spec` / `skill-code-plan`).
- Image generation (use `skill-design-image`).
- Code logic without UI (use `skill-code-build`).

## Core Process

```
LOAD DESIGN SYSTEM → CREATE COMPONENT / SCAFFOLD PAGE → REVIEW → CHECK A11Y → EXPORT TOKENS
```

1. Load the design system tokens and philosophy.
2. Create the component or scaffold the page.
3. Review for consistency.
4. Check accessibility (WCAG 2.2 AA).
5. Export or extract tokens as needed.

## Design Philosophy

1. Hierarchy is created by contrast, not decoration.
2. Type is the primary voice — choose one family and use scale.
3. Color is functional: primary, secondary, success, warning, error, neutral.
4. Spacing follows a 4px grid.
5. Motion is felt, not seen: 200ms hovers, 300ms transitions.
6. Components are predictable: same name, same shape, same tokens.
7. States are explicit: default, hover, focus, active, disabled.
8. Accessibility is non-negotiable: WCAG 2.2 AA is the floor.
9. Dark mode is a parallel semantic map, not an inversion.
10. Restraint creates calm.

## Token Reference

### Typography (px)
`12 · 14 · 16 · 18 · 20 · 24 · 30 · 36 · 48 · 60 · 72`

### Spacing (px, 4px grid)
`4 · 8 · 12 · 16 · 24 · 32 · 48 · 64 · 96`

### Motion
- Hover: 200ms ease-out
- Transition: 300ms ease-in-out
- Page: 500ms cubic-bezier(0.16, 1, 0.3, 1)

### Radius
- Default: 8px
- Card: 16px

### Color ramps (50–900)
- `neutral` — slate
- `primary` — indigo
- `secondary` — violet
- `success` — green
- `warning` — amber
- `error` — red

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I can use arbitrary values." | Follow the 4px grid and token ramps. |
| "Accessibility is extra." | WCAG 2.2 AA is the floor, not optional. |
| "v0-pool is required." | Falls back to built-in templates if offline. |

## Red Flags

- Arbitrary spacing or colors.
- Missing focus/hover/disabled states.
- Skipping a11y check.
- Inconsistent component naming.

## Verification

- [ ] Design system loaded.
- [ ] Component or page created with tokens.
- [ ] Review passed for consistency.
- [ ] WCAG 2.2 AA check passed.
- [ ] Tokens exported or extracted if requested.
