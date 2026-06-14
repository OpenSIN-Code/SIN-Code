package observability

import (
	"context"
	"fmt"
	"time"
)

// MetricsCollector collects metrics
type MetricsCollector struct {
	metrics []*Metric
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		metrics: []*Metric{},
	}
}

// RecordMetric records a metric value
func (m *MetricsCollector) RecordMetric(ctx context.Context, name string, value float64, unit string, tags map[string]string) error {
	metric := &Metric{
		Name:      name,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
		Tags:      tags,
	}

	m.metrics = append(m.metrics, metric)
	return nil
}

// RecordLatency records operation latency
func (m *MetricsCollector) RecordLatency(ctx context.Context, operation string, duration time.Duration) error {
	return m.RecordMetric(ctx, fmt.Sprintf("%s_latency", operation), float64(duration.Milliseconds()), "ms", map[string]string{})
}

// GetMetrics retrieves all collected metrics
func (m *MetricsCollector) GetMetrics(ctx context.Context) []*Metric {
	return m.metrics
}

// GetMetricsByName retrieves metrics by name
func (m *MetricsCollector) GetMetricsByName(ctx context.Context, name string) []*Metric {
	result := []*Metric{}
	for _, metric := range m.metrics {
		if metric.Name == name {
			result = append(result, metric)
		}
	}
	return result
}
