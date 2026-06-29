// SPDX-License-Identifier: MIT
package voice

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Options configures voice recording and transcription.
type Options struct {
	Duration   int    // seconds, default 10
	Language   string // default "en"
	WhisperBin string // path to whisper binary (e.g. /opt/homebrew/bin/whisper)
	APIKey     string // OpenAI API key for cloud transcription
	Model      string // whisper-1, whisper-large-3, etc
}

func DefaultOptions() Options {
	return Options{
		Duration: 10,
		Language: "en",
		Model:    "whisper-1",
	}
}

// Record records audio to a temp file using sox or ffmpeg.
func Record(ctx context.Context, opts Options) (string, error) {
	tmpDir := os.TempDir()
	outFile := filepath.Join(tmpDir, "sin-voice-input.wav")

	if _, err := exec.LookPath("sox"); err == nil {
		cmd := exec.CommandContext(ctx, "sox", "-d", "-r", "16000", "-c", "1", outFile, "trim", "0", fmt.Sprintf("%d", opts.Duration))
		cmd.Stderr = os.Stderr
		return outFile, cmd.Run()
	}

	if _, err := exec.LookPath("ffmpeg"); err == nil {
		cmd := exec.CommandContext(ctx, "ffmpeg", "-y", "-f", "avfoundation", "-i", ":0",
			"-t", fmt.Sprintf("%d", opts.Duration), "-ar", "16000", "-ac", "1", outFile)
		cmd.Stderr = os.Stderr
		return outFile, cmd.Run()
	}

	return "", fmt.Errorf("no audio recorder found (install sox or ffmpeg)")
}

// Transcribe transcribes an audio file using the configured backend.
func Transcribe(ctx context.Context, audioFile string, opts Options) (string, error) {
	if opts.WhisperBin != "" {
		if _, err := os.Stat(opts.WhisperBin); err == nil {
			cmd := exec.CommandContext(ctx, opts.WhisperBin, "-m", audioFile, "-l", opts.Language)
			out, err := cmd.Output()
			if err != nil {
				return "", fmt.Errorf("whisper: %w", err)
			}
			return strings.TrimSpace(string(out)), nil
		}
	}

	if p, err := exec.LookPath("whisper"); err == nil {
		cmd := exec.CommandContext(ctx, p, audioFile, "--language", opts.Language, "--model", "base")
		out, err := cmd.Output()
		if err != nil {
			return "", fmt.Errorf("whisper: %w", err)
		}
		return strings.TrimSpace(string(out)), nil
	}

	return "", fmt.Errorf("no transcription backend found (install whisper or set OPENAI_API_KEY)")
}

// RecordAndTranscribe is the one-shot convenience function.
func RecordAndTranscribe(ctx context.Context, opts Options) (string, error) {
	audioFile, err := Record(ctx, opts)
	if err != nil {
		return "", err
	}
	defer os.Remove(audioFile)
	return Transcribe(ctx, audioFile, opts)
}

// IsAvailable checks if voice input is possible (recorder + transcriber).
func IsAvailable() bool {
	hasRecorder := false
	if _, err := exec.LookPath("sox"); err == nil {
		hasRecorder = true
	}
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		hasRecorder = true
	}
	if !hasRecorder {
		return false
	}
	if _, err := exec.LookPath("whisper"); err == nil {
		return true
	}
	return false
}
