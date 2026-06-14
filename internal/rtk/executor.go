package rtk

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// SimpleExecutor is a basic RTK executor implementation
type SimpleExecutor struct {
	config     *RTKConfig
	metrics    *RTKMetrics
	binaryInfo *RTKBinaryInfo
	cache      *ResultCache
}

// NewSimpleExecutor creates a new RTK executor
func NewSimpleExecutor(config *RTKConfig) (*SimpleExecutor, error) {
	if config == nil {
		config = &RTKConfig{
			Enabled:       true,
			DetectBinary:  true,
			GlobalTimeout: DefaultGlobalTimeout,
			CacheEnabled:  true,
			CacheTTL:      DefaultCacheTTL,
			StripANSI:     true,
			LogLevel:      DefaultLogLevel,
			ExecutionMode: RTKExecutionModeLocal,
		}
	}

	executor := &SimpleExecutor{
		config:  config,
		metrics: &RTKMetrics{},
		cache:   NewResultCache(config.CacheTTL),
	}

	// Try to detect binary
	if config.DetectBinary {
		result, err := executor.detectBinary()
		if err == nil && result.Found {
			executor.binaryInfo = &RTKBinaryInfo{
				Path:     result.Path,
				Version:  result.Version,
				Detected: true,
				LastSeen: time.Now(),
			}
		}
	}

	return executor, nil
}

// Execute runs an RTK tool
func (e *SimpleExecutor) Execute(ctx context.Context, tool *RTKTool) (*RTKResult, error) {
	if !e.config.Enabled {
		return nil, ErrInvalidConfig
	}

	if tool == nil || tool.Name == "" {
		return nil, ErrInvalidTool
	}

	// Check cache
	if e.config.CacheEnabled && e.cache != nil {
		if cached, found := e.cache.Get(tool.CacheKey); found {
			e.metrics.CacheHits++
			cached.Cached = true
			return cached, nil
		}
	}

	e.metrics.CacheMisses++

	// Set timeout
	timeout := tool.Timeout
	if timeout == 0 {
		timeout = e.config.GlobalTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Build command
	cmd := e.buildCommand(execCtx, tool)
	if cmd == nil {
		e.metrics.FailedCalls++
		return nil, ErrBinaryNotFound
	}

	// Execute
	result := &RTKResult{
		Name:      tool.Name,
		Timestamp: time.Now(),
	}

	start := time.Now()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result.Duration = time.Since(start)

	// Capture output
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()

	// Strip ANSI if configured
	if e.config.StripANSI {
		result.StdoutClean = StripANSI(result.Stdout)
		result.StderrClean = StripANSI(result.Stderr)
		
		// Calculate token savings
		originalTokens := CountTokens(result.Stdout)
		cleanTokens := CountTokens(result.StdoutClean)
		result.TokenCount = cleanTokens
		saved := originalTokens - cleanTokens
		e.metrics.TokensSaved += int64(saved)
		
		if originalTokens > 0 {
			e.metrics.TokenReduction = float64(e.metrics.TokensSaved) / float64(originalTokens)
		}
	} else {
		result.TokenCount = CountTokens(result.Stdout)
	}

	// Set status and exit code
	if err != nil {
		if execCtx.Err() == context.DeadlineExceeded {
			result.Status = RTKStatusTimeout
			e.metrics.FailedCalls++
		} else {
			result.Status = RTKStatusError
			result.ExitCode = getExitCode(err)
			e.metrics.FailedCalls++
		}
	} else {
		result.Status = RTKStatusSuccess
		e.metrics.SuccessfulCalls++
	}

	// Update metrics
	e.metrics.TotalExecutions++
	e.metrics.TotalDuration += result.Duration
	e.metrics.LastExecution = result.Timestamp
	if result.Status == RTKStatusError {
		e.metrics.LastError = result.Stderr
	}

	// Cache result
	if e.config.CacheEnabled && e.cache != nil && result.Status == RTKStatusSuccess {
		e.cache.Set(tool.CacheKey, result)
	}

	return result, nil
}

// Detect finds the RTK binary
func (e *SimpleExecutor) Detect(ctx context.Context) (*RTKDetectionResult, error) {
	result, err := e.detectBinary()
	if err != nil {
		return result, err
	}

	// Cache binary info for subsequent command execution
	if result.Found {
		e.binaryInfo = &RTKBinaryInfo{
			Path:     result.Path,
			Version:  result.Version,
			Detected: true,
			LastSeen: time.Now(),
		}
	}

	return result, nil
}

func (e *SimpleExecutor) detectBinary() (*RTKDetectionResult, error) {
	start := time.Now()

	// Check custom path first
	if e.config.BinaryPath != "" {
		if fileExists(e.config.BinaryPath) {
			info, err := e.getBinaryInfo(e.config.BinaryPath)
			if err == nil {
				return &RTKDetectionResult{
					Found:         true,
					Path:          e.config.BinaryPath,
					Version:       info.Version,
					DetectionTime: time.Since(start),
					Method:        "custom_path",
				}, nil
			}
		}
	}

	// Search PATH
	paths := []string{
		"rtk",
		"./rtk",
		"/usr/local/bin/rtk",
		"/usr/bin/rtk",
	}

	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".local/bin/rtk"))
	}

	for _, path := range paths {
		if fullPath, err := exec.LookPath(path); err == nil {
			info, err := e.getBinaryInfo(fullPath)
			if err == nil {
				return &RTKDetectionResult{
					Found:         true,
					Path:          fullPath,
					Version:       info.Version,
					DetectionTime: time.Since(start),
					Method:        "path_search",
				}, nil
			}
		}
	}

	return &RTKDetectionResult{
		Found:         false,
		DetectionTime: time.Since(start),
		Method:        "not_found",
	}, ErrBinaryNotFound
}

func (e *SimpleExecutor) getBinaryInfo(path string) (*RTKBinaryInfo, error) {
	// Check if file exists and is executable
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	if fileInfo.IsDir() {
		return nil, fmt.Errorf("path is a directory: %s", path)
	}

	// Try to get version
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.Output()
	
	version := "unknown"
	if err == nil {
		version = strings.TrimSpace(string(output))
	}

	return &RTKBinaryInfo{
		Path:     path,
		Version:  version,
		LastSeen: time.Now(),
		Detected: true,
	}, nil
}

func (e *SimpleExecutor) buildCommand(ctx context.Context, tool *RTKTool) *exec.Cmd {
	if e.binaryInfo == nil {
		// Try to detect
		result, err := e.detectBinary()
		if err != nil {
			return nil
		}
		e.binaryInfo = &RTKBinaryInfo{
			Path:     result.Path,
			Version:  result.Version,
			Detected: true,
			LastSeen: time.Now(),
		}
	}

	args := []string{}
	if tool.Args != nil {
		args = append(args, tool.Args...)
	}

	return exec.CommandContext(ctx, e.binaryInfo.Path, args...)
}

// GetConfig returns the executor config
func (e *SimpleExecutor) GetConfig() *RTKConfig {
	return e.config
}

// SetConfig updates the executor config
func (e *SimpleExecutor) SetConfig(config *RTKConfig) error {
	if config == nil {
		return ErrInvalidConfig
	}
	e.config = config
	return nil
}

// GetMetrics returns collected metrics
func (e *SimpleExecutor) GetMetrics() *RTKMetrics {
	if e.metrics.TotalExecutions > 0 {
		e.metrics.AverageDuration = time.Duration(int64(e.metrics.TotalDuration) / e.metrics.TotalExecutions)
	}
	return e.metrics
}

// ResetMetrics clears metrics
func (e *SimpleExecutor) ResetMetrics() {
	e.metrics = &RTKMetrics{}
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getExitCode(err error) int {
	if err == nil {
		return 0
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode()
	}
	return -1
}

// StripANSI removes ANSI color codes from text
func StripANSI(text string) string {
	// Remove common ANSI sequences
	ansiRegex := "\x1b\\[[0-9;]*m"
	
	result := text
	for {
		prev := result
		// Match ANSI escape sequences
		if idx := strings.Index(result, "\x1b["); idx != -1 {
			end := strings.IndexByte(result[idx:], 'm')
			if end == -1 {
				break
			}
			result = result[:idx] + result[idx+end+1:]
		} else {
			break
		}
	}
	
	_ = ansiRegex // Used for documentation
	return result
}

// CountTokens estimates token count (rough approximation)
func CountTokens(text string) int {
	// Rough estimation: 1 token ≈ 4 characters
	return (len(text) + 3) / 4
}
