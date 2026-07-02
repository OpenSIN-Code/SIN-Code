# Browser Test Report Template

## Test: {test_name}
**Date**: {timestamp}
**URL**: {url}
**Browser**: Chromium (headless)

---

## Summary

| Metric | Value |
|--------|-------|
| Steps Executed | {step_count} |
| Passed | {passed} |
| Failed | {failed} |
| Screenshots | {screenshot_count} |
| Duration | {duration} |

---

## Steps

### Step 1: {step_1_name}
- **Action**: {action}
- **Selector**: {selector}
- **Expected**: {expected}
- **Actual**: {actual}
- **Status**: {PASS/FAIL}
- **Screenshot**: {screenshot_path}

### Step 2: {step_2_name}
...

---

## Findings (CDP Evidence)

| Category | Severity | Count | Title |
|----------|----------|-------|-------|
| {category} | {severity} | {count} | {title} |

---

## Console Errors

```
{console_errors}
```

## Network Failures

```
{network_failures}
```

---

## Screenshots

{screenshot_descriptions}

---

## Verdict

{verdict}
