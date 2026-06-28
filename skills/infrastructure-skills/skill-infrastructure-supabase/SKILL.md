---
name: skill-infrastructure-supabase
description: Use when user says 'supabase', 'SQL migrations', 'RLS policies', 'edge functions', 'self-hosted supabase'. Supabase self-hosted skill — SQL migrations, RLS policies, Auth, Storage, Realtime, Edge Functions, Triggers, Backups.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 1.0.0
required_tools:
  - sin_execute
  - sin_write
lifecycle: external
sources:
  - "OpenSIN-Code/Infra-SIN-OpenCode-Stack"
---

# skill-infrastructure-supabase

## Overview

Supabase self-hosted skill — SQL migrations, RLS policies, Auth, Storage, Realtime, Edge Functions, Triggers, Backups.

## When to Use

- The user asks about skill-infrastructure-supabase infrastructure operations.
- A project needs to provision, query, or manage skill-infrastructure-supabase resources.

## When NOT to Use

- The project does not use skill-infrastructure-supabase.
- A native `sin-code skill-infrastructure-supabase` command is available (future).

## Core Process

```
IDENTIFY -> AUTHENTICATE -> OPERATE -> VERIFY
```

1. Identify the requested infrastructure operation.
2. Ensure the correct credentials and environment are configured.
3. Perform the operation via the external canonical skill or CLI.
4. Verify the result and surface errors.

## Verification

- [ ] Credentials are configured and not exposed in logs.
- [ ] The operation target matches the project/environment.
- [ ] Changes are idempotent where possible.
- [ ] Output is validated against the expected schema.
