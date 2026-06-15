---
name: sin-preview
description: Open generated images and screenshots in macOS Preview automatically. Always use when creating or referencing images.
license: MIT
compatibility:
  - opencode
  - sin-code
metadata:
  author: SIN-Code
  version: 1.0.0
---

# sin-preview

## Overview

Open any image or screenshot in macOS Preview automatically. Never tell the user to browse to /tmp.

## When to Use

- Always when creating or referencing an image file.

## When NOT to Use

- Never skip this for images.

## Core Process

```
GENERATE IMAGE → SAVE → OPEN IN PREVIEW
```

1. Create or locate the image.
2. Save to an absolute path.
3. Open in Preview.

## Verification

- [ ] Image file exists.
- [ ] Preview opens without error.
- [ ] User is informed of the path.
