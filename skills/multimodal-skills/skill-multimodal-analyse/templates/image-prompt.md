---
name: image-prompt
description: Reusable prompt template for an image preprocessing task. Pair with tasks/preprocess-image.md.
---

# Preprocess image: {{PATH}}

You are opening the image asset at `{{PATH}}`. Use the
`skill-multimodal-analyse` workflow to extract structure before
responding to the user.

## Required steps

1. Confirm the extension is in the known image set. If not, route
   through `analyse__data_detect` first.
2. Call `analyse__image_extract` with `path = {{PATH}}`.
3. Branch on the response:
   - `ocr_text` non-empty → summarise the text in one paragraph.
   - `ocr_text` empty → report dimensions, format, EXIF. Do not
     invent content.
   - `ocr_text` very long (over 4 KB) → ask the user whether to
     persist via `sin_write` to `{{OCR_OUT}}`.
4. If the user asks to fix or edit something visible in the image,
   return to the upstream source (Figma / SVG / diagram file) — do
   not edit the raster.

## User context

{{CONTEXT}}

## Output expectations

- One short paragraph describing the image.
- Dimensions, format, and EXIF as a small table.
- If OCR was meaningful, the first 3 lines verbatim (full text
  available on request).
- If security-relevant EXIF (GPS coordinates, internal hostnames),
  flag with `M4` notation.
