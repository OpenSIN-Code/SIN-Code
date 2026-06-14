package observability

import (
	"time"
)

// Metric represents a collected metric
type Metric struct {
	Name      string            `json:"name"`
	Value     float64           `json:"value"`
	Unit      string            `json:"unit"`
	Timestamp time.Time         `json:"timestamp"`
	Tags      map[string]string `json:"tags"`
}

// TraceSpan represents a trace span
type TraceSpan struct {
	ID        string            `json:"id"`
	ParentID  string            `json:"parent_id"`
	Name      string            `json:"name"`
	StartTime time.Time         `json:"start_time"`
	EndTime   time.Time         `json:"end_time"`
	Duration  time.Duration     `json:"duration"`
	Tags      map[string]string `json:"tags"`
	Events    []string          `json:"events"`
}

// Trace represents a complete trace
type Trace struct {
	ID    string       `json:"id"`
	Spans []*TraceSpan `json:"spans"`
}

// LogEntry represents a structured log entry
type LogEntry struct {
	Timestamp time.Time             `json:"timestamp"`
	Level     string                `json:"level"`
	Message   string                `json:"message"`
	Fields    map[string]interface{} `json:"fields"`
}
