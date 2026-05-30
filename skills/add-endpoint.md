---
name: add-endpoint
description: Add an API endpoint with an ephemeral mock and verification.
arguments:
  - name: spec
    description: One-line description of the endpoint (method, path, behavior)
    required: true
---

Add the endpoint described as: {{spec}}.

1. Call `mock_env("up")` to get an ephemeral full-stack environment.
2. Implement the endpoint with input validation and error handling.
3. Call `semantic_review(before, after)` on each changed file; justify any
   non-"low" risk.
4. Write tests covering success + failure paths.
5. Call `verify_tests(...)`; iterate until the verdict is `pass`.
6. Call `mock_env("down")` to tear down the environment.

Do not report done while verification is red or the mock is still running.
