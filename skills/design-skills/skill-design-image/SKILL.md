---
name: skill-design-image
description: Generate, edit, and inspect images for the project. Create diagrams, screenshots, or artwork.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
lifecycle: native
---

# skill-design-image

## Overview

Generate images, diagrams, or artwork for the project. Open images in Preview automatically.

## When to Use

- User asks for an image, diagram, illustration, or screenshot.
- A visual artifact is needed for documentation or design.

## When NOT to Use

- The task is purely textual or analytical.

## Core Process

```
DESCRIBE → GENERATE → INSPECT → SAVE
```

1. Capture the image description or intent.
2. Generate the image using the appropriate tool.
3. Inspect the result.
4. Save and open the artifact.

## Verification

- [ ] Image matches prompt.
- [ ] File saved to expected path.
- [ ] Preview opened if applicable.
- [ ] User is informed of the path.
