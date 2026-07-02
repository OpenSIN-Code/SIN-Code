# Browser Automation Prompt Templates

## Test Login Flow

```
Test the login flow at {url}:
1. Navigate to the login page
2. Fill in email: {email}
3. Fill in password: {password}
4. Click the submit button
5. Wait for navigation to complete
6. Verify the URL contains "/dashboard"
7. Take a screenshot as proof
8. Then test with invalid credentials:
   - Email: wrong@example.com
   - Password: wrongpassword
9. Verify error message appears
10. Take a screenshot of the error state
```

## Scrape a Page

```
Scrape the data from {url}:
1. Navigate to the page
2. Wait for the content to load
3. Extract all {element_type} elements
4. For each element, get: {fields}
5. Return the data as structured JSON
```

## Debug a Broken Page

```
Debug why {url} is broken:
1. Navigate to the page
2. Capture CDP evidence (sin_browser_navigate)
3. Check findings for errors (sin_browser_findings)
4. Take a screenshot of the current state
5. Check the console for JS errors
6. Check the network for failed requests
7. Inspect the DOM for missing elements
8. Report all findings
```

## Test Responsive Design

```
Test responsive design at {url}:
1. Set viewport to iPhone X (375x812)
2. Navigate and screenshot
3. Set viewport to iPad (768x1024)
4. Navigate and screenshot
5. Set viewport to Desktop (1920x1080)
6. Navigate and screenshot
7. Compare layouts at each breakpoint
```

## Test a Form

```
Test the form at {url}:
1. Navigate to the page
2. Snapshot to find form elements
3. Fill each field:
   {field_list}
4. Submit the form
5. Wait for response
6. Verify success/error state
7. Take a screenshot
```

## Mock an API

```
Test error handling by mocking the API:
1. Navigate to {url}
2. Mock {api_endpoint} to return {mock_response} with status {status_code}
3. Trigger the API call (click/reload)
4. Verify the UI shows the error state
5. Take a screenshot
6. Clean up the mock
7. Verify the UI recovers
```
