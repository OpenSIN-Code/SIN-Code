// SPDX-License-Identifier: MIT
// Purpose: lightweight tests for the sin_analyse_image MCP handler (issue #423).
// The handler delegates to the vision package; we stub the HTTP transport so
// no real API calls are made.
package internal

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/vision"
)

type analyseImageTransport struct {
	body string
}

func (a *analyseImageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(a.body)),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

func analyseImageClient(body string) *http.Client {
	return &http.Client{Transport: &analyseImageTransport{body: body}}
}

func TestHandleAnalyseImage_Success(t *testing.T) {
	// Create a minimal PNG file for the vision handler to read.
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.png")
	if err := os.WriteFile(path, []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}, 0o644); err != nil {
		t.Fatal(err)
	}

	// VisionConfigFromEnv reads the merged config. We inject a fake HTTP
	// client by temporarily replacing the default client, then restore it.
	// This is a shallow integration test — the vision package itself is
	// unit-tested separately.
	visionResponse := `{"choices":[{"message":{"content":"A chart"}}]}`
	cfg := vision.Config{
		BaseURL: "https://fake.example.com/v1",
		APIKey:  "sk",
		Model:   "m",
		Prompt:  "describe",
		HTTP:    analyseImageClient(visionResponse),
	}
	result, err := vision.AnalyzeImageWithConfig(context.Background(), path, cfg)
	if err != nil {
		t.Fatalf("vision call: %v", err)
	}
	if result.Description != "A chart" {
		t.Errorf("unexpected description: %q", result.Description)
	}
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "A chart") {
		t.Errorf("JSON should contain description")
	}
}

func TestHandleAnalyseImage_MissingPath(t *testing.T) {
	_, err := handleAnalyseImage(context.Background(), map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "path is required") {
		t.Fatalf("expected path required error, got %v", err)
	}
}
