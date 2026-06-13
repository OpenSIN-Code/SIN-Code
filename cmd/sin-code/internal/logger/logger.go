// SPDX-License-Identifier: MIT
// Purpose: Structured logging for sin-code with JSON output and log levels.
package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

type Logger struct {
	level  Level
	output io.Writer
	mu     sync.Mutex
}

// getOutput returns the current output writer.
// If no custom output is set, it returns os.Stderr.
func (l *Logger) getOutput() io.Writer {
	if l.output == nil {
		return os.Stderr
	}
	return l.output
}

type Entry struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

var defaultLogger = &Logger{
    level:  LevelInfo,
    output: nil, // nil means use os.Stderr dynamically
}

func Default() *Logger {
	return defaultLogger
}

func SetLevel(level Level) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.level = level
}

func SetOutput(w io.Writer) {
	defaultLogger.mu.Lock()
	defer defaultLogger.mu.Unlock()
	defaultLogger.output = w
}

func (l *Logger) log(level Level, msg string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.level {
		return
	}

	entry := Entry{
		Time:    time.Now().UTC().Format(time.RFC3339),
		Level:   level.String(),
		Message: msg,
		Fields:  fields,
	}

	data, err := json.Marshal(entry)
	if err != nil {
		fmt.Fprintf(l.getOutput(), `{"time":"%s","level":"ERROR","message":"logger marshal failed: %v"}`+"\n",
			time.Now().UTC().Format(time.RFC3339), err)
		return
	}

	fmt.Fprintln(l.getOutput(), string(data))
}

func (l *Logger) Debug(msg string, fields ...map[string]any) {
	l.log(LevelDebug, msg, mergeFields(fields...))
}

func (l *Logger) Info(msg string, fields ...map[string]any) {
	l.log(LevelInfo, msg, mergeFields(fields...))
}

func (l *Logger) Warn(msg string, fields ...map[string]any) {
	l.log(LevelWarn, msg, mergeFields(fields...))
}

func (l *Logger) Error(msg string, fields ...map[string]any) {
	l.log(LevelError, msg, mergeFields(fields...))
}

func Debug(msg string, fields ...map[string]any) {
	defaultLogger.Debug(msg, fields...)
}

func Info(msg string, fields ...map[string]any) {
	defaultLogger.Info(msg, fields...)
}

func Warn(msg string, fields ...map[string]any) {
	defaultLogger.Warn(msg, fields...)
}

func Error(msg string, fields ...map[string]any) {
	defaultLogger.Error(msg, fields...)
}

func mergeFields(fields ...map[string]any) map[string]any {
	if len(fields) == 0 {
		return nil
	}
	result := make(map[string]any)
	for _, f := range fields {
		for k, v := range f {
			result[k] = v
		}
	}
	return result
}
