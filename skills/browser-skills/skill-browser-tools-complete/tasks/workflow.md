# Browser Automation Workflows — sin-browser-tools

## Workflow 1: Test a Complete Webapp Login Flow

**Goal**: Verify login works end-to-end with valid and invalid credentials.

### Steps

PHASE 1: Valid Login
1. `browser_navigate("https://app.example.com/login")`
2. `browser_wait_for_load("networkidle")`
3. `browser_snapshot()` — get form refs
4. `browser_fill("input[name='email']", "user@example.com")`
5. `browser_fill("input[name='password']", "validpassword")`
6. `browser_click("button[type='submit']")`
7. `browser_wait_for_load("networkidle")`
8. `browser_assert(url_contains="/dashboard")`
9. `browser_screenshot()` — proof of success

PHASE 2: Invalid Login
10. `browser_navigate("https://app.example.com/login")`
11. `browser_snapshot()`
12. `browser_fill("input[name='email']", "wrong@example.com")`
13. `browser_fill("input[name='password']", "wrongpassword")`
14. `browser_click("button[type='submit']")`
15. `browser_wait_for_text("Invalid credentials")`
16. `browser_assert(text_contains="Invalid credentials")`
17. `browser_screenshot()` — proof of error handling

### Verification
- Valid login redirects to dashboard
- Invalid login shows error message
- No JS errors in console
- No network failures

---

## Workflow 2: Scrape Product Data from an E-Commerce Site

**Goal**: Extract product names, prices, and images from a listing page.

### Steps

1. `browser_navigate("https://shop.example.com/products")`
2. `browser_wait_for(".product-card")`
3. `browser_set_viewport(1920, 1080)` — full desktop view
4. `browser_get_links()` — all product URLs
5. For each product URL:
   - `browser_navigate(product_url)`
   - `browser_wait_for_load("networkidle")`
   - `browser_get_text(".product-description")` — full description
   - `browser_get_images()` — all images
   - `browser_screenshot()` — visual proof

### Verification
- Extracted data matches visible products
- No 404 errors in network
- All images load successfully

---

## Workflow 3: Debug a Broken Page

**Goal**: Identify why a page shows a blank screen or errors.

### Steps

PHASE 1: CDP Evidence (builtin layer)
1. `sin_browser_navigate("https://broken.example.com")`
2. `sin_browser_findings()` — classified errors

PHASE 2: Playwright Inspection (external layer)
3. `browser_navigate("https://broken.example.com")`
4. `browser_wait_for_load("networkidle")`
5. `browser_snapshot()` — accessibility tree
6. `browser_get_text()` — visible text
7. `browser_get_html()` — raw HTML

PHASE 3: Deep Diagnostics
8. `browser_diag_start(scope="all")`
9. `browser_reload()` — re-trigger
10. `browser_diag_stop()` — full report
11. `browser_diag_console()` — JS errors
12. `browser_diag_network()` — failing requests
13. `browser_screenshot()` — visual state

PHASE 4: Fix Verification
14. `sin_browser_navigate(url, save_baseline="true")`
15. Apply fix
16. `sin_browser_navigate(url)`
17. `sin_browser_diff()` — before/after comparison

### Verification
- Findings count decreased after fix
- No JS exceptions in console
- All API requests return 200
- Page renders correctly (screenshot)

---

## Workflow 4: Test Responsive Design

**Goal**: Verify UI works at mobile, tablet, and desktop breakpoints.

### Steps

1. `browser_set_viewport(375, 812)` — iPhone X
2. `browser_navigate(url)`
3. `browser_screenshot()` — mobile view
4. `browser_snapshot()` — check mobile layout
5. `browser_set_viewport(768, 1024)` — iPad
6. `browser_navigate(url)`
7. `browser_screenshot()` — tablet view
8. `browser_set_viewport(1920, 1080)` — Desktop
9. `browser_navigate(url)`
10. `browser_screenshot()` — desktop view

### Verification
- Layout adapts correctly at each breakpoint
- No horizontal scroll on mobile
- Touch targets are large enough on mobile
- Navigation works at all sizes

---

## Workflow 5: Multi-User Testing

**Goal**: Test a multi-user workflow (admin + regular user).

### Steps

1. `browser_create_session("admin")`
2. `browser_switch_session("admin")`
3. `browser_navigate("https://app.example.com/admin")`
4. `browser_snapshot()`
5. `browser_fill("input#email", "admin@example.com")`
6. `browser_fill("input#password", "adminpass")`
7. `browser_click("button[type='submit']")`
8. `browser_wait_for_load("networkidle")`

9. `browser_create_session("user")`
10. `browser_switch_session("user")`
11. `browser_navigate("https://app.example.com")`
12. `browser_snapshot()`
13. `browser_fill("input#email", "user@example.com")`
14. `browser_fill("input#password", "userpass")`
15. `browser_click("button[type='submit']")`
16. `browser_wait_for_load("networkidle")`

17. `browser_list_sessions()` — verify both active

### Verification
- Both sessions are independent (separate cookies/storage)
- Admin sees admin dashboard
- User sees user dashboard
- No session leaking

---

## Workflow 6: Mock API and Test Error Handling

**Goal**: Test how the frontend handles backend failures.

### Steps

1. `browser_navigate("https://app.example.com")`
2. `browser_mock_response("/api/users", '{"error": "Server Error"}', status=500)`
3. `browser_reload()` — triggers mocked API
4. `browser_wait_for_text("Error")` — UI shows error state
5. `browser_screenshot()` — proof of error handling
6. `browser_unroute_all()` — clean up mocks
7. `browser_reload()` — back to normal
8. `browser_wait_for_load("networkidle")`
9. `browser_screenshot()` — proof of recovery

### Verification
- UI shows error message when API fails
- UI recovers when API is restored
- No unhandled promise rejections

---

## Workflow 7: Test Cross-Origin Iframes (OOPIF)

**Goal**: Interact with payment forms inside cross-origin iframes.

### Steps

1. `browser_navigate("https://shop.example.com/checkout")`
2. `browser_snapshot_full_oopif()` — pierces all frames
3. `browser_list_frames()` — find payment iframe
4. `browser_click_in_frame("iframe[src*='stripe']", "input#card-number")`
5. `browser_type_in_frame("iframe[src*='stripe']", "4242424242424242")`
6. `browser_click_in_frame("iframe[src*='stripe']", "button#submit-payment")`
7. `browser_wait_for_text("Payment successful")`
8. `browser_screenshot()`

### Verification
- Payment form inside iframe is accessible
- Form submission works through iframe boundary
- Success message appears after payment

---

## Workflow 8: Screen Recording for Bug Reports

**Goal**: Record a video reproduction of a bug for a bug report.

### Steps

1. `browser_screen_record_start(filename="bug-repro.mp4")`
2. `browser_navigate("https://app.example.com/buggy-feature")`
3. `browser_wait_for_load("networkidle")`
4. `browser_snapshot()`
5. `browser_click("@trigger-element")`
6. `browser_wait_for_text("Error")`
7. `browser_screenshot()` — final state
8. `browser_screen_record_stop()` — returns path to mp4
9. `sin_browser_findings()` — CDP evidence

### Verification
- Recording captures the full reproduction steps
- Video shows the error occurring
- CDP findings match the visual evidence

---

## Workflow 9: Capture PDF for Documentation

**Goal**: Generate a PDF of a page for documentation or approval.

### Steps

1. `browser_navigate("https://docs.example.com/page")`
2. `browser_wait_for_load("networkidle")`
3. `browser_pdf()` — render to PDF
4. Save the Base64 PDF output to file

### Verification
- PDF contains all page content
- PDF is properly formatted
- Images and tables are included
