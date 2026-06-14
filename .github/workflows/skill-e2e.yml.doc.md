What it does: Runs the E2E integration test that installs the `websearch` skill from GitHub and verifies the `sin-websearch` binary is built and registered.

Dependencies: Triggered on push to `main`, on PRs that touch `skillmgr` or `mcpclient`, and via `workflow_dispatch`. Requires Go 1.25.11 and network access to GitHub.

Important config:
- Runs `go test -tags=e2e ./cmd/sin-code/internal/skillmgr/ -run TestInstallWebsearchFromGitHub`.
- Builds `sin-code` and runs `sin-code mcp list` to verify the websearch server is registered.

Caveats:
- Slow (~1 minute) due to cloning and building `web_search_bundle` from source.
- Can fail due to external network issues.
