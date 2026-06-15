---
name: sin-honcho
description: Behavioral memory layer for opencode agents. Stores conversations, preferences, and peer models across sessions with graceful degradation.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-honcho

## Overview

Persist behavioral memory across sessions: conversations, preferences, and peer models. Complementary to SCKG (code structure) and ceo-audit (quality).

## When to Use

- User says "user preference", "remember that", "remember this", "what did the user say", "user feedback", "behavioral memory", "session context", "across sessions", "persistent memory", "honcho", "peer model".

## When NOT to Use

- The information is purely about code structure (use SCKG).
- The user wants to delete everything without confirmation.

## Core Process

```
STORE → RECALL → UPDATE → DECAY
```

1. Store conversations, preferences, and peer models.
2. Recall relevant context when needed.
3. Update entries based on new feedback.
4. Let old entries decay naturally.

## Graceful Degradation

If the Honcho server is unreachable, continue without crashing. Log the failure.

## Verification

- [ ] Memory is stored.
- [ ] Recall returns relevant context.
- [ ] Updates do not overwrite without intent.
- [ ] Server unreachable degrades gracefully.
