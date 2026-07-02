# Browser Automation Standards — sin-browser-tools

## 1. The Snapshot-Act-Verify Loop

Every browser interaction MUST follow this three-step pattern:

```
SNAPSHOT → ACT → VERIFY
```

- **SNAPSHOT**: Call `browser_snapshot()` to see the page and get @eN refs
- **ACT**: Call the interaction tool (`browser_click`, `browser_fill`, etc.)
- **VERIFY**: Call `browser_snapshot()` again or `browser_screenshot()` to confirm

**Why**: DOM changes between snapshot and action. The @eN refs from the first
snapshot may be stale. Always re-snapshot after navigation or state changes.

## 2. Ref-IDs (@eN)

Every interactive element gets a stable ref-ID from `browser_snapshot()`:

```
@e1  [button]  "Submit"
@e2  [input]   "Email"
@e3  [link]    "Login"
```

Use @eN refs instead of CSS selectors when possible. They are:
- Stable across DOM changes
- OOPIF-safe (work across frames)
- Shadow DOM-piercing
- Human-readable

## 3. Wait Strategy

Never interact without waiting first. The wait hierarchy:

1. `browser_wait_for_load("networkidle")` — after navigation
2. `browser_wait_for("selector")` — for specific elements
3. `browser_wait_for_text("text")` — for dynamic content
4. `browser_wait_for_spa_transition()` — for SPA route changes
5. `browser_wait_for_response("/api/...")` — for API-dependent UI

**Timeout**: All wait tools have a default timeout. Override with `timeout_ms`
for slow operations.

## 4. Selector Priority

When you need a selector (and can't use @eN refs):

1. **data-testid** — most stable, designed for testing
2. **aria-label** — accessible, stable
3. **name attribute** — stable for forms
4. **id** — stable but may change
5. **CSS class** — fragile, may change with styling
6. **XPath** — last resort, breaks with DOM changes

## 5. React / SPA Forms

React-controlled inputs need special handling:

```bash
# WRONG — React state won't update
browser_fill("input#email", "test@example.com")

# CORRECT — fires React-compatible events
browser_fill_react("input[data-testid='email']", "test@example.com")
```

**Rule**: If the app uses React, Vue, or Angular, always use `*_react` variants.

## 6. Shadow DOM / OOPIF

Cross-origin iframes and Shadow DOM require special tools:

```bash
# See everything including frames
browser_snapshot_full_oopif()

# List all frames
browser_list_frames()

# Interact inside a frame
browser_click_in_frame("iframe[src*='payment']", "button#pay")

# Evaluate inside a frame
browser_eval_in_frame("iframe[src*='payment']", "document.title")
```

## 7. Evidence Capture

For debugging, always capture evidence before fixing:

```bash
# Layer 1: CDP evidence (builtin)
sin_browser_navigate(url, save_baseline="true")
sin_browser_findings()

# Layer 2: Playwright diagnostics (external)
browser_diag_start(scope="all")
... reproduce ...
browser_diag_stop()
browser_diag_console()
browser_diag_network()
```

## 8. Network Mocking

Mock APIs to test error handling and edge cases:

```bash
# Mock a 500 error
browser_mock_response("/api/users", '{"error": "Internal Server Error"}', status=500)

# Mock a slow response
browser_mock_response("/api/data", '{"data": []}', delay_ms=3000)

# Block tracking scripts
browser_block_resources(patterns=["**/analytics.js", "**/tracking.js"])

# Clean up
browser_unroute_all()
```

## 9. Multi-Session Testing

Test multi-user flows with isolated sessions:

```bash
browser_create_session("admin")
browser_create_session("user")

# Each session has independent cookies/storage
browser_switch_session("admin")
... admin actions ...

browser_switch_session("user")
... user actions ...
```

## 10. Responsive Testing

Test at canonical breakpoints:

| Device | Viewport | Preset |
|--------|----------|--------|
| iPhone X | 375×812 | `small` |
| iPad | 768×1024 | `medium` |
| Desktop HD | 1920×1080 | `large` |
| 4K | 2560×1440 | `full` |

```bash
browser_set_viewport(375, 812)  # or browser_set_window_mode("small")
browser_navigate(url)
browser_screenshot()
```

## 11. Screenshot Best Practices

- Take screenshots at key milestones (after login, after form submit, after error)
- Use `browser_screenshot_element()` for specific components
- Use `browser_pdf()` for printable output
- Use `browser_vision()` for LLM-powered visual analysis

## 12. Playbook Learning

Record successful automations for reuse:

```bash
# After a successful flow
browser_playbook_record(
    task="login",
    url="https://example.com/login",
    steps=["navigate", "fill email", "fill password", "click submit", "wait for dashboard"],
    success=true
)

# Before a new flow
browser_playbook_suggest(task="login", url="https://other.example.com/login")
```

## 13. Error Recovery

When an action fails:

1. Take a screenshot to see current state
2. Run `browser_diag_start()` to capture evidence
3. Re-snapshot to see what changed
4. Try alternative selectors or approaches
5. If stuck, use `browser_console()` to check for JS errors

## 14. Performance Considerations

- Use `browser_block_resources()` to skip images/fonts when testing logic
- Use `browser_wait_for_load("domcontentloaded")` instead of `"networkidle"` for faster loads
- Use `browser_set_viewport()` to avoid rendering overhead
- Close unused tabs with `browser_close_tab()`

## 15. Security

- Never log or store credentials in screenshots
- Use isolated sessions for untrusted URLs
- Clear cookies after testing with `browser_cookies_clear()`
- Respect rate limits and robots.txt
