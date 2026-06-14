package observability

import (
	"context"
	"fmt"
	"time"
)

// Logger implements structured logging
type Logger struct {
	entries []*LogEntry
	level   string
}

// NewLogger creates a new logger
func NewLogger(level string) *Logger {
	return &Logger{
		entries: []*LogEntry{},
		level:   level,
	}
}

// Log logs a message
func (l *Logger) Log(ctx context.Context, level string, message string, fields map[string]interface{}) error {
	entry := &LogEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Fields:    fields,
	}

	l.entries = append(l.entries, entry)
	fmt.Printf("[%s] %s: %s\n", level, entry.Timestamp.Format(time.RFC3339), message)

	return nil
}

// Info logs an info message
func (l *Logger) Info(ctx context.Context, message string, fields map[string]interface{}) error {
	return l.Log(ctx, "INFO", message, fields)
}

// Error logs an error message
func (l *Logger) Error(ctx context.Context, message string, fields map[string]interface{}) error {
	return l.Log(ctx, "ERROR", message, fields)
}

// Debug logs a debug message
func (l *Logger) Debug(ctx context.Context, message string, fields map[string]interface{}) error {
	if l.level == "DEBUG" {
		return l.Log(ctx, "DEBUG", message, fields)
	}
	return nil
}

// GetEntries retrieves all log entries
func (l *Logger) GetEntries(ctx context.Context) []*LogEntry {
	return l.entries
}
