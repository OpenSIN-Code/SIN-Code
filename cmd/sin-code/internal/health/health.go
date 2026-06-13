// SPDX-License-Identifier: MIT
// Purpose: Health check endpoints for monitoring and readiness probes.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusDegraded  Status = "degraded"
	StatusUnhealthy Status = "unhealthy"
)

type HealthResponse struct {
	Status    Status            `json:"status"`
	Version   string            `json:"version"`
	Uptime    string            `json:"uptime"`
	Timestamp string            `json:"timestamp"`
	Checks    map[string]Check  `json:"checks"`
}

type Check struct {
	Status  Status `json:"status"`
	Message string `json:"message,omitempty"`
}

type Checker struct {
	startTime time.Time
	version   string
	checks    map[string]func(context.Context) Check
}

func NewChecker(version string) *Checker {
	return &Checker{
		startTime: time.Now(),
		version:   version,
		checks:    make(map[string]func(context.Context) Check),
	}
}

func (c *Checker) RegisterCheck(name string, check func(context.Context) Check) {
	c.checks[name] = check
}

func (c *Checker) Version() string {
	return c.version
}

func (c *Checker) Check(ctx context.Context) HealthResponse {
	resp := HealthResponse{
		Status:    StatusHealthy,
		Version:   c.version,
		Uptime:    time.Since(c.startTime).String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Checks:    make(map[string]Check),
	}

	for name, check := range c.checks {
		result := check(ctx)
		resp.Checks[name] = result
		if result.Status == StatusUnhealthy {
			resp.Status = StatusUnhealthy
		} else if result.Status == StatusDegraded && resp.Status == StatusHealthy {
			resp.Status = StatusDegraded
		}
	}

	return resp
}

func (c *Checker) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resp := c.Check(ctx)

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(resp)
	})
}

// LivenessHandler returns a simple liveness probe (always 200 OK).
func LivenessHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

// ReadinessHandler returns readiness based on dependency checks.
func ReadinessHandler(checker *Checker) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		resp := checker.Check(ctx)

		w.Header().Set("Content-Type", "application/json")
		if resp.Status == StatusUnhealthy {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "not ready",
				"reason": "unhealthy checks",
			})
		} else {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "ready",
			})
		}
	})
}

// RuntimeInfo returns runtime information for diagnostics.
func RuntimeInfo() map[string]interface{} {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	return map[string]interface{}{
		"go_version":    runtime.Version(),
		"go_os":         runtime.GOOS,
		"go_arch":       runtime.GOARCH,
		"num_cpu":       runtime.NumCPU(),
		"num_goroutine": runtime.NumGoroutine(),
		"memory": map[string]interface{}{
			"alloc_mb":       float64(m.Alloc) / 1024 / 1024,
			"total_alloc_mb": float64(m.TotalAlloc) / 1024 / 1024,
			"sys_mb":         float64(m.Sys) / 1024 / 1024,
			"num_gc":         m.NumGC,
		},
	}
}

// InfoHandler returns runtime and version information.
func InfoHandler(version string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		info := RuntimeInfo()
		info["version"] = version

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(info)
	})
}
