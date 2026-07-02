---
name: skill-browser-tools-complete
description: >
  Complete browser automation and webapp testing — 116 tools covering
  navigation, interaction, forms, Shadow DOM, OOPIF frames, screenshots,
  PDFs, drag-drop, network mocking, screen recording, multi-session
  isolation, diagnostics, and playbooks. Triggers: open browser, navigate,
  click, fill form, screenshot, scrape, extract, test webapp, test website,
  browser automation, web scraping, debug page, playwright, browser test,
  headless, chrome, puppeteer, cypress, browser click, browser type,
  browser fill, browser wait, browser scroll, browser snapshot, browser PDF,
  browser download, browser upload, browser cookie, browser mock, browser
  assert, browser captcha, browser frame, browser shadow DOM, browser tab,
  browser session, browser record, browser diagnose, browser learn.
license: MIT
compatibility:
  - sin-code
  - opencode
  - claude-code
  - codex
  - cursor
metadata:
  author: SIN-Code
  version: 3.29.0
  sources:
    - "OpenSIN-Code/SIN-Browser-Tools"
    - "SIN-Code built-in CDP tools"
required_tools:
  - sin_scout
  - sin_execute
  - sin_web_search
lifecycle: external
---

# sin-browser-tools — Complete Browser Automation Skill

## Overview

This skill provides **100% browser coverage** for agents: navigate pages, fill
forms, click buttons, take screenshots, scrape data, test webapps, debug
failures, mock APIs, record sessions, and learn from past automations.

**Two tool layers work together:**

| Layer | Prefix | Count | Engine | Purpose |
|-------|--------|-------|--------|---------|
| **Builtin CDP** | `sin_browser_*` | 10 | Chrome DevTools Protocol | Evidence capture, findings, diff |
| **External MCP** | `browser_*` | 106 | Playwright (async) | Full human-like browser automation |

Use `sin_browser_*` for diagnostic evidence capture (network failures, JS
exceptions, CORS violations). Use `browser_*` for everything else (clicks,
forms, screenshots, scraping, tabs, sessions, mocking).

---

## When to Use

- **Test a webapp**: navigate, fill forms, click through flows, verify UI
- **Scrape data**: extract text, links, tables, images from pages
- **Debug a broken page**: capture console errors, network failures, JS exceptions
- **Verify a fix**: compare browser findings before/after a code change
- **Automate repetitive tasks**: login, checkout, data entry
- **Test responsive design**: resize viewport, take screenshots at breakpoints
- **Test across sessions**: simulate multiple users simultaneously
- **Mock API responses**: intercept network requests, return fake data
- **Capture evidence**: screenshots, PDFs, screen recordings for bug reports
- **Learn from past runs**: record successful automations as playbooks

## When NOT to Use

- The data is available via a public API (use `sin_http_get`)
- The user has not authorized web interaction
- The operation requires real credentials the agent should not have

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  AGENT (sin-code chat / opencode / Claude Code / Cursor)        │
│                                                                   │
│  Layer 1: sin_browser_* (builtin CDP)                           │
│    sin_browser_navigate → sin_browser_findings → sin_browser_diff│
│    Purpose: diagnostic evidence capture                          │
│                                                                   │
│  Layer 2: browser_* (external MCP — SIN-Browser-Tools)          │
│    browser_navigate → browser_snapshot → browser_click → ...     │
│    Purpose: full human-like browser automation                   │
└─────────────────────────────────────────────────────────────────┘
         │                              │
         ▼                              ▼
   Chrome DevTools Protocol      Playwright (async)
   (headless Chrome)             (Chromium/Firefox/WebKit)
```

---

## Tool Inventory — All 116 Tools

### Layer 1: Builtin CDP Tools (10 tools)

These are built into the sin-code binary. They capture a full CDP event
stream and produce classified findings.

| Tool | Policy | Description |
|------|--------|-------------|
| `sin_browser_navigate` | ask | Navigate to URL, record full CDP event stream (network, console, exceptions, audits, security, Web Vitals) |
| `sin_browser_findings` | allow | Return classified Findings from last navigation (errors, warnings, info) |
| `sin_browser_snapshot` | allow | Compact JSON summary of last session (event counts, finding count, JSONL path) |
| `sin_browser_vitals_flush` | allow | Force Web Vitals metric flush (LCP, CLS, INP) before calling findings |
| `sin_browser_diff` | allow | Compare two sessions — baseline vs current — return resolved/introduced/persisted findings |

**Evidence flow:**
```
sin_browser_navigate(url)     → records CDP events to JSONL
sin_browser_vitals_flush()    → captures final Web Vitals
sin_browser_findings()        → classified findings (error/warn/info)
sin_browser_diff()            → before/after comparison
```

### Layer 2: External MCP Tools (106 tools)

These come from SIN-Browser-Tools (`browser_*` prefix). They use Playwright
for full human-like browser automation.

#### Navigation (8 tools)

| Tool | Description |
|------|-------------|
| `browser_navigate` | Navigate to a URL (domcontentloaded, 30s timeout) |
| `browser_back` | Go back in browser history |
| `browser_forward` | Go forward in browser history |
| `browser_reload` | Reload the current page |
| `browser_scroll` | Scroll by direction and pixel amount |
| `browser_press` | Press a key or key combination (Enter, Tab, Escape, etc.) |
| `browser_get_url` | Return current URL and page title |
| `browser_set_viewport` | Resize viewport (responsive/mobile testing) |

#### Waiters (6 tools)

| Tool | Description |
|------|-------------|
| `browser_wait_for` | Wait for CSS/XPath selector to reach a state (attached/visible/hidden) |
| `browser_wait_for_text` | Poll DOM for text substring to appear (supports Shadow DOM, iframes) |
| `browser_wait_for_load` | Wait for page load state (load/domcontentloaded/networkidle) |
| `browser_wait_for_spa_transition` | MutationObserver-based SPA transition detector (React/Vue/Angular) |
| `browser_wait_for_response` | Wait for network response matching URL substring |
| `browser_wait_for_request` | Wait for network request matching URL substring |

#### Tab Management (4 tools)

| Tool | Description |
|------|-------------|
| `browser_list_tabs` | List all tabs with index, URL, title, active flag |
| `browser_new_tab` | Open new tab, optionally navigate to URL |
| `browser_switch_tab` | Switch active page to tab at given index |
| `browser_close_tab` | Close tab by index; re-focuses another |

#### Interaction — Core (10 tools)

| Tool | Description |
|------|-------------|
| `browser_click` | Click element; auto-routes CDP-backed refs through OOPIF-safe path |
| `browser_click_cdp` | Click via CDP with two-strategy fallback (Playwright role-locator then native coordinate) |
| `browser_double_click` | Double-click element |
| `browser_right_click` | Right-click (context menu) element |
| `browser_hover` | Hover mouse to reveal menus/tooltips |
| `browser_drag` | Drag source element and drop onto target |
| `browser_select_option` | Select option in native `<select>` by value or label |
| `browser_check` | Check or uncheck checkbox/radio input |
| `browser_type` | Type text into element (with optional clear) |
| `browser_fill` | Fill element with text (clear + type) |

#### Interaction — Advanced (7 tools)

| Tool | Description |
|------|-------------|
| `browser_find_by_text` | Find interactive refs by visible/accessible text |
| `browser_click_by_text` | Click best-matching element by visible text; falls back to live locator |
| `browser_click_checkbox_by_text` | Click checkbox by visible label, piercing shadow DOM, SPA-safe |
| `browser_fill_react` | Fill React-controlled input via native value setter + React-compatible events |
| `browser_click_checkbox_react` | Click checkbox while avoiding the `<a>`-tag trap |
| `browser_upload_file` | Upload file to `<input type="file">` element |
| `browser_download` | Trigger and capture file download, saving to disk |

#### Accessibility / Snapshot (2 tools)

| Tool | Description |
|------|-------------|
| `browser_snapshot` | CDP accessibility tree; registers @eN refs for interactive elements |
| `browser_snapshot_full_oopif` | Full accessibility tree across ALL frames (OOPIF + Shadow DOM) |

#### Frame / Shadow DOM (6 tools)

| Tool | Description |
|------|-------------|
| `browser_list_frames` | List every frame with name, URL, parent |
| `browser_eval_in_frame` | Evaluate JavaScript inside specific frame |
| `browser_snapshot_in_frame` | Shadow-DOM-piercing snapshot scoped to one frame |
| `browser_click_in_frame` | Click element (incl. shadow DOM) inside specific frame |
| `browser_type_in_frame` | Type into input inside specific frame, piercing shadow DOM |
| `browser_scan_frames` | Scan ALL frames for text content (finds content in unnamed `about:blank` iframes) |

#### Vision / Screenshots (6 tools)

| Tool | Description |
|------|-------------|
| `browser_vision` | Full-page screenshot as Base64 PNG |
| `browser_screenshot` | Alias for `browser_vision` |
| `browser_screenshot_element` | Screenshot single element by CSS selector |
| `browser_get_images` | List all `<img>` with src, alt, width, height |
| `browser_get_text` | Extract visible text from selector (default: body), truncated to 8000 chars |
| `browser_pdf` | Render page to PDF (headless Chromium only), Base64 |

#### Dialog (2 tools)

| Tool | Description |
|------|-------------|
| `browser_dialog` | Accept or dismiss pending native JS dialog (alert/confirm/prompt) |
| `browser_wait_for_dialog` | Wait for native dialog to appear without consuming it |

#### Data Extraction (9 tools)

| Tool | Description |
|------|-------------|
| `browser_console` | Evaluate JavaScript expression and return result |
| `browser_cdp` | Send raw CDP method with params |
| `browser_get_cookies` | Return cookies for current context |
| `browser_set_cookie` | Set cookie by name/value with URL or domain+path |
| `browser_clear_cookies` | Clear all cookies in current context |
| `browser_get_html` | Return raw HTML of page or selector |
| `browser_get_links` | List every hyperlink with text, href, visibility |
| `browser_get_attribute` | Read single attribute of element by selector |
| `browser_storage` | Read/write localStorage or sessionStorage |

#### Structured Extraction (2 tools)

| Tool | Description |
|------|-------------|
| `browser_extract_table` | Extract HTML table as `list[dict]` with header-to-cell mapping |
| `browser_extract_list` | Extract repeated elements as `list[dict]` with optional field selectors |

#### Cookie Management (2 tools)

| Tool | Description |
|------|-------------|
| `browser_cookies_set` | Batch-add cookies to active context |
| `browser_cookies_clear` | Remove all cookies from active context |

#### Assertions (1 tool)

| Tool | Description |
|------|-------------|
| `browser_assert` | Confirm condition (text visible, selector present, URL contains); returns ok=True/False |

#### CAPTCHA / Bot Detection (1 tool)

| Tool | Description |
|------|-------------|
| `browser_detect_captcha` | Scan page for reCAPTCHA, hCAPTCHA, Turnstile, DataDome, Cloudflare challenge |

#### Network Control (3 tools)

| Tool | Description |
|------|-------------|
| `browser_block_resources` | Abort matching requests by resource type or URL regex |
| `browser_mock_response` | Intercept URL pattern and return fixed response body/status |
| `browser_unroute_all` | Remove all active routes (block/mock) from page |

#### Identity (1 tool)

| Tool | Description |
|------|-------------|
| `browser_set_identity` | Set User-Agent, locale, timezone, geolocation; supports presets |

#### Screen Recording (3 tools)

| Tool | Description |
|------|-------------|
| `browser_screen_record_start` | Start macOS screen recording |
| `browser_screen_record_stop` | Stop recording, return saved video path |
| `browser_screen_record_analyze` | Extract keyframes from recording as Base64 PNGs for vision diagnosis |

#### Window Management (8 tools)

| Tool | Description |
|------|-------------|
| `browser_get_window_bounds` | Get window position, size, state |
| `browser_set_window_bounds` | Set window to exact pixel position/size |
| `browser_set_window_mode` | Resize by preset (small/medium/large/maximized/fullscreen) |
| `browser_maximize_window` | Maximize window |
| `browser_minimize_window` | Minimize to dock/taskbar |
| `browser_fullscreen_window` | Enter true fullscreen mode |
| `browser_restore_window` | Restore from max/min/fullscreen |
| `browser_move_window` | Move to specific screen position |

#### macOS Spaces (5 tools)

| Tool | Description |
|------|-------------|
| `browser_list_spaces` | List all macOS Spaces (virtual desktops) |
| `browser_create_space` | Create new macOS Space |
| `browser_move_to_space` | Move browser window to specific Space |
| `browser_get_window_space` | Get Space browser window is on |
| `browser_send_to_background_space` | Move window to dedicated background Space |

#### Multi-Session / Isolated Contexts (5 tools)

| Tool | Description |
|------|-------------|
| `browser_create_session` | Create isolated session with own cookies/storage/auth |
| `browser_list_sessions` | List all sessions with name, active flag, tab count |
| `browser_switch_session` | Switch to different session |
| `browser_close_session` | Close session and all its tabs |
| `browser_parallel_navigate` | Navigate multiple sessions simultaneously |

#### Diagnostics (11 tools)

| Tool | Description |
|------|-------------|
| `browser_diag_start` | Start CDP evidence recording |
| `browser_diag_stop` | Stop recording, write actions.json, generate report |
| `browser_diag_status` | Get current recorder status and event counts |
| `browser_diag_snapshot_all` | Capture full evidence snapshot (screenshot + DOM + accessibility) |
| `browser_diag_element` | Resolve @eN ref to hard DOM evidence |
| `browser_diag_action` | Execute another tool with before/after evidence capture |
| `browser_diag_query` | Query recorded event stream with filters |
| `browser_diag_console` | Get console messages, JS exceptions, log entries |
| `browser_diag_network` | Summarize recorded network requests |
| `browser_diag_get_body` | Retrieve stored response body by request ID |
| `browser_diag_report` | Generate report.md and report.html from raw data |

#### Learning / Playbooks (4 tools)

| Tool | Description |
|------|-------------|
| `browser_playbook_suggest` | Retrieve top playbook variants for (task, url), ranked by success rate |
| `browser_playbook_record` | Record or update playbook variant with trajectory and metrics |
| `browser_playbook_list` | List all stored playbooks, optionally filtered |
| `browser_playbook_compare` | Compare top 3 variants side-by-side with metrics |

---

## Core Patterns

### Pattern 1: Snapshot-Act-Verify (recommended)

The golden rule of browser automation. Always snapshot before clicking.

```
1. browser_snapshot()          → See the page, get @eN refs
2. browser_click("@e3")       → Act on a specific element
3. browser_snapshot()          → Verify the result
4. browser_screenshot()        → Visual proof
```

### Pattern 2: Login Flow

```
1. browser_navigate("https://example.com/login")
2. browser_fill("input[name='email']", "user@example.com")
3. browser_fill("input[name='password']", "secret")
4. browser_click("button[type='submit']")
5. browser_wait_for_load("networkidle")
6. browser_assert(url_contains="/dashboard")
7. browser_screenshot()
```

### Pattern 3: Form Testing

```
1. browser_navigate(url)
2. browser_snapshot()                    # find all form elements
3. browser_fill("input#name", "Test User")
4. browser_fill_react("input[data-testid='email']", "test@example.com")
5. browser_select_option("select#country", "DE")
6. browser_click_checkbox_react("[data-testid='agree']")
7. browser_click("button[type='submit']")
8. browser_wait_for_text("Success")
```

### Pattern 4: Scrape a List Page

```
1. browser_navigate("https://shop.example.com/products")
2. browser_wait_for(".product-card")
3. browser_extract_list(".product-card", fields=["title", "price", "link"])
4. OR: browser_get_links() + browser_get_text()
```

### Pattern 5: Debug a Failing Page

```
1. sin_browser_navigate("https://broken.example.com")    # CDP evidence
2. sin_browser_findings()                                 # classified errors
3. browser_navigate("https://broken.example.com")         # Playwright view
4. browser_snapshot()                                     # accessibility tree
5. browser_diag_start()                                   # deep diagnostics
6. ... reproduce ...
7. browser_diag_stop()                                    # full report
8. browser_diag_console()                                 # JS errors
9. browser_diag_network()                                 # failing requests
```

### Pattern 6: Mock API Responses

```
1. browser_navigate("https://app.example.com")
2. browser_mock_response("/api/data", '{"status": "mocked", "data": []}')
3. browser_reload()                                       # triggers mocked API
4. browser_snapshot()                                     # verify mock displayed
```

### Pattern 7: Multi-User Testing

```
1. browser_create_session("admin")
2. browser_switch_session("admin")
3. browser_navigate("https://app.example.com/admin")
4. ... admin workflow ...

5. browser_create_session("user")
6. browser_switch_session("user")
7. browser_navigate("https://app.example.com")
8. ... user workflow ...

9. browser_list_sessions()   # verify both active
```

### Pattern 8: Responsive Design Testing

```
1. browser_set_viewport(375, 812)     # iPhone X
2. browser_navigate(url)
3. browser_screenshot()
4. browser_set_viewport(768, 1024)    # iPad
5. browser_navigate(url)
6. browser_screenshot()
7. browser_set_viewport(1920, 1080)   # Desktop
8. browser_navigate(url)
9. browser_screenshot()
```

### Pattern 9: Cross-Origin Iframes (OOPIF)

```
1. browser_snapshot_full_oopif()      # pierces all frames
2. browser_list_frames()              # find the iframe
3. browser_click_in_frame("iframe[src*='payment']", "button#pay")
4. browser_eval_in_frame("iframe[src*='payment']", "document.title")
```

### Pattern 10: Screen Recording for Bug Reports

```
1. browser_screen_record_start(filename="repro.mp4")
2. ... reproduce the bug ...
3. browser_screen_record_stop()       # returns path to mp4
4. browser_screenshot()               # final state
5. sin_browser_findings()             # CDP evidence
```

---

## Decision Tree: Which Tool to Use

| Situation | Use |
|-----------|-----|
| Need to see what's on the page | `browser_snapshot()` |
| Need to click something | `browser_click()` or `browser_click_by_text()` |
| Need to fill a form | `browser_fill()` (vanilla) or `browser_fill_react()` (React) |
| Need to check if element exists | `browser_assert()` |
| Need to wait for content | `browser_wait_for()` or `browser_wait_for_text()` |
| Need to extract data | `browser_get_text()`, `browser_extract_table()`, `browser_extract_list()` |
| Need visual proof | `browser_screenshot()` or `browser_pdf()` |
| Need to debug failures | `browser_diag_start/stop` + `sin_browser_findings()` |
| Need to mock an API | `browser_mock_response()` |
| Need multiple users | `browser_create_session()` |
| Need to test responsive | `browser_set_viewport()` |
| Need to handle iframes | `browser_snapshot_full_oopif()` + `browser_click_in_frame()` |
| Need to learn from runs | `browser_playbook_record()` |
| Need CDP evidence log | `sin_browser_navigate()` → `sin_browser_findings()` |
| Need before/after diff | `sin_browser_navigate(save_baseline=true)` → fix → `sin_browser_diff()` |

---

## Golden Rules

### DO

1. **Always** `browser_snapshot()` before clicking on tricky pages
2. **Always** `browser_wait_for_load()` after navigation
3. **Always** `browser_screenshot()` after critical actions
4. **Always** use `browser_diag_start()` when debugging failures
5. **Always** use `browser_set_viewport()` for consistent rendering
6. **Always** use `browser_fill_react()` for React forms
7. **Always** use `browser_snapshot_full_oopif()` for cross-origin iframes
8. **Always** record playbooks for repeatable flows

### DON'T

1. Don't assume selectors — use `browser_snapshot()` to get @eN refs
2. Don't skip waits — pages take time to load
3. Don't hardcode paths — use `browser_get_url()` to confirm
4. Don't click without verifying — screenshot first
5. Don't use `browser_type()` when `browser_fill()` works (fill is faster)
6. Don't forget `browser_wait_for_load("networkidle")` after SPA navigation
7. Don't bypass diagnostics — `browser_diag_start()` saves time

---

## Tool Name Schema (CRITICAL)

MCP tool names MUST match `^[a-zA-Z0-9_-]{1,64}$`. Always use **underscore**
form (`browser_click`), never slash (`browser/click`). Slash form is rejected
by Claude Desktop, Cursor, and Cline and silently disables every tool.

---

## Verification Checklist

- [ ] `browser_snapshot()` returns @eN refs for interactive elements
- [ ] `browser_fill()` successfully populates form inputs
- [ ] `browser_click()` triggers expected navigation or state change
- [ ] `browser_screenshot()` returns Base64 PNG
- [ ] `browser_assert()` returns `ok: true` for expected conditions
- [ ] `sin_browser_findings()` returns 0 errors (or known/acceptable ones)
- [ ] `sin_browser_diff()` shows resolved findings after a fix
- [ ] `browser_mock_response()` intercepts and returns mocked data
- [ ] `browser_create_session()` creates isolated context
- [ ] `browser_diag_stop()` generates complete report

---

## Safety

- Respect `robots.txt`
- Do not submit sensitive credentials — recording captures all network traffic
- Avoid automated account creation unless explicitly allowed
- Use `browser_block_resources()` to avoid loading unnecessary tracking scripts
- Use isolated sessions for untrusted content

---

## References

- SIN-Browser-Tools repo: `OpenSIN-Code/SIN-Browser-Tools`
- Builtin CDP tools: `cmd/sin-code/chat_tools_browser.go`
- Permission defaults: `cmd/sin-code/internal/permission_defaults.go`
- MCP server config: `~/.config/opencode/opencode.json`
