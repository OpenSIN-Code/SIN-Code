# Fireworks provider gap analysis

## Current SIN-Code strengths

- Native Go profile and setup-wizard support.
- Curated multi-model Fireworks tournament pool.
- Explicit cost, context, vision and thinking metadata.
- Permission-gated tool surface.
- Optional SINator pool-router support.

## Retained OpenCodex ideas not fully represented

- Runtime discovery through `GET /v1/models`.
- ETag-aware remote catalog refresh.
- Local model-catalog cache.
- Five-second bounded refresh timeout.
- Merge/deduplication of returned models and known router fallbacks.
- Unit tests for alternate model response formats.

## Recommended implementation boundary

Do not port the Rust code directly. Implement a small Go catalog component
behind the existing Fireworks profile/fusion layer:

- static curated lineup remains the safe fallback;
- dynamic discovery is optional and never blocks startup;
- cached data has an explicit age and provenance;
- capability overrides remain version-controlled;
- no API key, account data or live response is committed;
- tests use local fixtures only.
