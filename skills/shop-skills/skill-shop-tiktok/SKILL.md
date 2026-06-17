---
name: skill-shop-tiktok
description: TikTok Shop automation and scraper skill for SIN-Code agents — product discovery, listing sync, order tracking, and trend analytics.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 1.0.0
lifecycle: external
sources:
  - https://github.com/SIN-Shop-Center/SIN-eCommerce-Scraper-Bundle
---

# skill-shop-tiktok

## Overview

Automate TikTok Shop operations: product discovery, listing synchronization, order tracking, and trend-based commerce decisions.

## When to Use

- Scraping or discovering TikTok Shop products.
- Syncing TikTok Shop listings with the local catalog.
- Tracking orders and trend metrics from TikTok Shop.

## When NOT to Use

- The shop does not operate on TikTok Shop.
- A native SIN-Code `sin-code skill-shop-tiktok` command is available (future).

## Core Process

```
DISCOVER → SCRAPE → SYNC → LIST → TRACK
```

1. Discover trending products and categories.
2. Scrape product details and pricing.
3. Sync scraped data into the local shop catalog.
4. Create or update TikTok Shop listings.
5. Track orders and trend performance.

## Verification

- [ ] Scraping respects rate limits and terms of service.
- [ ] Product data is validated before sync.
- [ ] Listing updates are idempotent.
- [ ] Trend metrics are timestamped and reproducible.
