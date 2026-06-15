---
name: supabase
description: Supabase self-hosted skill — SQL migrations, RLS policies, Auth, Storage, Realtime, Edge Functions, Triggers, Backups.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
lifecycle: external
sources:
  - 
---

# supabase

## Overview

Supabase self-hosted skill — SQL migrations, RLS policies, Auth, Storage, Realtime, Edge Functions, Triggers, Backups.

## When to Use

- The user asks about supabase infrastructure operations.
- A project needs to provision, query, or manage supabase resources.

## When NOT to Use

- The project does not use supabase.
- A native `sin-code supabase` command is available (future).

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
