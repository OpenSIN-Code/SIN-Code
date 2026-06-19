// SPDX-License-Identifier: MIT
// Purpose: VisualProcessor — visual understanding for autonomous goals
// (issue #386). Accepts screenshot/diagram image data, base64-encodes it,
// and sends it to a multimodal vision API via a pluggable HTTP transport.
// DescribeImage returns a natural-language description; ExtractDiagram
// returns structured diagram elements parsed from the vision response.
// The transport is injectable (HTTPDoer) so tests never touch the network
// (M2: no CGO, stdlib-only; M7: stateless methods are race-safe).
package autonomy

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// ImageInput holds raw image bytes, their format (e.g. "png", "jpeg"), and
// the pre-computed base64 encoding suitable for multimodal API payloads.
type ImageInput struct {
	Data   []byte
	Format string
	Base64 string
}

// DiagramElement is a single geometric/semantic element extracted from a
// diagram image. Coordinates are normalized to a 0–1000 canvas.
type DiagramElement struct {
	Type   string
	Label  string
	X      float64
	Y      float64
	Width  float64
	Height float64
}

// DiagramResult is the structured output of ExtractDiagram. Text is the
// raw vision description; Elements is the parsed structured list; Format
// is the detected diagram type (e.g. "flowchart", "sequence", "arch").
type DiagramResult struct {
	Text     string
	Elements []DiagramElement
	Format   string
}

// VisualProcessor encodes images and queries a vision API. It is stateless
// aside from the injectable transport and is safe for concurrent use (M7).
type VisualProcessor struct {
	transport HTTPDoer
	endpoint  string
}

// defaultVisionEndpoint is the vision API URL used when no override is
// configured. It can be set via the SIN_VISION_ENDPOINT environment
// variable so deployments can point at any OpenAI-compatible endpoint.
const defaultVisionEndpoint = "https://api.openai.com/v1/chat/completions"

// NewVisualProcessor creates a VisualProcessor backed by http.DefaultClient.
// The endpoint is read from the SIN_VISION_ENDPOINT env var, falling back to
// the default OpenAI-compatible chat completions URL.
func NewVisualProcessor() *VisualProcessor {
	endpoint := os.Getenv("SIN_VISION_ENDPOINT")
	if endpoint == "" {
		endpoint = defaultVisionEndpoint
	}
	return &VisualProcessor{
		transport: http.DefaultClient,
		endpoint:  endpoint,
	}
}

// EncodeImage base64-encodes the given image data and returns an ImageInput
// ready to be passed to DescribeImage or ExtractDiagram.
func (p *VisualProcessor) EncodeImage(data []byte, format string) ImageInput {
	return ImageInput{
		Data:   data,
		Format: format,
		Base64: base64.StdEncoding.EncodeToString(data),
	}
}

// visionRequest is the JSON payload sent to the vision API.
type visionRequest struct {
	Model    string          `json:"model"`
	Messages []visionMessage `json:"messages"`
}

type visionMessage struct {
	Role    string         `json:"role"`
	Content []visionContent `json:"content"`
}

type visionContent struct {
	Type     string          `json:"type"`
	Text     string          `json:"text,omitempty"`
	ImageURL *visionImageURL `json:"image_url,omitempty"`
}

type visionImageURL struct {
	URL string `json:"url"`
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

// DescribeImage sends the image and a prompt to the vision API and returns
// the natural-language description from the model. The model name is read
// from the SIN_VISION_MODEL env var, defaulting to "gpt-4o".
func (p *VisualProcessor) DescribeImage(ctx context.Context, img ImageInput, prompt string) (string, error) {
	if len(img.Data) == 0 {
		return "", fmt.Errorf("visual_input: image data is empty")
	}
	if img.Base64 == "" {
		img.Base64 = base64.StdEncoding.EncodeToString(img.Data)
	}
	if prompt == "" {
		prompt = "Describe this image in detail."
	}

	model := os.Getenv("SIN_VISION_MODEL")
	if model == "" {
		model = "gpt-4o"
	}

	mimeType := "image/" + img.Format
	if img.Format == "" {
		mimeType = "image/png"
	}

	body := visionRequest{
		Model: model,
		Messages: []visionMessage{
			{
				Role: "user",
				Content: []visionContent{
					{Type: "text", Text: prompt},
					{Type: "image_url", ImageURL: &visionImageURL{
						URL: "data:" + mimeType + ";base64," + img.Base64,
					}},
				},
			},
		},
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("visual_input: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("visual_input: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key := os.Getenv("SIN_VISION_API_KEY"); key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}

	resp, err := p.transport.Do(req)
	if err != nil {
		return "", fmt.Errorf("visual_input: transport: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("visual_input: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("visual_input: API returned %d: %s", resp.StatusCode, string(raw))
	}

	var vresp visionResponse
	if err := json.Unmarshal(raw, &vresp); err != nil {
		return "", fmt.Errorf("visual_input: unmarshal response: %w", err)
	}
	if vresp.Error != nil {
		return "", fmt.Errorf("visual_input: API error: %s", vresp.Error.Message)
	}
	if len(vresp.Choices) == 0 {
		return "", fmt.Errorf("visual_input: no choices in response")
	}

	return strings.TrimSpace(vresp.Choices[0].Message.Content), nil
}

// diagramExtractionPrompt instructs the model to describe diagram elements
// in a parseable line format: TYPE|LABEL|X|Y|W|H.
const diagramExtractionPrompt = `Analyze this diagram. First state the diagram type on a line starting with "FORMAT:". Then list each element on its own line as: TYPE|LABEL|X|Y|W|H where coordinates are floats 0-1000. Example: box|Start|100|50|200|80`

// ExtractDiagram sends the image to the vision API with a diagram-extraction
// prompt and parses the response into a structured DiagramResult.
func (p *VisualProcessor) ExtractDiagram(ctx context.Context, img ImageInput) (*DiagramResult, error) {
	text, err := p.DescribeImage(ctx, img, diagramExtractionPrompt)
	if err != nil {
		return nil, err
	}

	result := &DiagramResult{
		Text:   text,
		Format: "unknown",
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "FORMAT:") {
			result.Format = strings.TrimSpace(strings.TrimPrefix(line, "FORMAT:"))
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) != 6 {
			continue
		}
		el := DiagramElement{
			Type:  strings.TrimSpace(parts[0]),
			Label: strings.TrimSpace(parts[1]),
		}
		fmt.Sscanf(parts[2], "%f", &el.X)
		fmt.Sscanf(parts[3], "%f", &el.Y)
		fmt.Sscanf(parts[4], "%f", &el.Width)
		fmt.Sscanf(parts[5], "%f", &el.Height)
		if el.Type != "" {
			result.Elements = append(result.Elements, el)
		}
	}

	return result, nil
}
