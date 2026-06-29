// SPDX-License-Identifier: MIT
package voice

import (
	"testing"
)

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	if opts.Duration != 10 {
		t.Errorf("Duration = %d, want 10", opts.Duration)
	}
	if opts.Language != "en" {
		t.Errorf("Language = %q, want en", opts.Language)
	}
	if opts.Model != "whisper-1" {
		t.Errorf("Model = %q, want whisper-1", opts.Model)
	}
}

func TestIsAvailable(t *testing.T) {
	_ = IsAvailable()
}
