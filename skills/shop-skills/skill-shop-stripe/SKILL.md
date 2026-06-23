---
name: skill-shop-stripe
description: Stripe payment and payout automation skill for SIN-Code agents — checkout, webhooks, payment links, instant payouts, and subscription management.
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
deprecated: true
deprecated_reason: upstream repo SIN-Shop-Center/SIN-Stripe-Bundle is not maintained and no runnable MCP entrypoint exists
sources:
  - https://github.com/SIN-Shop-Center/SIN-Stripe-Bundle
---

# skill-shop-stripe

## Overview

Automate Stripe operations for the SIN webshop fleet: payment links, checkout sessions, webhooks, instant payouts, and reconciliation.

## When to Use

- Creating payment links or checkout sessions.
- Processing instant payouts.
- Handling webhook events and reconciliation.

## When NOT to Use

- The project does not use Stripe as a payment provider.
- A native SIN-Code `sin-code skill-shop-stripe` command is available (future).

## Core Process

```
SETUP → CREATE LINK → PAY → HANDLE WEBHOOK → PAYOUT
```

1. Configure Stripe API keys and webhook endpoints.
2. Create payment links or checkout sessions.
3. Process customer payments.
4. Handle and verify webhook events.
5. Trigger instant payouts where configured.

## Verification

- [ ] Stripe API keys are stored securely.
- [ ] Webhook signatures are verified.
- [ ] Payouts are only triggered for eligible balances.
- [ ] Failed payments are logged and retried.
