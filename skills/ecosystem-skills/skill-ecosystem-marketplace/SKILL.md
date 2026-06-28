---
name: skill-ecosystem-marketplace
description: Use when user says 'marketplace', 'install skill', 'search skills', 'skill catalog', 'remove skill'. Manage the SIN-Code skill marketplace. Search, install, update, and remove skills from the catalog.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/Infra-SIN-OpenCode-Stack/skills/marketplace"
required_tools:
  - sin_execute
lifecycle: external
---

# skill-ecosystem-marketplace

## Overview

Browse and manage the SIN-Code skill catalog. Search, install, update, and remove skills.

## When to Use

- User wants to install a new skill, update existing skills, or browse the catalog.

## When NOT to Use

- The user is asking about application code, not skills.

## Core Process

```
SYNC → SEARCH → INSTALL → UPDATE/REMOVE
```

1. Sync the catalog with the remote index.
2. Search or list skills.
3. Install selected skills.
4. Update or remove as needed.

## Verification

- [ ] Catalog is synced.
- [ ] Skill installed successfully.
- [ ] Dependency check passed.
- [ ] Removal confirmed with user.
