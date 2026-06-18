# Task: Preprocess an Image

Use this task when the user opens an image asset (PNG, JPG, WebP, GIF,
HEIC, TIFF) and asks the agent to describe it, OCR it, or extract
metadata. The agent must never read the binary directly into context
— it routes through `analyse__image_extract`.

## Inputs

- `path` — absolute path to the image file inside the workspace root

## Workflow

```
1. Verify extension in the known set (PNG/JPG/WebP/GIF/HEIC/TIFF)
2. Call analyse__image_extract(path=...)
3. Branch on ocr_text presence
4. Surface dimensions + format + EXIF for coding agents that need geometry
5. If user wants the OCR text persisted:
     sin_write(path="assets/<basename>.ocr.txt", content=ocr_text)
6. Return a one-paragraph caption + dimensions to the conversation
```

## Step-by-step

### Step 1 — Verify extension

If the extension is not in the known set, route through
`analyse__data_detect` first per
`frameworks/detection-decision-tree.md`.

### Step 2 — Call the tool

```bash
sin-code mcp call analyse__image_extract \
  --path assets/diagram.png \
  --json
```

Expected response:

```json
{
  "ok": true,
  "path": "assets/diagram.png",
  "width": 1920,
  "height": 1080,
  "format": "png",
  "ocr_text": "Auth flow: User -> OAuth -> Token -> API",
  "exif": {
    "software": "Figma",
    "create_date": "2026-05-04T10:14:00Z"
  }
}
```

### Step 3 — Branch on OCR

- `ocr_text` is non-empty and the user asked to summarise → return
  the text plus dimensions as a one-paragraph caption.
- `ocr_text` is empty (UI screenshot, diagram, photo) → return
  dimensions, format, and EXIF; do not invent content.
- `ocr_text` is very long (> 4 KB) → persist to a sibling file via
  `sin_write` and `sin_read` the result back on demand.

### Step 4 — Persist payload (optional, ask if writing)

If the user wants the OCR text preserved:

```bash
sin-code mcp call analyse__image_extract --path assets/diagram.png > /tmp/raw.json
sin_write assets/diagram.ocr.txt "$(jq -r .ocr_text /tmp/raw.json)"
```

`sin_write` is `ask` policy. Surface the proposed write path to the
user; do not run it headlessly without `--yolo`.

## Verification

- [ ] `analyse__image_extract` returned `{ok: true, ...}`.
- [ ] Dimensions + format reported to user.
- [ ] If OCR text was non-empty, offered to persist via `sin_write`.
- [ ] If persist was chosen, file written and `sha256` returned to
  user; source image unchanged (`stat` before/after identical).
