// SPDX-License-Identifier: MIT
// Stub — will be implemented by the agent that introduced sin_vision_image.
package vision

import (
	"context"
	"fmt"
)

// AnalyzeImage analyzes an image and returns a text description.
func AnalyzeImage(ctx context.Context, source string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("analyze image: %w", err)
	}
	// TODO: implement actual vision analysis using ctx for HTTP requests
	return "", nil
}
