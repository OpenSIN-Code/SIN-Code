# Tool Reference

The eight `analyse__*` tools exposed by `sin-analyse-suite`. Every
tool is read-only and `allow` policy (M4 invariant). One-line
description per tool plus expected return shape.

| Tool | Modality | Description | Returns |
|------|----------|-------------|---------|
| `analyse__image_extract` | image | Detect dimensions, OCR text, EXIF metadata | `{width, height, format, ocr_text, exif}` |
| `analyse__pdf_parse` | document | Extract text passages and tables per page | `{pages: [{n, text, tables: [...]}]}` |
| `analyse__log_analyze` | logs | Detect format, parse lines, identify errors + signatures | `{format, lines, errors, signatures}` |
| `analyse__data_detect` | data file | Detect encoding + schema (JSON / CSV / Parquet / Arrow / ...) | `{format, encoding, schema, sample_rows}` |
| `analyse__audio_transcribe` | audio | Speech-to-text with speaker diarisation | `{transcript, speakers, segments}` |
| `analyse__video_extract` | video | Extract keyframes, audio track, codec metadata | `{codec, duration, keyframes: [...], audio_ref}` |

## Conventions

- All tools accept `path` (absolute or relative to the workspace root).
- All tools return JSON. No tool returns a binary blob.
- All tools are **deterministic per `(path, suite_version)`** — the
  same input returns the same bytes, byte-for-byte. This is the
  prerequisite for command-cache hits and for the four-arm comparator
  (issue #171) to pin eval snapshots.
- All tools refuse to operate on paths outside the workspace root
  unless `SIN_ANALYSE_ALLOW_EXTERNAL=1` is set (escape hatch).

## Error surface

Each tool returns one of:

- `{ok: true, ...payload}` — success
- `{ok: false, error: "ENOENT", path: "..."}` — file missing
- `{ok: false, error: "EUNSUPPORTED", format: "..."}` — modality
  recognised but format not supported
- `{ok: false, error: "ETOOLARGE", size_mb: N, limit_mb: M}` — payload
  exceeds the per-tool byte budget; the agent should narrow scope
  (e.g. page range, time range) before retrying

Each failure short-circuits the workflow at the EXTRACT step; the
agent falls back to user-visible error and asks for a narrower scope.
