# Makefile for SIN-Code-Bundle
# Default build: zero-deps, no tree-sitter. For full feature set: `make build`.

GO_TAGS ?= treesitter
BIN_DIR ?= bin

.PHONY: build build-minimal lint vuln release-snapshot test test-coverage clean

## build: default build WITH tree-sitter structural editing
build:
	go build -tags "$(GO_TAGS)" -o $(BIN_DIR)/sin-code ./cmd/sin-code

## build-minimal: zero-CGO fallback build without tree-sitter
build-minimal:
	CGO_ENABLED=0 go build -o $(BIN_DIR)/sin-code-minimal ./cmd/sin-code

## test: run the full test suite (hermetic — no network)
test:
	go test ./... -count=1

## test-coverage: emit coverage profile
test-coverage:
	go test ./... -coverprofile=coverage.out -count=1
	go tool cover -func=coverage.out

## lint: run golangci-lint (requires local install: https://golangci-lint.run)
lint:
	golangci-lint run --timeout=5m

## vuln: check for known vulnerabilities in dependencies
vuln:
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

## release-snapshot: dry-run the signed release pipeline locally
release-snapshot:
	goreleaser release --snapshot --clean --skip=sign,sbom

## clean: remove build artifacts
clean:
	rm -rf $(BIN_DIR) coverage.out dist/
