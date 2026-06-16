---
name: skill-memory-infisical
description: Centralized secret management via Infisical CLI. Stores API keys, tokens, and credentials without .env files or shell history.
license: MIT
compatibility: 
metadata: 
lifecycle: external
---

# skill-memory-infisical

## Overview

Manage secrets via Infisical. Store, retrieve, and rotate API keys and credentials without putting them in repos or shell history.

## When to Use

- User says "secret", "api key", "api-key", "password", "credentials", "infisical", "store secret", "retrieve secret", "rotate token", "secrets manager", "env var", "set api key", "list keys", "get key", "token storage".

## When NOT to Use

- The project uses a different secret manager.
- The user wants secrets in plain text.

## Core Process

```
IDENTIFY SECRET → ENCRYPT → STORE → RETRIEVE/ROTATE
```

1. Identify the secret and its purpose.
2. Store it in Infisical under the correct project and environment.
3. Retrieve it when needed (never log the value).
4. Rotate if exposed or on schedule.

## Security Rules

- No .env files in repos.
- No tokens in shell history.
- No values in CI logs.
- Rotate any exposed token immediately.

## Verification

- [ ] Secret stored in Infisical.
- [ ] Value never logged in plain text.
- [ ] Rotation performed if exposed.
- [ ] Degraded mode handled gracefully.
