package rtk

import (
	"context"
	"encoding/json"
	"time"
)

// RTKToolKind represents the type of RTK tool
type RTKToolKind string

const (
	RTKToolKindCommand   RTKToolKind = "command"
	RTKToolKindAnalyzer  RTKToolKind = "analyzer"
	RTKToolKindValidator RTKToolKind = "validator"
	RTKToolKindFormatter RTKToolKind = "formatter"
)

// RTKStatus represents the status of RTK operation
type RTKStatus string

const (
	RTKStatusSuccess RTKStatus = "success"
	RTKStatusError   RTKStatus = "error"
	RTKStatusWarning RTKStatus = "warning"
	RTKStatusTimeout RTKStatus = "timeout"
)

// RTKExecutionMode represents how RTK is executed
type RTKExecutionMode string

const (
	RTKExecutionModeLocal  RTKExecutionMode = "local"
	RTKExecutionModeRemote RTKExecutionMode = "remote"
	RTKExecutionModeMCP    RTKExecutionMode = "mcp"
)

// RTKBinaryInfo contains information about the RTK binary
type RTKBinaryInfo struct {
	Path    string    // Path to rtk binary
	Version string    // RTK version
	Mode    string    // Installation mode (system, local, docker)
	LastSeen time.Time // Last successful execution
	Detected bool      // Auto-detected or manually configured
}

// RTKTool represents a single RTK tool/command
type RTKTool struct {
	Name        string            // Tool name (e.g., "lint", "format", "test")
	Kind        RTKToolKind       // Tool kind
	Description string            // Tool description
	Args        []string          // Command arguments
	Timeout     time.Duration     // Execution timeout
	CacheKey    string            // Cache key for results
	RetryPolicy *RetryPolicy      // Retry configuration
	Tags        map[string]string // Metadata tags
	Enabled     bool              // Whether tool is enabled
}

// RTKResult represents the result of an RTK operation
type RTKResult struct {
	Name          string            // Tool name
	Status        RTKStatus         // Execution status
	ExitCode      int               // Exit code
	Stdout        string            // Standard output
	Stderr        string            // Standard error
	StdoutClean   string            // ANSI-stripped stdout
	StderrClean   string            // ANSI-stripped stderr
	Duration      time.Duration     // Execution duration
	TokenCount    int               // Token count (output)
	CacheMiss     bool              // Cache miss indicator
	Cached        bool              // Was result cached
	CachedAt      time.Time         // Cache timestamp
	Metadata      map[string]interface{} // Additional metadata
	Timestamp     time.Time         // Execution timestamp
}

// RTKConfig represents RTK configuration
type RTKConfig struct {
	Enabled          bool                      // Enable RTK integration
	BinaryPath       string                    // Custom rtk binary path
	DetectBinary     bool                      // Auto-detect rtk binary
	Tools            map[string]*RTKTool      // Configured tools
	GlobalTimeout    time.Duration             // Global timeout for all tools
	CacheEnabled     bool                      // Enable result caching
	CacheTTL         time.Duration             // Cache time-to-live
	CacheDir         string                    // Cache directory path
	StripANSI        bool                      // Strip ANSI color codes
	MetricsEnabled   bool                      // Enable metrics collection
	LogLevel         string                    // Log level (debug, info, warn, error)
	RetryPolicy      *RetryPolicy              // Global retry policy
	ExecutionMode    RTKExecutionMode          // How RTK is executed
	MCPServerAddress string                    // MCP server address (for remote mode)
}

// RetryPolicy defines retry behavior
type RetryPolicy struct {
	MaxRetries      int           // Maximum number of retries
	InitialBackoff  time.Duration // Initial backoff duration
	MaxBackoff      time.Duration // Maximum backoff duration
	BackoffMultiplier float64     // Backoff multiplier (exponential)
	RetryableErrors []string      // Error patterns to retry on
}

// RTKMetrics tracks RTK performance metrics
type RTKMetrics struct {
	TotalExecutions  int64          // Total number of executions
	SuccessfulCalls  int64          // Successful executions
	FailedCalls      int64          // Failed executions
	CacheHits        int64          // Cache hits
	CacheMisses      int64          // Cache misses
	TotalDuration    time.Duration  // Total execution time
	AverageDuration  time.Duration  // Average execution time
	TokensSaved      int64          // Tokens saved through ANSI stripping
	TokenReduction   float64        // Token reduction percentage (0.0-1.0)
	LastExecution    time.Time      // Last execution timestamp
	LastError        string         // Last error message
}

// RTKDetectionResult represents binary detection result
type RTKDetectionResult struct {
	Found         bool              // Binary found
	Path          string            // Path to binary (if found)
	Version       string            // Binary version
	Capabilities  []string          // Detected capabilities
	DetectionTime time.Duration     // Time taken to detect
	Method        string            // Detection method used
}

// RTKContext wraps context with RTK-specific information
type RTKContext struct {
	ctx          context.Context
	config       *RTKConfig
	metrics      *RTKMetrics
	cacheEnabled bool
}

// RTKExecutor handles RTK tool execution
type RTKExecutor interface {
	Execute(ctx context.Context, tool *RTKTool) (*RTKResult, error)
	Detect(ctx context.Context) (*RTKBinaryInfo, error)
	GetConfig() *RTKConfig
	SetConfig(config *RTKConfig) error
	GetMetrics() *RTKMetrics
	ResetMetrics()
}

// Default values
const (
	DefaultRTKTimeout    = 30 * time.Second
	DefaultGlobalTimeout = 60 * time.Second
	DefaultCacheTTL      = 24 * time.Hour
	DefaultLogLevel      = "info"
	DefaultRetryCount    = 3
)

// RTKToolRegistry maintains a registry of available tools
type RTKToolRegistry struct {
	tools map[string]*RTKTool
}

// NewRTKToolRegistry creates a new tool registry
func NewRTKToolRegistry() *RTKToolRegistry {
	return &RTKToolRegistry{
		tools: make(map[string]*RTKTool),
	}
}

// Register adds a tool to the registry
func (r *RTKToolRegistry) Register(tool *RTKTool) error {
	if tool == nil || tool.Name == "" {
		return ErrInvalidTool
	}
	r.tools[tool.Name] = tool
	return nil
}

// Get retrieves a tool from registry
func (r *RTKToolRegistry) Get(name string) (*RTKTool, bool) {
	tool, ok := r.tools[name]
	return tool, ok
}

// List returns all registered tools
func (r *RTKToolRegistry) List() []*RTKTool {
	tools := make([]*RTKTool, 0, len(r.tools))
	for _, t := range r.tools {
		tools = append(tools, t)
	}
	return tools
}

// RTKError represents RTK-specific errors
type RTKError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

func (e *RTKError) Error() string {
	return e.Message
}

// Common RTK errors
var (
	ErrBinaryNotFound    = &RTKError{Code: "RTK_BINARY_NOT_FOUND", Message: "RTK binary not found"}
	ErrInvalidTool       = &RTKError{Code: "RTK_INVALID_TOOL", Message: "Invalid or missing RTK tool"}
	ErrExecutionFailed   = &RTKError{Code: "RTK_EXECUTION_FAILED", Message: "RTK execution failed"}
	ErrTimeout           = &RTKError{Code: "RTK_TIMEOUT", Message: "RTK execution timeout"}
	ErrInvalidConfig     = &RTKError{Code: "RTK_INVALID_CONFIG", Message: "Invalid RTK configuration"}
	ErrCacheError        = &RTKError{Code: "RTK_CACHE_ERROR", Message: "Cache operation failed"}
)
