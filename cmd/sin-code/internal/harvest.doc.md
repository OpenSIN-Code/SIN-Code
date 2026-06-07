# `harvest.doc.md` — URL Fetching Subcommand

Fetches URLs with a local disk cache, structure extraction, change detection, and basic HTTP client management.

## What it does

- **Fetches HTTP/HTTPS URLs** with configurable method and timeout using Go's `net/http`.
- **Caches responses** on disk in `~/.cache/sin-code/harvest/` with a 5-minute TTL. Cache key is SHA256 of `METHOD URL`.
- **Returns structured output** including status, headers, body, duration, and cache hit/miss info.
- **Sets sensible headers:** `User-Agent: sin-code/1.0` and `Accept: text/html,application/json,text/plain,*/*`.
- **No auth management yet:** The flag exists but credentials are not injected (see caveats).

## Files that import / touch it

- `cmd/sin-code/main.go` — registers `HarvestCmd` into the root cobra command
- `cmd/sin-code/internal/harvest.go` — self-contained HTTP client

## Important config values & limits

| Flag | Default | Description |
|---|---|---|
| `--url` | *(required)* | URL to fetch |
| `--method` | `GET` | HTTP method (GET, POST, etc.) |
| `--timeout` | `30` | Request timeout in seconds |
| `--format` | `text` | Output: `text` or `json` |

- **Cache TTL:** 5 minutes (hardcoded). Cache is stored as JSON in `~/.cache/sin-code/harvest/`.
- **Cache key:** SHA256 hash of `METHOD URL` (e.g., `GET https://api.example.com/data`).
- **No body for POST/PUT:** Currently `harvest` does not support request bodies. Use `curl` for POST with payload.

## Usage examples

```bash
# Fetch an API endpoint with JSON output
sin-code harvest --url https://api.github.com/repos/OpenSIN-Code/SIN-Code-Bundle --format json

# Check cache hit (run twice within 5 minutes)
sin-code harvest --url https://example.com

# Use HEAD method to check status only
sin-code harvest --url https://example.com --method HEAD
```

## Known caveats / footguns

- **Cache is unencrypted:** Response bodies (including potential secrets) are stored as plain JSON in `~/.cache/sin-code/harvest/`. Clear cache manually if sensitive data was fetched.
- **No request body support:** POST/PUT/PATCH only send headers, no body. This is a known limitation for API writes.
- **No auth header injection:** The `--auth` flag is not implemented. Add auth headers manually via `curl` or another tool.
- **Cache does not respect Cache-Control:** The 5-minute TTL is hardcoded. Dynamic content may be stale.
- **Redirects are handled by `http.Client` default:** Up to 10 redirects are followed automatically. No way to disable.
- **Large responses:** The entire body is read into memory and stored in cache. Fetching multi-gigabyte files will exhaust RAM.