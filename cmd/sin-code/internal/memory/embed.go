// SPDX-License-Identifier: MIT
// Purpose: text embedding via NVIDIA NIM. Uses nv-embed-v1 (or any
// OpenAI-compatible /v1/embeddings endpoint).
package memory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const NIMEmbedBaseURL = "https://integrate.api.nvidia.com/v1"
const NIMEmbedModel = "nvidia/nv-embed-v1"

type EmbedRequest struct {
	Input []string `json:"input"`
	Model string   `json:"model"`
}

type EmbedResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

type Embedder struct {
	BaseURL string
	Model   string
	APIKey  string
	HTTP    *http.Client
}

func NewNIMEmbedder() *Embedder {
	return &Embedder{
		BaseURL: getenv("SIN_NIM_BASE_URL", NIMEmbedBaseURL),
		Model:   getenv("SIN_NIM_EMBED_MODEL", NIMEmbedModel),
		APIKey:  os.Getenv("SIN_NIM_API_KEY"),
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

func (e *Embedder) EmbedOne(ctx context.Context, text string) ([]float32, error) {
	results, err := e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, errors.New("no embedding returned")
	}
	return results[0], nil
}

func (e *Embedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if e.APIKey == "" {
		return nil, errors.New("no API key (set SIN_NIM_API_KEY)")
	}
	req := EmbedRequest{Input: texts, Model: e.Model}
	body, _ := json.Marshal(req)
	httpReq, err := http.NewRequestWithContext(ctx, "POST", e.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+e.APIKey)
	resp, err := e.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		data, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings API error %d: %s", resp.StatusCode, string(data))
	}
	var out EmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Data) != len(texts) {
		return nil, fmt.Errorf("expected %d embeddings, got %d", len(texts), len(out.Data))
	}
	results := make([][]float32, len(out.Data))
	for _, d := range out.Data {
		if d.Index < 0 || d.Index >= len(results) {
			return nil, fmt.Errorf("bad embedding index %d", d.Index)
		}
		results[d.Index] = d.Embedding
	}
	return results, nil
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// SetupNIMEmbedder registers the NIM embedder as the default.
func SetupNIMEmbedder() {
	e := NewNIMEmbedder()
	if e.APIKey == "" {
		return
	}
	SetEmbedder(func(text string) ([]float32, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return e.EmbedOne(ctx, text)
	}, 4096)
}
