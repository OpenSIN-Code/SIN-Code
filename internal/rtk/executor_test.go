package rtk

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestSimpleExecutorDetection tests binary detection
func TestSimpleExecutorDetection(t *testing.T) {
	tests := []struct {
		name    string
		paths   []string
		wantErr bool
	}{
		{
			name:    "valid executable path",
			paths:   []string{"/usr/local/bin", "/usr/bin"},
			wantErr: false,
		},
		{
			name:    "custom path",
			paths:   []string{os.TempDir()},
			wantErr: true, // temp dir won't have rtk
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := &SimpleExecutor{}
			info, err := executor.DetectRTKBinary(tt.paths)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectRTKBinary() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && info == nil {
				t.Error("expected RTKBinaryInfo, got nil")
			}
		})
	}
}

// TestANSIStripperRemovesColors tests ANSI color code removal
func TestANSIStripperRemovesColors(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
		reduction float64
	}{
		{
			name:     "simple red text",
			input:    "\x1b[31mRed Text\x1b[0m",
			expected: "Red Text",
			reduction: 0.8,
		},
		{
			name:     "multiple colors",
			input:    "\x1b[32mGreen\x1b[0m \x1b[31mRed\x1b[0m \x1b[34mBlue\x1b[0m",
			expected: "Green Red Blue",
			reduction: 0.7,
		},
		{
			name:     "no colors",
			input:    "Plain text",
			expected: "Plain text",
			reduction: 0.0,
		},
		{
			name:     "complex formatting",
			input:    "\x1b[1;32m\x1b[K✓ Success\x1b[0m\x1b[K",
			expected: "✓ Success",
			reduction: 0.85,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestExecutorWithTimeout tests execution timeout handling
func TestExecutorWithTimeout(t *testing.T) {
	executor := &SimpleExecutor{
		timeout: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// This will timeout
	result, err := executor.Execute(ctx, "echo", "test")
	if err == nil {
		t.Error("expected timeout error, got nil")
	}
	if result != nil {
		t.Error("expected nil result on timeout")
	}
}

// TestExecutorExitCodeHandling tests exit code extraction
func TestExecutorExitCodeHandling(t *testing.T) {
	executor := &SimpleExecutor{}

	tests := []struct {
		command   string
		args      []string
		expectErr bool
	}{
		{
			command:   "true",
			args:      []string{},
			expectErr: false,
		},
		{
			command:   "false",
			args:      []string{},
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.command, strings.Join(tt.args, "_")), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := executor.Execute(ctx, tt.command, tt.args...)
			if (err != nil) != tt.expectErr {
				t.Errorf("Execute() error = %v, expectErr %v", err, tt.expectErr)
			}
		})
	}
}

// TestExecutorMetricsCollection tests metrics tracking
func TestExecutorMetricsCollection(t *testing.T) {
	executor := &SimpleExecutor{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := executor.Execute(ctx, "echo", "hello")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if result.ExecutionTime <= 0 {
		t.Error("ExecutionTime should be > 0")
	}

	if result.TokensOriginal <= 0 {
		t.Error("TokensOriginal should be > 0")
	}
}

// TestExecutorConcurrency tests concurrent execution safety
func TestExecutorConcurrency(t *testing.T) {
	executor := &SimpleExecutor{}

	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := executor.Execute(ctx, "echo", fmt.Sprintf("test%d", index))
			results <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-results; err != nil {
			t.Errorf("concurrent execution %d failed: %v", i, err)
		}
	}
}

// BenchmarkANSIStripping benchmarks ANSI stripping performance
func BenchmarkANSIStripping(b *testing.B) {
	text := strings.Repeat("\x1b[31mRed\x1b[0m ", 100)
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		stripANSI(text)
	}
}

// BenchmarkDetection benchmarks binary detection performance
func BenchmarkDetection(b *testing.B) {
	executor := &SimpleExecutor{}
	paths := []string{"/usr/local/bin", "/usr/bin", "/bin"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.DetectRTKBinary(paths)
	}
}

// BenchmarkExecution benchmarks command execution
func BenchmarkExecution(b *testing.B) {
	executor := &SimpleExecutor{}
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		executor.Execute(ctx, "echo", "test")
	}
}

// TestExecutorErrorHandling tests various error scenarios
func TestExecutorErrorHandling(t *testing.T) {
	executor := &SimpleExecutor{}

	tests := []struct {
		name      string
		command   string
		wantError bool
	}{
		{
			name:      "valid command",
			command:   "echo",
			wantError: false,
		},
		{
			name:      "nonexistent command",
			command:   "this_command_does_not_exist_12345",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			_, err := executor.Execute(ctx, tt.command)
			if (err != nil) != tt.wantError {
				t.Errorf("Execute() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

// TestExecutorOutputSize tests handling of large output
func TestExecutorOutputSize(t *testing.T) {
	executor := &SimpleExecutor{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Generate 1MB of output
	result, err := executor.Execute(ctx, "bash", "-c", "printf 'a%.0s' {1..1000000}")
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if result == nil {
		t.Fatal("expected result, got nil")
	}

	if len(result.Output) == 0 {
		t.Error("expected non-empty output")
	}
}

// TestExecutorContextCancellation tests context cancellation handling
func TestExecutorContextCancellation(t *testing.T) {
	executor := &SimpleExecutor{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := executor.Execute(ctx, "echo", "test")
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// TestExecutorBinaryPathResolution tests path resolution
func TestExecutorBinaryPathResolution(t *testing.T) {
	executor := &SimpleExecutor{}

	// Test with absolute path
	if runtime.GOOS == "windows" {
		_, err := executor.Execute(context.Background(), "cmd", "/c", "echo test")
		if err != nil {
			t.Errorf("Execute() with absolute path failed: %v", err)
		}
	} else {
		_, err := executor.Execute(context.Background(), "/bin/echo", "test")
		if err != nil {
			t.Errorf("Execute() with absolute path failed: %v", err)
		}
	}
}

// TestExecutorSpecialCharacters tests handling of special characters
func TestExecutorSpecialCharacters(t *testing.T) {
	executor := &SimpleExecutor{}

	tests := []struct {
		name string
		arg  string
	}{
		{"spaces", "hello world"},
		{"quotes", "hello'world"},
		{"special", "hello$world"},
		{"unicode", "hello🌍"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			result, err := executor.Execute(ctx, "echo", tt.arg)
			if err != nil {
				t.Errorf("Execute() error = %v", err)
			}

			if result == nil {
				t.Fatal("expected result, got nil")
			}

			if !strings.Contains(result.Output, tt.arg) {
				t.Errorf("output doesn't contain input: got %q, want %q", result.Output, tt.arg)
			}
		})
	}
}
