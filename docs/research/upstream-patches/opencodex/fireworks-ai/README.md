# OpenCodex Fireworks AI provider research

**Status:** preserved research; not compiled into SIN-Code.

This directory retains the only product-specific commit found in
`Delqhi/OpenCodex` before that fork was retired. The fork's `main` branch was
byte-for-byte aligned with its upstream at the time of the audit; the unique
work lived on `feat/fireworks-ai-provider`.

SIN-Code already has a newer native Fireworks implementation. Therefore this
Rust provider is preserved as an attributable reference patch rather than
introduced as a second runtime implementation.

## Ideas worth porting

1. Refresh the Fireworks `/v1/models` catalog dynamically.
2. Cache the model catalog and apply a bounded network timeout.
3. Merge API results with router/model fallbacks that are absent from the API.
4. Normalize model capabilities such as context, vision and reasoning.
5. Add parser, deduplication, fallback and provider-construction tests.

See `GAP-ANALYSIS.md` before porting. The exact original commit is retained in
`fireworks-ai-provider.patch`; the complete deleted repository is also kept as
a verified local Git bundle outside this repository.
