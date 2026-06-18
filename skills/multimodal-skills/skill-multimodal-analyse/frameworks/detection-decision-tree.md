# Detection Decision Tree

The first step of the workflow is **DETECT** — picking the right
`analyse__*` tool for the asset at hand. Use the tree below; do not
guess.

```
Is the file path or extension obvious?
   --- YES ------------------------------------+
   | PNG/JPG/WebP/GIF/HEIC/TIFF  -> analyse__image_extract |
   | PDF                          -> analyse__pdf_parse     |
   | .log / .jsonl / .ndjson     -> analyse__log_analyze   |
   ---------------------------------------------+

No obvious extension / generic blob
   |
   v
Call analyse__data_detect FIRST.
   |
   +---> recognised as CSV/Parquet/Arrow/JSON/...
   |       -> use analyse__data_detect payload (already structured)
   |
   +---> recognised as text/* (plain text, markdown, source code)
   |       -> STOP. Return to sin_read / sin_scout path.
   |
   +---> recognised as audio (mp3/wav/flac/ogg/m4a)
   |       -> analyse__audio_transcribe
   |
   +---> recognised as video (mp4/mov/webm/mkv)
   |       -> analyse__video_extract
   |
   +---> still unknown (binary blob)
           -> surface to user with analyse__data_detect's
              `{format: "unknown", magic: "...", size_bytes: N}`
```

## Why sniff by extension first

`analyse__data_detect` is correct in all cases but it is **slow**
compared to extension sniffing. For obvious cases (PDF, image, log)
the detector's output is already implied by the extension; calling
it would add latency for no benefit.

For unknown paths — `attachments/<hash>`, files retrieved via
`analyse__harvest`, blob storage — the detector is mandatory.

## Concrete rules

| Filename pattern | Skip detect, call directly |
|------------------|----------------------------|
| `*.png`, `*.jpg`, `*.jpeg`, `*.webp`, `*.gif`, `*.heic`, `*.tiff` | `analyse__image_extract` |
| `*.pdf` | `analyse__pdf_parse` |
| `*.log`, `*.jsonl`, `*.ndjson` | `analyse__log_analyze` |
| `*.mp3`, `*.wav`, `*.flac`, `*.ogg`, `*.m4a` | `analyse__audio_transcribe` |
| `*.mp4`, `*.mov`, `*.webm`, `*.mkv` | `analyse__video_extract` |
| Anything else | `analyse__data_detect` first |

## Never the wrong tool

If the user explicitly names a tool (e.g. "OCR this PNG with
`analyse__image_extract`") honour the request and skip the tree.
The decision tree is for the common case where the user says "what
is this file?" without naming a tool.
