---
name: skill-infrastructure-oci-vm
description: OCI VM inventory, access, and management skill — Frankfurt Always Free Tier.
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
lifecycle: external
sources:
  - 
---

# skill-infrastructure-oci-vm

## Overview

OCI VM inventory, access, and management skill — Frankfurt Always Free Tier.

## When to Use

- The user asks about skill-infrastructure-oci-vm infrastructure operations.
- A project needs to provision, query, or manage skill-infrastructure-oci-vm resources.

## When NOT to Use

- The project does not use skill-infrastructure-oci-vm.
- A native `sin-code skill-infrastructure-oci-vm` command is available (future).

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
