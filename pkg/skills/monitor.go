package skills

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type SkillEvent struct {
	ID         string                 `json:"id"`
	SkillName  string                 `json:"skill_name"`
	StepNumber int                    `json:"step_number"`
	EventType  string                 `json:"event_type"` // "start", "step_start", "step_end", "tool_call", "error", "complete"
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data"`
}

type SkillMonitor struct {
	mu         sync.Mutex
	events     []SkillEvent
	logFile    *os.File
	metrics    map[string]*SkillMetrics
	ctx        context.Context
	cancelFunc context.CancelFunc
}

type SkillMetrics struct {
	Name           string
	TotalRuns      int
	SuccessCount   int
	FailureCount   int
	AvgDuration    time.Duration
	LastRun        time.Time
	StepsExecuted  map[string]int // step index -> count
	ToolCallCount  map[string]int // tool name -> count
}

func NewSkillMonitor(logDir string) (*SkillMonitor, error) {
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("skills_%s.log", time.Now().Format("20060102")))
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &SkillMonitor{
		events:     []SkillEvent{},
		logFile:    f,
		metrics:    make(map[string]*SkillMetrics),
		ctx:        ctx,
		cancelFunc: cancel,
	}
	go m.periodicFlush()
	return m, nil
}

func (m *SkillMonitor) Record(event SkillEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()
	event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	m.events = append(m.events, event)
	// Update metrics
	if _, ok := m.metrics[event.SkillName]; !ok {
		m.metrics[event.SkillName] = &SkillMetrics{
			Name:          event.SkillName,
			StepsExecuted: make(map[string]int),
			ToolCallCount: make(map[string]int),
		}
	}
	metric := m.metrics[event.SkillName]
	switch event.EventType {
	case "start":
		metric.TotalRuns++
		metric.LastRun = event.Timestamp
	case "complete":
		if success, ok := event.Data["success"].(bool); ok && success {
			metric.SuccessCount++
		} else {
			metric.FailureCount++
		}
	case "step_end":
		stepKey := fmt.Sprintf("step_%d", event.StepNumber)
		metric.StepsExecuted[stepKey]++
	case "tool_call":
		toolName, _ := event.Data["tool"].(string)
		metric.ToolCallCount[toolName]++
	}
}

func (m *SkillMonitor) periodicFlush() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.Flush()
		}
	}
}

func (m *SkillMonitor) Flush() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.events) == 0 {
		return nil
	}
	encoder := json.NewEncoder(m.logFile)
	for _, ev := range m.events {
		if err := encoder.Encode(ev); err != nil {
			return err
		}
	}
	m.events = m.events[:0]
	return nil
}

func (m *SkillMonitor) Close() error {
	m.cancelFunc()
	m.Flush()
	return m.logFile.Close()
}

// GetMetrics returns a copy of metrics for reporting.
func (m *SkillMonitor) GetMetrics() map[string]*SkillMetrics {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make(map[string]*SkillMetrics)
	for k, v := range m.metrics {
		copy := *v
		result[k] = &copy
	}
	return result
}
