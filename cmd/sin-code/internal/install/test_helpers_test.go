// SPDX-License-Identifier: MIT
// Purpose: Test-only exports for the install package. Exposes a few
// `ForTest` symbols that let the external test suite (install_test)
// hermetically exercise the package without leaking test hooks into
// the production binary. Go's toolchain ignores *_test.go in normal
// builds, so these never ship.
package install

import (
	"context"
	"io"
	"net/http"
	"time"
)

// NewHTTPClientWithBaseURLForTest returns an HTTP client whose every
// outbound request is re-pointed at the given base URL. Used by the
// test suite so FetchLatest can run against an httptest server.
func NewHTTPClientWithBaseURLForTest(baseURL string) *http.Client {
	return &http.Client{
		Timeout:   30 * time.Second,
		Transport: rewriteTransport{base: baseURL},
	}
}

type rewriteTransport struct {
	base string
}

func (t rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite only the SCHEME + HOST (and re-use path). The package's
	// prod API URL is fixed to api.github.com ... — we swap it to the
	// test server URL so the production selector logic is the same
	// code path, just with a different host.
	if req.URL.Scheme != "" {
		req.URL.Scheme = "http"
	}
	req.URL.Host = stripHost(t.base)
	// Pass the path+query through unchanged.
	return http.DefaultTransport.RoundTrip(req)
}

func stripHost(baseURL string) string {
	// baseURL looks like http://127.0.0.1:43679 (no path). Trim
	// scheme so we can extract "host:port".
	if len(baseURL) > 7 && baseURL[:7] == "http://" {
		return baseURL[7:]
	}
	if len(baseURL) > 8 && baseURL[:8] == "https://" {
		return baseURL[8:]
	}
	return baseURL
}

// FetchLatestWithConfigForTest mirrors FetchLatest but uses the
// rewrite-transport client so requests hit the test server.
func FetchLatestWithConfigForTest(ctx context.Context, c *http.Client, p Platform) (*Release, error) {
	return FetchLatest(ctx, c, p)
}

// FetchAssetWithConfigForTest mirrors FetchAsset but uses the
// rewrite-transport client. The `tsURL` parameter is the test server
// root; empty means use the production DownloadBaseURL.
func FetchAssetWithConfigForTest(ctx context.Context, c *http.Client, p Platform, tsURL string) (path string, sha256hex string, err error) {
	if c == nil {
		c = NewHTTPClient()
	}
	url := p.DownloadURL()
	if tsURL != "" {
		url = tsURL + "/" + p.AssetName()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Accept", "application/octet-stream")
	resp, err := c.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", errFromStatus(resp)
	}
	sw := newSHA256Writer()
	out, err := writeToTempFile("", p.AssetName())
	if err != nil {
		return "", "", err
	}
	if _, err := io.Copy(io.MultiWriter(out, sw), resp.Body); err != nil {
		_ = out.Close()
		return "", "", err
	}
	if err := out.Close(); err != nil {
		return "", "", err
	}
	return out.Name(), sw.hex(), nil
}

func errFromStatus(resp *http.Response) error {
	return &httpErr{Status: resp.StatusCode, StatusText: resp.Status}
}

type httpErr struct {
	Status     int
	StatusText string
}

func (e *httpErr) Error() string { return e.StatusText }

// ParseChecksumsTxtForTest is a test-re-export of parseChecksumsTxt.
func ParseChecksumsTxtForTest(body string) map[string]string {
	return parseChecksumsTxt(body)
}

// VerifyFromFileForTest is a test-re-export of VerifyFromFile.
func VerifyFromFileForTest(path string, expectedHex string) (string, error) {
	return VerifyFromFile(path, expectedHex)
}

// HashOfForTest returns the SHA256 hex string of body. Test-only —
// production code paths use newSHA256Writer to stream-hash.
func HashOfForTest(body []byte) string {
	sw := newSHA256Writer()
	_, _ = sw.Write(body)
	return sw.hex()
}
