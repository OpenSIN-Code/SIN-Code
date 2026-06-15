---
name: cj-dropshipping
description: CJ Dropshipping API skill for SIN-Code agents — product search, import, sync, orders, freight, reviews, and supplier orchestration.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
lifecycle: external
sources:
  - https://github.com/OpenSIN-Code/cj-dropshipping-skill
  - https://github.com/SIN-Shop-Center/SIN-CJDropshipping-Bundle
---

# cj-dropshipping

## Overview

Interface with CJ Dropshipping for product sourcing, order fulfillment, and freight calculation in the SIN webshop fleet.

## When to Use

- Searching or importing products from CJ Dropshipping.
- Syncing orders, inventory, or tracking information.
- Calculating freight costs for cross-border shipments.

## When NOT to Use

- The shop does not use CJ Dropshipping as a supplier.
- A native SIN-Code `sin-code cj` command is available (future).

## Core Process

```
SEARCH → IMPORT → SYNC → ORDER → TRACK
```

1. Search products by keyword or category.
2. Import selected products into the local shop catalog.
3. Sync inventory and pricing.
4. Push customer orders to CJ Dropshipping.
5. Track fulfillment and freight status.

## Verification

- [ ] Product data is validated before import.
- [ ] Order payloads match CJ Dropshipping API schema.
- [ ] Freight estimates are requested for the correct destination.
- [ ] Sync errors are logged and retried.
