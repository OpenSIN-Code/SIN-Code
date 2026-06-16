// SPDX-License-Identifier: MIT
package logger

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestLoggerLevels(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	defer SetOutput(nil)

	SetLevel(LevelInfo)
	Debug("debug message")
	Info("info message")
	Warn("warn message")
	Error("error message")

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	if len(lines) != 3 {
		t.Errorf("expected 3 log lines (info, warn, error), got %d: %v", len(lines), lines)
	}

	if !strings.Contains(lines[0], `"level":"INFO"`) {
		t.Errorf("first line should be INFO, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"level":"WARN"`) {
		t.Errorf("second line should be WARN, got: %s", lines[1])
	}
	if !strings.Contains(lines[2], `"level":"ERROR"`) {
		t.Errorf("third line should be ERROR, got: %s", lines[2])
	}
}

func TestLoggerFields(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	defer SetOutput(nil)

	SetLevel(LevelInfo)
	Info("test message", map[string]any{
		"key1": "value1",
		"key2": 42,
	})

	output := buf.String()
	var entry Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry.Message != "test message" {
		t.Errorf("expected message 'test message', got %q", entry.Message)
	}
	if entry.Level != "INFO" {
		t.Errorf("expected level INFO, got %q", entry.Level)
	}
	if entry.Fields["key1"] != "value1" {
		t.Errorf("expected field key1=value1, got %v", entry.Fields["key1"])
	}
	if entry.Fields["key2"] != 42.0 {
		t.Errorf("expected field key2=42, got %v", entry.Fields["key2"])
	}
}

func TestLoggerTimeFormat(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	defer SetOutput(nil)

	SetLevel(LevelInfo)
	Info("timestamp test")

	output := buf.String()
	var entry Entry
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &entry); err != nil {
		t.Fatalf("failed to parse log entry: %v", err)
	}

	if entry.Time == "" {
		t.Error("expected non-empty timestamp")
	}
	if !strings.Contains(entry.Time, "T") {
		t.Errorf("expected RFC3339 format, got %q", entry.Time)
	}
}

func TestDefault(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("Default() returned nil")
	}
}

func TestLevelStringUnknown(t *testing.T) {
	if got := Level(99).String(); got != "UNKNOWN" {
		t.Errorf("expected UNKNOWN, got %q", got)
	}
}

func TestLevelStrings(t *testing.T) {
	for level, want := range map[Level]string{
		LevelDebug: "DEBUG",
		LevelInfo:  "INFO",
		LevelWarn:  "WARN",
		LevelError: "ERROR",
	} {
		if got := level.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", level, got, want)
		}
	}
}

func TestGetOutputNil(t *testing.T) {
	l := &Logger{}
	if got := l.getOutput(); got != os.Stderr {
		t.Errorf("expected os.Stderr for nil output, got %v", got)
	}
}

func TestLogMarshalError(t *testing.T) {
	var buf bytes.Buffer
	SetOutput(&buf)
	defer SetOutput(nil)

	SetLevel(LevelInfo)
	Info("marshal error", map[string]any{"ch": make(chan int)})

	output := buf.String()
	if !strings.Contains(output, "logger marshal failed") {
		t.Errorf("expected marshal failure log, got %q", output)
	}
}
