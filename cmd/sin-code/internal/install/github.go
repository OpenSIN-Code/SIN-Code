// SPDX-License-Identifier: MIT
// Purpose: install — pure-stdlib GitHub Releases REST client. The
// bootstrap path (root install.sh) cannot rely on the `gh` CLI being
// installed yet (that would be a chicken-and-egg) and the in-binary
// install flow (~/.local/share/sin-code/install/*.go) cannot depend
// on internal/ghbridge for the same reason during a fresh install.
// stdlib + net/http is the only honest answer.
package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// NewHTTPClient returns an http.Client with a sane bootstrap timeout.
// The default Go client has no timeout — without one, a hung TLS
// handshake on a flaky network pins the user for minutes. 90s is
// generous enough to absorb cold-start DNS + TLS on cellular but short
// enough that an interactive `curl|bash` user never wonders if the
// installer died.
func NewHTTPClient() *http.Client {
	return &http.Client{Timeout: 90 * time.Second}
}

// FetchLatest resolves the latest published release for the canonical
// Repo via the GitHub REST API. Returns the Release with at least one
// matching platform asset, or an error suitable for surfacing to the
// user verbatim (don't wrap in jargon).
//
// ctx cancellation propagates — the install cmd uses a parent ctx so
// the cobra RunE can short-circuit on SIGINT.
func FetchLatest(ctx context.Context, client *http.Client, p Platform) (*Release, error) {
	return fetchReleaseByURL(ctx, client, p, LatestReleaseURL)
}

// FetchRelease resolves a specific published release by tag. It is used
// when the caller pins a release with `--release <tag>`.
func FetchRelease(ctx context.Context, client *http.Client, p Platform, tag string) (*Release, error) {
	if tag == "" {
		return nil, errors.New("install: release tag required")
	}
	return fetchReleaseByURL(ctx, client, p, TagReleaseURL+tag)
}

func fetchReleaseByURL(ctx context.Context, client *http.Client, p Platform, url string) (*Release, error) {
	if client == nil {
		client = NewHTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("install: build request: %w", err)
	}
	// GitHub requires a UA — the API rejects the default Go UA with a
	// 403. Send a stable, identifiable string so a leaked token in
	// logs is searchable.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sin-code-install/1.0 (+https://github.com/"+Repo+")")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("install: fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusForbidden && strings.Contains(resp.Header.Get("X-RateLimit-Remaining"), "0") {
		return nil, errors.New("install: GitHub API rate limit hit (unauthenticated)")
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("install: release not found at %s", url)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("install: GitHub API returned HTTP %d (%s)", resp.StatusCode, resp.Status)
	}
	// BodyReadAll is bounded by Content-Length when present so a
	// malicious upstream can't stream us into a 4 GB allocation.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, fmt.Errorf("install: read response: %w", err)
	}
	var rel Release
	if err := json.Unmarshal(body, &rel); err != nil {
		return nil, fmt.Errorf("install: parse release JSON: %w", err)
	}
	if rel.TagName == "" {
		return nil, errors.New("install: release JSON missing tag_name (unexpected payload)")
	}
	want := p.AssetName()
	for i := range rel.Assets {
		if rel.Assets[i].Name == want {
			return &rel, nil
		}
	}
	return nil, fmt.Errorf("install: no asset named %q in release %s (supported platform: %s/%s)", want, rel.TagName, p.GOOS, p.GOARCH)
}

// FetchAsset downloads one asset from a release to a temporary file
// and returns the file path together with the SHA256 of its bytes.
// The verification is computed locally so a network MITM cannot forge
// the size + checksum mismatch that's checked by Verify().
//
// If the platform-specific BrowserURL is empty (e.g. asset list
// revealed a download restriction), falls back to the stable
// /releases/latest/download/<asset> pattern.
func FetchAsset(ctx context.Context, client *http.Client, rel *Release, p Platform, destDir string) (path string, sha256hex string, err error) {
	if rel == nil {
		return "", "", errors.New("install: nil release")
	}
	if client == nil {
		client = NewHTTPClient()
	}
	if destDir == "" {
		destDir = "."
	}
	u := p.DownloadURL()
	for _, a := range rel.Assets {
		if a.Name == p.AssetName() && a.BrowserURL != "" {
			u = a.BrowserURL
			break
		}
	}
	// Validate URL early — users pasting random mirrors shouldn't get
	// a 30-second TLS timeout on a typo.
	pu, perr := url.Parse(u)
	if perr != nil || pu.Scheme == "" || pu.Host == "" {
		return "", "", fmt.Errorf("install: invalid asset URL %q", u)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", "", fmt.Errorf("install: build asset request: %w", err)
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "sin-code-install/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("install: fetch asset %s: %w", p.AssetName(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("install: asset HTTP %d (%s)", resp.StatusCode, resp.Status)
	}
	sw := newSHA256Writer()
	out, err := writeToTempFile(destDir, p.AssetName())
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(io.MultiWriter(out, sw), resp.Body); err != nil {
		_ = out.Close()
		return "", "", fmt.Errorf("install: stream asset to disk: %w", err)
	}
	if err := out.Close(); err != nil {
		return "", "", fmt.Errorf("install: close temp file: %w", err)
	}
	return out.Name(), sw.hex(), nil
}

// FetchChecksums downloads the goreleaser-style checksums.txt next to
// the archive. Returns nil with no error if the file is missing
// (pre-goreleaser releases, dev tags) — verification is opt-in, not
// load-bearing.
func FetchChecksums(ctx context.Context, client *http.Client, rel *Release) (map[string]string, error) {
	if client == nil {
		client = NewHTTPClient()
	}
	url := DownloadBaseURL + ChecksumFileName
	for _, a := range rel.Assets {
		if a.Name == ChecksumFileName && a.BrowserURL != "" {
			url = a.BrowserURL
			break
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "sin-code-install/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("install: checksums HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseChecksumsTxt(string(body)), nil
}
