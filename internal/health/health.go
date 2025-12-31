// Package health provides health check functionality for the code reviewer.
package health

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status.
type Status string

const (
	StatusHealthy   Status = "healthy"
	StatusUnhealthy Status = "unhealthy"
	StatusDegraded  Status = "degraded"
)

// CheckResult represents the result of a health check.
type CheckResult struct {
	Status    Status        `json:"status"`
	Message   string        `json:"message,omitempty"`
	Duration  time.Duration `json:"duration_ms"`
	Timestamp time.Time     `json:"timestamp"`
}

// HealthResponse represents the full health response.
type HealthResponse struct {
	Status      Status                  `json:"status"`
	Version     string                  `json:"version,omitempty"`
	Uptime      time.Duration           `json:"uptime_seconds"`
	Checks      map[string]*CheckResult `json:"checks,omitempty"`
	Timestamp   time.Time               `json:"timestamp"`
}

// Checker is a function that performs a health check.
type Checker func(ctx context.Context) *CheckResult

// Health manages health checks.
type Health struct {
	mu        sync.RWMutex
	checkers  map[string]Checker
	version   string
	startTime time.Time
	timeout   time.Duration
}

// New creates a new Health instance.
func New(version string) *Health {
	return &Health{
		checkers:  make(map[string]Checker),
		version:   version,
		startTime: time.Now(),
		timeout:   5 * time.Second,
	}
}

// SetTimeout sets the timeout for health checks.
func (h *Health) SetTimeout(timeout time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.timeout = timeout
}

// Register registers a health check.
func (h *Health) Register(name string, checker Checker) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checkers[name] = checker
}

// Unregister removes a health check.
func (h *Health) Unregister(name string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.checkers, name)
}

// Check runs all health checks and returns the overall status.
func (h *Health) Check(ctx context.Context) *HealthResponse {
	h.mu.RLock()
	checkers := make(map[string]Checker, len(h.checkers))
	for k, v := range h.checkers {
		checkers[k] = v
	}
	timeout := h.timeout
	h.mu.RUnlock()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	response := &HealthResponse{
		Status:    StatusHealthy,
		Version:   h.version,
		Uptime:    time.Since(h.startTime),
		Checks:    make(map[string]*CheckResult),
		Timestamp: time.Now(),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex

	for name, checker := range checkers {
		wg.Add(1)
		go func(name string, checker Checker) {
			defer wg.Done()

			start := time.Now()
			result := checker(ctx)
			result.Duration = time.Since(start)
			result.Timestamp = time.Now()

			mu.Lock()
			response.Checks[name] = result

			// Update overall status
			switch result.Status {
			case StatusUnhealthy:
				response.Status = StatusUnhealthy
			case StatusDegraded:
				if response.Status != StatusUnhealthy {
					response.Status = StatusDegraded
				}
			}
			mu.Unlock()
		}(name, checker)
	}

	wg.Wait()
	return response
}

// LivenessHandler returns an HTTP handler for liveness probes.
// Liveness indicates if the service is running.
func (h *Health) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"status":    "alive",
			"timestamp": time.Now(),
		})
	}
}

// ReadinessHandler returns an HTTP handler for readiness probes.
// Readiness indicates if the service can accept traffic.
func (h *Health) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := h.Check(r.Context())

		w.Header().Set("Content-Type", "application/json")

		statusCode := http.StatusOK
		if response.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		}

		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// HealthHandler returns an HTTP handler for detailed health checks.
func (h *Health) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := h.Check(r.Context())

		w.Header().Set("Content-Type", "application/json")

		statusCode := http.StatusOK
		if response.Status == StatusUnhealthy {
			statusCode = http.StatusServiceUnavailable
		} else if response.Status == StatusDegraded {
			statusCode = http.StatusOK // Still serving, just degraded
		}

		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(response)
	}
}

// Common health check implementations

// HTTPChecker creates a checker for an HTTP endpoint.
func HTTPChecker(url string, timeout time.Duration) Checker {
	return func(ctx context.Context) *CheckResult {
		client := &http.Client{Timeout: timeout}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return &CheckResult{
				Status:  StatusUnhealthy,
				Message: "failed to create request: " + err.Error(),
			}
		}

		resp, err := client.Do(req)
		if err != nil {
			return &CheckResult{
				Status:  StatusUnhealthy,
				Message: "request failed: " + err.Error(),
			}
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return &CheckResult{
				Status:  StatusHealthy,
				Message: "OK",
			}
		}

		return &CheckResult{
			Status:  StatusUnhealthy,
			Message: "unexpected status: " + resp.Status,
		}
	}
}

// TCPChecker creates a checker for a TCP endpoint.
func TCPChecker(addr string, timeout time.Duration) Checker {
	return func(ctx context.Context) *CheckResult {
		conn, err := (&net.Dialer{Timeout: timeout}).DialContext(ctx, "tcp", addr)
		if err != nil {
			return &CheckResult{
				Status:  StatusUnhealthy,
				Message: "connection failed: " + err.Error(),
			}
		}
		conn.Close()

		return &CheckResult{
			Status:  StatusHealthy,
			Message: "OK",
		}
	}
}

// CustomChecker creates a checker from a custom function.
func CustomChecker(fn func(ctx context.Context) error) Checker {
	return func(ctx context.Context) *CheckResult {
		if err := fn(ctx); err != nil {
			return &CheckResult{
				Status:  StatusUnhealthy,
				Message: err.Error(),
			}
		}
		return &CheckResult{
			Status:  StatusHealthy,
			Message: "OK",
		}
	}
}

// AlwaysHealthy returns a checker that always reports healthy.
func AlwaysHealthy() Checker {
	return func(ctx context.Context) *CheckResult {
		return &CheckResult{
			Status:  StatusHealthy,
			Message: "OK",
		}
	}
}
