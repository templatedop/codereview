// Package shutdown provides graceful shutdown functionality.
package shutdown

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/yourorg/code-reviewer/internal/logging"
)

// Manager handles graceful shutdown of services.
type Manager struct {
	mu           sync.Mutex
	hooks        []Hook
	timeout      time.Duration
	logger       *logging.Logger
	shutdownChan chan struct{}
	done         chan struct{}
	inFlight     atomic.Int64
	draining     atomic.Bool
}

// Hook represents a shutdown hook function.
type Hook struct {
	Name     string
	Priority int // Lower priority runs first
	Fn       func(ctx context.Context) error
}

// Config holds shutdown configuration.
type Config struct {
	Timeout time.Duration
	Logger  *logging.Logger
}

// DefaultConfig returns default shutdown configuration.
func DefaultConfig() Config {
	return Config{
		Timeout: 30 * time.Second,
		Logger:  logging.Default(),
	}
}

// New creates a new shutdown manager.
func New(cfg Config) *Manager {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.Logger == nil {
		cfg.Logger = logging.Default()
	}

	return &Manager{
		timeout:      cfg.Timeout,
		logger:       cfg.Logger,
		shutdownChan: make(chan struct{}),
		done:         make(chan struct{}),
	}
}

// Register adds a shutdown hook.
func (m *Manager) Register(name string, priority int, fn func(ctx context.Context) error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.hooks = append(m.hooks, Hook{
		Name:     name,
		Priority: priority,
		Fn:       fn,
	})
}

// RegisterServer registers an HTTP server for graceful shutdown.
func (m *Manager) RegisterServer(name string, shutdownFn func(ctx context.Context) error) {
	m.Register(name, 10, shutdownFn)
}

// RegisterWorker registers a Temporal worker for graceful shutdown.
func (m *Manager) RegisterWorker(name string, stopFn func()) {
	m.Register(name, 20, func(ctx context.Context) error {
		stopFn()
		return nil
	})
}

// RegisterCleanup registers a cleanup function (runs last).
func (m *Manager) RegisterCleanup(name string, fn func(ctx context.Context) error) {
	m.Register(name, 100, fn)
}

// TrackRequest tracks an in-flight request.
func (m *Manager) TrackRequest() func() {
	m.inFlight.Add(1)
	return func() {
		m.inFlight.Add(-1)
	}
}

// InFlight returns the number of in-flight requests.
func (m *Manager) InFlight() int64 {
	return m.inFlight.Load()
}

// IsDraining returns true if the server is draining.
func (m *Manager) IsDraining() bool {
	return m.draining.Load()
}

// WaitForSignal blocks until a shutdown signal is received.
func (m *Manager) WaitForSignal() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	select {
	case sig := <-sigChan:
		m.logger.Info("received shutdown signal", "signal", sig.String())
	case <-m.shutdownChan:
		m.logger.Info("shutdown requested programmatically")
	}
}

// Shutdown initiates graceful shutdown.
func (m *Manager) Shutdown() {
	close(m.shutdownChan)
}

// Execute runs all shutdown hooks in priority order.
func (m *Manager) Execute() error {
	m.draining.Store(true)

	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	defer cancel()

	// Wait for in-flight requests to complete
	m.logger.Info("waiting for in-flight requests", "count", m.inFlight.Load())
	if err := m.waitForDrain(ctx); err != nil {
		m.logger.Warn("timeout waiting for requests to drain", "remaining", m.inFlight.Load())
	}

	// Sort hooks by priority
	m.mu.Lock()
	hooks := make([]Hook, len(m.hooks))
	copy(hooks, m.hooks)
	m.mu.Unlock()

	// Sort by priority (simple bubble sort for small slice)
	for i := 0; i < len(hooks)-1; i++ {
		for j := 0; j < len(hooks)-i-1; j++ {
			if hooks[j].Priority > hooks[j+1].Priority {
				hooks[j], hooks[j+1] = hooks[j+1], hooks[j]
			}
		}
	}

	// Execute hooks
	var errs []error
	for _, hook := range hooks {
		m.logger.Info("executing shutdown hook", "name", hook.Name, "priority", hook.Priority)

		start := time.Now()
		if err := hook.Fn(ctx); err != nil {
			m.logger.Error("shutdown hook failed",
				"name", hook.Name,
				"error", err.Error(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
			errs = append(errs, err)
		} else {
			m.logger.Info("shutdown hook completed",
				"name", hook.Name,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}
	}

	close(m.done)

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// Done returns a channel that's closed when shutdown is complete.
func (m *Manager) Done() <-chan struct{} {
	return m.done
}

// waitForDrain waits for all in-flight requests to complete.
func (m *Manager) waitForDrain(ctx context.Context) error {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if m.inFlight.Load() == 0 {
				return nil
			}
		}
	}
}

// GracefulHandler wraps an HTTP handler with graceful shutdown support.
func (m *Manager) GracefulHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m.IsDraining() {
			w.Header().Set("Connection", "close")
			http.Error(w, "server is shutting down", http.StatusServiceUnavailable)
			return
		}

		done := m.TrackRequest()
		defer done()

		next.ServeHTTP(w, r)
	})
}
