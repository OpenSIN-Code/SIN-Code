---
name: skill-browser-tools
description: Use when user says 'open browser', 'navigate to', 'click', 'screenshot', 'scrape', 'browser automation', 'web scraping'. Browser automation and CDP evidence capture for agents. Navigate, record, screenshot, scrape, and interact with web pages. Surfaces deterministic findings (network failures, JS exceptions, CORS/CSP violations, security state) without requiring LLM interpretation of raw logs.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
metadata:
  author: SIN-Code
  version: 3.20.0
  sources: "OpenSIN-Code/SIN-Browser-Tools"
required_tools:
  - sin_scout
  - sin_execute
lifecycle: external
---

# skill-browser-tools

## Overview

Use browser automation to interact with the web and to capture a full Chrome DevTools Protocol (CDP) ground-truth log of every navigation. The builtin tools drive a headless Chrome instance and expose three levels of output:

1. **sin_browser_navigate** — navigate and record
2. **sin_browser_findings** — classified, grouped findings (errors / warnings)
3. **sin_browser_snapshot** — full session summary with raw event counts

## When to Use

- Visit a website, test a page, screenshot, or scrape content.
- Diagnose why a page is broken: network failures, JS exceptions, CORS/CSP errors.
- Verify a fix by comparing findings before and after.
- Capture a reproducible evidence log for a bug report.

## When NOT to Use

- The data is available via a public API (use `sin_http_get` instead).
- The user has not authorised web interaction.
- The operation requires a logged-in session with sensitive credentials.

## Core Process

```
NAVIGATE (sin_browser_navigate)
  → OBSERVE findings (sin_browser_findings)
  → ACT on errors (edit code / config)
  → VERIFY (sin_browser_navigate again, compare findings)
```

## Builtin Tools

### sin_browser_navigate

Drives headless Chrome to a URL and records the complete CDP event stream.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `url` | string | yes | `http(s)` URL to load |
| `step` | string | no | Correlation label (e.g. `"login_submit"`) |
| `wait_sec` | string | no | Seconds to wait after load (default `"3"`, max `"120"`) |

Returns: event count by CDP domain, path to the JSONL ground-truth log.

Permission: **ask** (loads arbitrary URLs).

**What is recorded (CDP domains):**

| Domain | Events captured |
|--------|----------------|
| Network | `requestWillBeSent`, `requestWillBeSentExtraInfo`, `responseReceived`, `responseReceivedExtraInfo`, `dataReceived`, `loadingFinished`, `loadingFailed`, `requestServedFromCache`, `resourceChangedPriority`, `webSocketCreated/Frame*/Closed`, `eventSourceMessageReceived` |
| Runtime | `consoleAPICalled`, `exceptionThrown` |
| Log | `entryAdded` |
| **Audits** | `issueAdded` — CORS, CSP violations, mixed content, SameSite cookies, low contrast, deprecation warnings (pre-classified by Chrome) |
| **Security** | `securityStateChanged` — TLS/certificate/mixed-content state |
| Page | `loadEventFired`, `domContentEventFired`, `lifecycleEvent`, `frameNavigated`, `frameRequestedNavigation`, `javascriptDialogOpening`, `fileChooserOpened`, `downloadWillBegin/Progress` |
| Target | `attachedToTarget`, `detachedFromTarget`, `targetCreated/Destroyed` — OOPIFs and workers via `setAutoAttach(flatten=true)` |
| Performance | `getMetrics` polled every 2 s |

Response bodies are captured on `loadingFinished` (deferred for correct timing), capped at 2 MiB each, and appended as synthetic `Network/responseBody` events.

Each event carries:
- `seq` — global monotonic sequence number
- `wall_time` — RFC3339Nano timestamp
- `mono_nanos` — nanoseconds since session start (use for latency)
- `step_id` — correlation label set by `step` parameter
- `session_id` — CDP session for OOPIF / worker events

---

### sin_browser_findings

Runs the deterministic Findings engine over the last recorded session.

No parameters required.

Returns: JSON array of `Finding` objects, sorted by severity (error → warn → info) then by count (descending). Returns a "no findings" message when the page loaded cleanly.

Permission: **allow** (reads in-memory state only, no network calls).

**Finding schema:**

```json
{
  "category": "console | exception | network | audit | security",
  "severity":  "error | warn | info",
  "title":     "Human-readable description",
  "signature": "stable dedup key",
  "count":     42,
  "first_seq": 17,
  "last_seq":  312,
  "sample":    "First 200 chars of the triggering event"
}
```

**What is classified:**

| Category | Severity rule |
|----------|--------------|
| Console `error` / `assert` | error |
| Console `warning` | warn |
| Uncaught JS exception | error |
| Network `loadingFailed` (blocked) | error |
| HTTP 5xx response | error |
| HTTP 4xx response | warn |
| Audits `issueAdded` (any code) | warn |
| Security state `insecure` / `neutral` | warn |

---

### sin_browser_snapshot

Returns a compact JSON summary of the last session without re-running the full engine: total event count, first/last wall times, per-domain-method event counts, finding count, findings list, and the JSONL file path.

No parameters required.

Permission: **allow**.

---

## JSONL Ground-Truth Log

Every session writes a JSONL file to the OS temp directory. The path is returned by `sin_browser_navigate` and included in the snapshot. The file is a forensic record that can be:

- Read directly with `sin_read` for deep inspection.
- Used as evidence in bug reports.
- Diffed across runs to isolate regressions.

Format: one JSON object per line, schema matches the `Event` type in `pkg/browser/cdp/event.go`.

---

## Known Limitations

- **OOPIF console capture**: `Runtime.enable` / `Network.enable` / `Audits.enable` are not yet sent on auto-attached child sessions (cross-origin iframes, workers). Console errors and network failures inside those frames are not captured. Tracked as `TODO(oopif)` in `pkg/browser/cdp/recorder.go`.
- **Requires Chrome/Chromium**: the `chromedp` allocator uses the system Chrome binary. If Chrome is not installed, `sin_browser_navigate` returns an error.
- **Single session**: only one recording session is active at a time; a new `sin_browser_navigate` call tears down the previous session.

---

## Safety

- Respect `robots.txt`.
- Do not submit sensitive credentials — the recording captures all network traffic including request bodies.
- Avoid automated account creation unless explicitly allowed.

---

## Verification Checklist

- [ ] `sin_browser_navigate` returns event counts for expected domains.
- [ ] `sin_browser_findings` returns 0 findings (or known/acceptable ones) after a fix.
- [ ] JSONL path is present and non-empty.
- [ ] Screenshot or DOM snapshot saved if requested.
