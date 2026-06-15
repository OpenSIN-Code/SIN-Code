---
name: skill-code-add-endpoint
description: Add an API endpoint with an ephemeral mock and verification. Use when the user asks to add a new endpoint, route, or API method.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# skill-code-add-endpoint

## Overview

Add a new API endpoint, route, or method. Use an ephemeral mock server to verify the contract before touching production code.

## When to Use

- User asks to add a new endpoint, route, or API method.
- A new integration point needs to be defined and tested.

## When NOT to Use

- The change is purely internal (no API surface).
- The user wants to refactor existing endpoints (use `skill-code-refactor`).

## Core Process

```
DESCRIBE CONTRACT → MOCK → VERIFY → IMPLEMENT → TEST → DOCUMENT
```

1. Capture the HTTP method, path, request shape, and response shape.
2. Spin up an ephemeral mock server (e.g., EFM) to test the contract.
3. Verify the mock with a sample request/response.
4. Implement the endpoint in the real codebase.
5. Write tests covering success and failure cases.
6. Update API docs / OpenAPI spec.

## Common Rationalizations

| Rationalization | Reality |
|---|---|
| "I'll implement directly." | Mocking first catches contract mismatches early. |
| "Tests are enough." | Tests need a defined contract first. |
| "Docs can wait." | API docs should be updated in the same PR. |

## Red Flags

- No request/response contract defined.
- Skipping the mock step.
- Missing failure-case tests.
- Not updating API docs.

## Verification

- [ ] Contract defined and approved.
- [ ] Mock verified with sample request/response.
- [ ] Real implementation matches the contract.
- [ ] Tests cover success and failure paths.
- [ ] API docs updated.
