// SPDX-License-Identifier: MIT
// Purpose: install — release-resolution subsystem for `sin-code install`
// (issue #170). Owns (1) platform detection + asset naming that mirrors
// goreleaser, (2) the typed GitHub Release struct, and (3) the canonical
// repository + binary / shim URLs so the install subcommand and the
// root install.sh / install.ps1 shims agree on the source of truth.
package install

import (
	"fmt"
	"runtime"
)

// Repo is the canonical GitHub repo (owner/name) for the unified
// `sin-code` binary. Both the Go install subcommand and the root
// install.sh / install.ps1 shims hard-code this string — changing the
// GitHub org without updating every shim is a quiet break, so keep
// the constant exported and DRY across packages.
const Repo = "OpenSIN-Code/SIN-Code"

// LatestReleaseURL is the endpoint used by both the Go subcommand and
// the install.sh shim to resolve the most recent published release.
// Stable URL — does not require `gh`, `jq`, or any GitHub auth.
const LatestReleaseURL = "https://api.github.com/repos/" + Repo + "/releases/latest"

// TagReleaseURL is the prefix for fetching a specific release by tag.
// The Go installer uses this when the caller pins a release with `--release`.
const TagReleaseURL = "https://api.github.com/repos/" + Repo + "/releases/tags/"

// DownloadBaseURL is the prefix used by the `/releases/latest/download/<asset>`
// pattern. GitHub redirects this to the latest published matching asset
// without requiring the response to be tagged "latest".
const DownloadBaseURL = "https://github.com/" + Repo + "/releases/latest/download/"

// Release mirrors the subset of the GitHub Releases JSON response we
// care about. Asset.URL is browser_download_url — the same URL the
// existing internal/self-update.go uses, so a single archive format
// serves both the in-place self-update flow and the from-scratch
// `sin-code install` flow.
type Release struct {
	TagName   string  `json:"tag_name"`
	Name      string  `json:"name"`
	Published string  `json:"published_at"`
	Body      string  `json:"body"`
	Assets    []Asset `json:"assets"`
}

// Asset describes a single downloadable file attached to a release.
type Asset struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	BrowserURL  string `json:"browser_download_url"`
	ContentType string `json:"content_type"`
}

// Platform reports the OS / arch pair the running binary was compiled
// for. It exists so install_test.go can swap the value via an injection
// hook without rebuilding the binary on every test machine.
type Platform struct {
	GOOS   string
	GOARCH string
}

// CurrentPlatform returns the host platform.
func CurrentPlatform() Platform {
	return Platform{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
}

// AssetName returns the goreleaser-shaped archive name for the given
// platform (e.g. `sin-code-linux-amd64.tar.gz`,
// `sin-code-windows-amd64.zip`). The naming MUST match .goreleaser.yaml
// (line 8-13: builds → goos × goarch; line 32-35: archives → tar.gz
// with a zip override on windows) — divergence here silently breaks
// every download path.
func (p Platform) AssetName() string {
	if p.GOOS == "windows" {
		return fmt.Sprintf("sin-code-%s-%s.zip", p.GOOS, p.GOARCH)
	}
	return fmt.Sprintf("sin-code-%s-%s.tar.gz", p.GOOS, p.GOARCH)
}

// BinaryName is the executable file name inside the archive.
// Windows uses `.exe` regardless of goreleaser rename.
func (p Platform) BinaryName() string {
	if p.GOOS == "windows" {
		return "sin-code.exe"
	}
	return "sin-code"
}

// DownloadURL returns the direct download URL for the platform's
// archive asset. Uses the stable /releases/latest/download/<asset>
// pattern — GitHub returns a 302 redirect to the latest published
// matching file. No auth, no jq, no gh binary required.
func (p Platform) DownloadURL() string {
	return DownloadBaseURL + p.AssetName()
}

// ChecksumFileName mirrors goreleaser's name_template (line 42 of
// .goreleaser.yaml): exactly `checksums.txt` at the archive root.
const ChecksumFileName = "checksums.txt"

// CosignSignatureName mirrors goreleaser's signs stanza (line 56 of
// .goreleaser.yaml): the signed artifact is `checksums.txt` so the
// signature + certificate live alongside it. Both are opt-in; nil
// values in VerifyOpts disable the check.
const (
	CosignSignatureName   = "checksums.txt.sig"
	CosignCertificateName = "checksums.txt.pem"
)

// ShimBaseURL is the raw.githubusercontent.com prefix used by the
// install.sh / install.ps1 shims at the repo root. We hard-link to
// `main` here because (1) the shims are short-lived thin wrappers and
// a stable tag-pin would require a coordinated goreleaser + shim-tag
// dance on every release, and (2) caveman (the reference install
// pattern) does the same. The trade-off is documented in the doc
// siblings next to install.sh / install.ps1.
const (
	ShimBaseURL = "https://raw.githubusercontent.com/" + Repo + "/main/"
	InstallSh   = "install.sh"
	InstallPS1  = "install.ps1"
)
