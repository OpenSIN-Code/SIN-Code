# instinct/store.go — disk layout and atomic writes

## On-disk layout

```
<base>/
├── global/
│   └── instincts/
│       └── <id>.md
├── projects/
│   └── <project_id>/
│       ├── meta.json
│       └── instincts/
│           └── <id>.md
└── audit.jsonl          # append-only learning event log
```

`<base>` resolution: `SIN_INSTINCT_DIR` → `$XDG_DATA_HOME/sin-code/instinct`
→ `~/.local/share/sin-code/instinct`. We deliberately do **not** write
into the working directory — that would pollute the repo and break
across-machine sync semantics.

## Why no cross-process lock

The ECC reference suggests `flock`-based serialization. SIN-Code uses
modernc/sqlite for the lessons/ledger store, and the instinct writer
runs on the same goroutine as the hook dispatcher. Cross-process
contention is a non-concern for the single-binary, single-writer model.
If a future user needs parallel writers, wrap `Save` in `flock` —
`saveUnlocked` is the inner function and is the natural extension point.

## Atomic writes

Every save uses `tmp` + `os.Rename`. On any modern filesystem the
rename is atomic at the directory-entry level, so a concurrent reader
sees either the old or the new file — never a half-written one.

## Related files

- `types.go` — `Instinct` schema
- `frontmatter.go` — `Marshal` / `Unmarshal`
- `audit.go` — append-only event log sharing `<base>`
