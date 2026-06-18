# sin-analyse-suite Overview

`sin-analyse-suite` is a read-only multimodal preprocessing MCP server
in the OpenSIN-Code ecosystem. It exposes the `analyse__*` tool surface
that any compliant MCP client (sin-code, opencode, claude-code, codex)
can call to extract structure from non-text assets before the agent's
text-native tools (`sin_read`, `sin_edit`, `sin_scout`) take over.

## Why it exists

A coding agent's loop is text-native. Source files, configs, READMEs,
logs — all flow through `sin_read` and produce text. But every real
repo has a long tail of binary assets: screenshots, diagrams, scanned
PDFs, screencast recordings, raw data dumps, log archives. Without a
preprocessing layer the agent either refuses to look at them or worse,
forces them into the context window raw.

`sin-analyse-suite` is the bridge. It returns **structured text
payloads** derived from the binary, so the agent loop can `sin_read`
the result and continue normally.

## Server surface

Eight tools, all read-only, all `allow` policy:

| Group | Tools |
|-------|-------|
| Image | `analyse__image_extract` |
| Document | `analyse__pdf_parse` |
| Logs | `analyse__log_analyze` |
| Data files | `analyse__data_detect` |
| Audio | `analyse__audio_transcribe` |
| Video | `analyse__video_extract` |

## Activation

Add the bundled skill `skill-multimodal-analyse` to a chat session
either interactively (`sin-code skill activate skill-multimodal-analyse`)
or via the project-local `.sin-code/autoactivate.toml` (issue #176).
Once active, the skill's `required_tools` are merged into the agent
loop's `CoverageRequiredTools` (issue #248), so the loop refuses to
declare done without invoking at least one `analyse__*` call when an
asset of the relevant type was opened.

## What it is NOT

- Not a transcription service for production videos (use Whisper / AWS
  Transcribe directly for batch jobs).
- Not a renderer — it does not produce new images, PDFs, or videos.
- Not a database — payloads are returned per call, not stored server
  side. The agent persists them via `sin_write` if needed.
- Not a code analyser — `sin_sckg`, `sin_scout`, `sin_grasp` own the
  source-code side. Multimodal sits on the asset side.
