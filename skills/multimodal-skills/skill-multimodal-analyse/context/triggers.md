# Context: Triggers & Modality Detection

Docs: ../SKILL.md

## Trigger Phrases

- "analyse this image", "what's in the picture", "OCR this"
- "parse the PDF", "extract text from PDF", "read the document"
- "summarize the logs", "find errors in the log", "log analysis"
- "what format is this file", "detect schema", "data file"
- "transcribe the audio", "audio to text"
- "extract frames from video", "video metadata"

## Modality Detection Order

1. Extension: `.png/.jpg/.webp/.gif` → image, `.pdf` → document,
   `.log/.jsonl/.txt` (large) → log, `.csv/.parquet/.json` → data,
   `.mp3/.wav/.m4a` → audio, `.mp4/.mov/.webm` → video.
2. MIME type cross-check (when extension is ambiguous).
3. First 4 bytes magic-number sniff for unmarked binaries.

## Boundaries

- **In scope:** Calling exactly one `analyse__*` tool per binary file, then
  reasoning about its structured output.
- **Out of scope:** Calling `read` / `cat` / `od` on the same file (would
  produce garbage bytes for images / PDFs / video).

## Required Input

The file path (absolute or repo-relative) plus optional user question.

## Tone

Evidence-first. Quote tool output verbatim, then reason.
