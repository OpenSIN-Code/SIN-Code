# serve_loop_handler.doc.md

**Purpose:** MCP handler for `sin_run_loop` — synchronous full-agent-loop delegation.
**Docs:** Parses prompt + options, builds `loopbuilder.Config`, calls `loopbuilder.Build()` in-process, runs `loop.Run()`, returns `{session_id, summary, verified, turns}` JSON. Uses factory injection pattern to avoid import cycle (`internal` → `loopbuilder` → `internal`).
