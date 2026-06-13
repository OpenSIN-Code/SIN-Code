# `sca.doc.md` — Go-native SCA package

Go-native software composition analysis for Go projects, intended as a
peer to the Python `sin security sca` wrapper.

## What it does

- **Detects Go projects** by the presence of `go.mod`.
- **Parses `go.mod` natively** using `golang.org/x/mod/modfile` and returns
  direct and indirect dependencies.
- **Invokes `grype` as a subprocess** with `-o json` and parses the JSON report
  into a stable `Vulnerability` model.
- **Returns a summary** with severity counts and the number of packages scanned.

## Files that import / touch it

- `cmd/sin-code/internal/security.go` — can optionally call `sca.Scanner` for
  Go projects (added behind the existing `grype` tool runner).
- `cmd/sin-code/internal/security/sca/sca_test.go` — unit tests.

## Important config values & limits

- Grype binary name/path configurable via `Scanner` / `GrypeClient.Path`.
- If grype is not on PATH, the scanner returns parsed packages with zero
  vulnerabilities instead of failing.
- Severity normalization: `critical`, `high`, `medium`, `low`, `negligible`,
  `unknown`.

## Usage examples

```go
scanner := sca.New()
result, err := scanner.Scan(ctx, "./my-go-project")
// result.Vulnerabilities, result.Summary, result.PackagesScanned
```

## Known caveats / footguns

- Grype must be installed separately; the package does not download it.
- Grype scans the whole directory, so transitive dependencies found in the
  module cache are reported even if they are not listed in `go.mod`.
- The package only targets Go today; other ecosystems are out of scope for
  issue #41.
