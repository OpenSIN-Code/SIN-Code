// SPDX-License-Identifier: MIT
// Purpose: native image analysis for sin-code. Reads an image file, base64-encodes
// it, and sends it to a vision-capable LLM via the existing provider config
// (llm.base_url / llm.api_key / llm.model). No Tesseract / OCR runtime
// dependency — issue #423.
//
// Configuration precedence (highest first):
//  1. SIN_ANALYSE_IMAGE_MODEL / SIN_ANALYSE_IMAGE_API_KEY / SIN_ANALYSE_IMAGE_BASE_URL env vars
//  2. llm.model / llm.api_key / llm.base_url from merged config
//  3. Built-in defaults: model=accounts/fireworks/models/minimax-m3 (vision-capable)
//
// The wire format is OpenAI-compatible chat completions with a multimodal
// user message (text + image_url). Any OpenAI-compatible endpoint that supports
// vision models works, including Fireworks AI, NIM, OpenAI, and Anthropic
// proxies.
package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// LLMConfig holds the LLM fields the vision package needs from the merged
// sin-code config. It is populated by a provider set from the parent package
// so the vision package stays import-cycle-free.
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// DefaultVisionModel is the default vision-capable model used when the
// operator has not configured llm.model or SIN_ANALYSE_IMAGE_MODEL.
// minimax-m3 is a vision-capable model on the Fireworks AI platform.
const DefaultVisionModel = "accounts/fireworks/models/minimax-m3"

// DefaultPrompt is the system/user prompt for general image analysis.
const DefaultPrompt = "Describe this image in detail. Include visible text, UI elements, layout, and any diagram or chart structure."

// AnalyzeResult is the structured output returned by AnalyzeImage.
type AnalyzeResult struct {
	Description string `json:"description"`
	Model       string `json:"model"`
	Provider    string `json:"provider"`
}

// Config holds the runtime configuration for the vision analyzer. Production
// callers build it via internal.VisionConfigFromEnv(); tests override fields
// directly.
type Config struct {
	BaseURL string
	APIKey  string
	Model   string
	Prompt  string
	HTTP    *http.Client
}

// Validate returns a user-friendly error if the config cannot call a vision
// API. Missing API keys are allowed only for local providers (e.g. ollama);
// an empty base URL is always rejected.
func (c Config) Validate() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return fmt.Errorf("no base URL configured; set llm.base_url or SIN_ANALYSE_IMAGE_BASE_URL")
	}
	if strings.TrimSpace(c.Model) == "" {
		return fmt.Errorf("no model configured; set llm.model or SIN_ANALYSE_IMAGE_MODEL")
	}
	if strings.TrimSpace(c.APIKey) == "" && !isLocalProvider(c.BaseURL) {
		return fmt.Errorf("no API key configured; set llm.api_key or SIN_ANALYSE_IMAGE_API_KEY")
	}
	return nil
}

func isLocalProvider(baseURL string) bool {
	return strings.Contains(baseURL, "127.0.0.1") || strings.Contains(baseURL, "localhost")
}

// AnalyzeImageWithConfig is the canonical entry point: callers supply a fully
// wired Config including a mock HTTP client.
func AnalyzeImageWithConfig(ctx context.Context, path string, cfg Config) (*AnalyzeResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("analyze image: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	data, format, err := readImage(path)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	mimeType := imageMimeType(format, path)

	resp, err := callVisionAPI(ctx, cfg, mimeType, b64)
	if err != nil {
		return nil, err
	}

	return &AnalyzeResult{
		Description: resp,
		Model:       cfg.Model,
		Provider:    cfg.BaseURL,
	}, nil
}

// readImage loads the image bytes and detects the image format from the file
// extension (fallback: image/png). It does not validate magic bytes — the
// vision provider will reject unsupported formats.
func readImage(path string) ([]byte, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("read image: %w", err)
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("read image: file is empty")
	}
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if ext == "" {
		return data, "png", nil
	}
	return data, ext, nil
}

func imageMimeType(format, path string) string {
	format = strings.ToLower(strings.TrimPrefix(format, "."))
	switch format {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "tiff", "tif":
		return "image/tiff"
	case "heic":
		return "image/heic"
	default:
		// Best-effort based on extension; providers generally accept image/png.
		return "image/png"
	}
}

// visionContent is a single multimodal content part.
type visionContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
}

// visionMessage is the user message carrying the prompt and image.
type visionMessage struct {
	Role    string          `json:"role"`
	Content []visionContent `json:"content"`
}

// visionRequest is the JSON payload sent to the vision API.
type visionRequest struct {
	Model     string          `json:"model"`
	Messages  []visionMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
}

// visionResponse is the JSON returned by the vision API.
type visionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// callVisionAPI sends the base64 image to the provider and returns the
// assistant's content string.
func callVisionAPI(ctx context.Context, cfg Config, mimeType, b64 string) (string, error) {
	body := visionRequest{
		Model: cfg.Model,
		Messages: []visionMessage{
			{
				Role: "user",
				Content: []visionContent{
					{Type: "text", Text: cfg.Prompt},
					{Type: "image_url", ImageURL: &visionImageURL{
						URL: "data:" + mimeType + ";base64," + b64,
					}},
				},
			},
		},
		MaxTokens: 4096,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal vision request: %w", err)
	}

	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("create vision request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	resp, err := cfg.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("vision API request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read vision response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("vision API error %d: %s", resp.StatusCode, string(raw))
	}

	var vresp visionResponse
	if err := json.Unmarshal(raw, &vresp); err != nil {
		return "", fmt.Errorf("decode vision response: %w", err)
	}
	if vresp.Error != nil && vresp.Error.Message != "" {
		return "", fmt.Errorf("vision API error: %s", vresp.Error.Message)
	}
	if len(vresp.Choices) == 0 {
		return "", fmt.Errorf("vision API returned no choices")
	}
	return strings.TrimSpace(vresp.Choices[0].Message.Content), nil
}
