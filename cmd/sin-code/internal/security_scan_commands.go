// SPDX-License-Identifier: MIT
// Purpose: consolidated security scan subcommands for `sin-code security scan`.
//
// This file was the original monolith (1655 lines) that held the container,
// SAST, SCA, and SBOM scan constructors and their helpers. It has been split
// into focused implementation files:
//
//   - container_scan_impl.go — container image/path scan subcommand + helpers
//   - sast_scan_impl.go      — SAST scan subcommand + binary locator + runner
//   - sca_scan_impl.go       — SCA scan subcommand + binary locator + runner
//   - sbom_scan_impl.go      — SBOM generation subcommand + dependency parsers
//
// Secrets scanning lives in security_secrets.go; SARIF output helpers live in
// security_sarif.go. The SecurityFinding type and shared helpers (fileExists,
// binaryExt) are defined in their respective existing files.
//
// No shared types remain in this file — each scan type's structs are declared
// in their own implementation file and are accessible package-wide.
//
// Docs: security.doc.md
package internal
