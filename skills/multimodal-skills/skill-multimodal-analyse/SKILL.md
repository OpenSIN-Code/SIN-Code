---
name: skill-multimodal-analyse
description: "Read-only multimodal preprocessing using sin-analyse-suite MCP. Image, video, PDF, logs, data, audio detection/extraction."
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: OpenSIN-Code
  version: 3.22.0
  category: multimodal
  lifecycle: native
required_tools:
  - analyse__image_extract
  - analyse__pdf_parse
  - analyse__log_analyze
  - analyse__data_detect
  - analyse__audio_transcribe
lifecycle: native
---

# skill-multimodal-analyse

Read-only multimodal preprocessing for coding agents. Format detection, text
extraction, metadata scraping, and structural analysis for the six
non-text asset classes most repos touch: image, PDF, log file, data file,
audio, video. Sits upstream of `sin_read` / `sin_edit` when the input is
a binary blob and downstream of code analysis — never replaces either.

## When to activate

Activate when the user hands the agent a non-source artifact and the next
prompt implies "look at it, summarise it, structure it" rather than
"render it, edit it, ship it".

Concrete triggers:

- "OCR this image", "extract text from PDF", "parse this log"
- "what format is this file", "what codec is this video"
- "transcribe this audio", "summarise this screencast"
- "preprocess the screenshots in assets/", "scan logs for errors"
- The agent opens a file with a non-text MIME type and must decide
  what to do before downstream `sin_read` / `sin_edit`.

Do **not** activate for:

- Source code in any language (use `sin_read` / `sin_scout` / `sin_sckg`)
- Generated reports, markdown, JSON, TOML (already text)
- Live device control, screen capture, or any mutation of source assets

## Mandatory workflow

```
DETECT  ->  EXTRACT  ->  STRUCTURE  ->  DELEGATE
```

1. **DETECT** — call `analyse__data_detect` on the file (or sniff
   magic bytes for images/PDF where detection is obvious). Stop if the
   file is plain text — return to the regular `sin_read` path.
2. **EXTRACT** — call the modality-specific tool
   (`analyse__image_extract`, `analyse__pdf_parse`,
   `analyse__log_analyze`, `analyse__audio_transcribe`,
   `analyse__video_extract`). Never read the binary into the
   conversation directly.
3. **STRUCTURE** — emit a structured payload: dimensions / text /
   error signatures / schema / transcript / keyframes. No free-form
   prose — the downstream agent must be able to `sin_edit` the result.
4. **DELEGATE** — pass the structured payload back to `sin_read`
   (for code-adjacent assets) or to `sin_edit` (for assets that need
   replacement content).

## Read-only invariant

`analyse__*` is `allow` policy. **It never mutates the input.** The
skill must not call `sin_write`, `sin_edit`, or `sin_bash` to modify
the asset itself — only to **persist** the extracted payload to a
new path (e.g. `assets/<name>.ocr.txt`). This satisfies M4 (no
destructive tool calls without `ask`).

## Required-tools coverage

Per AGENTS.md issue #248, `required_tools` lists only the tools this
skill **invokes** during the workflow. The five activated tools
(`analyse__image_extract`, `analyse__pdf_parse`, `analyse__log_analyze`,
`analyse__data_detect`, `analyse__audio_transcribe`) cover the common
five modalities. `analyse__video_extract` is available via the suite
but is not listed because typical agent sessions wake this skill on
text-like assets; enable it on demand via `sin-code skill activate
skill-multimodal-analyse --with-video`.

## Permission policy

```
analyse__*  ->  allow  (read-only, M4 invariant)
sin_write   ->  ask    (only to persist extracted payloads)
sin_edit    ->  ask    (only to patch code with extracted strings)
```

## Skill coupling

This skill is independent of other bundled skills. It cooperates with:

- `sin_code_sin_read` — receives structured payloads via
  `data.attachment.extracted_text`.
- `sin_code_sin_edit` — receives extracted strings for surgical edits.
- `skill-code-audit` — log analysis feeds into security-scoring for
  findings such as leaked tokens in structured logs.
