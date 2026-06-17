---
name: skill-infrastructure-cloudflare
description: Cloudflare skill — Workers, Pages, Workers AI, R2, KV, D1, Cache, Tunnels, DNS, WAF.
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
  - sin_harvest
lifecycle: external
sources:
  - 
---

# skill-infrastructure-cloudflare

## Overview

Cloudflare skill — Workers, Pages, Workers AI, R2, KV, D1, Cache, Tunnels, DNS, WAF.

## When to Use

- The user asks about skill-infrastructure-cloudflare infrastructure operations.
- A project needs to provision, query, or manage skill-infrastructure-cloudflare resources.

## When NOT to Use

- The project does not use skill-infrastructure-cloudflare.
- A native `sin-code skill-infrastructure-cloudflare` command is available (future).

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
