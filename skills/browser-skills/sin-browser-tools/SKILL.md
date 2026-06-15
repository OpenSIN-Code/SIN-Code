---
name: sin-browser-tools
description: Browser automation tools for agents. Navigate, click, screenshot, scrape, and interact with web pages.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-browser-tools

## Overview

Use browser automation to interact with the web: navigate, click, screenshot, scrape, fill forms.

## When to Use

- User asks to visit a website, test a page, screenshot, scrape, or interact with a web app.

## When NOT to Use

- The data is available via API (use API instead).
- The user has not authorized web interaction.

## Core Process

```
NAVIGATE → OBSERVE → ACT → VERIFY
```

1. Navigate to the target URL.
2. Observe the page state (title, elements, text).
3. Perform actions (click, type, scroll).
4. Verify expected outcome.

## Safety

- Respect robots.txt.
- Do not submit sensitive data.
- Avoid automated account creation unless explicitly allowed.

## Verification

- [ ] URL reached.
- [ ] Expected element or text present.
- [ ] Action outcome matches expectation.
- [ ] Screenshot or artifact saved if requested.
