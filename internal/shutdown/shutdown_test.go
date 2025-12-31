package shutdown

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/yourorg/code-reviewer/internal/logging"
)

func TestManager_Register(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	m.Register("test", 1, func(ctx context.Context) error {
		return nil
	})

	if len(m.hooks) != 1 {
		t.Errorf("hooks count = %d, want 1", len(m.hooks))
	}
	if m.hooks[0].Name != "test" {
		t.Errorf("hook name = %q, want %q", m.hooks[0].Name, "test")
	}
}

func TestManager_TrackRequest(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	if m.InFlight() != 0 {
		t.Errorf("initial InFlight() = %d, want 0", m.InFlight())
	}

	done1 := m.TrackRequest()
	done2 := m.TrackRequest()

	if m.InFlight() != 2 {
		t.Errorf("InFlight() = %d, want 2", m.InFlight())
	}

	done1()
	if m.InFlight() != 1 {
		t.Errorf("InFlight() after done1 = %d, want 1", m.InFlight())
	}

	done2()
	if m.InFlight() != 0 {
		t.Errorf("InFlight() after done2 = %d, want 0", m.InFlight())
	}
}

func TestManager_IsDraining(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	if m.IsDraining() {
		t.Error("IsDraining() = true before shutdown")
	}

	// Start shutdown in background
	go m.Execute()

	// Wait a bit for draining to start
	time.Sleep(50 * time.Millisecond)

	if !m.IsDraining() {
		t.Error("IsDraining() = false during shutdown")
	}
}

func TestManager_Execute_Order(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	var order []int
	m.Register("third", 30, func(ctx context.Context) error {
		order = append(order, 3)
		return nil
	})
	m.Register("first", 10, func(ctx context.Context) error {
		order = append(order, 1)
		return nil
	})
	m.Register("second", 20, func(ctx context.Context) error {
		order = append(order, 2)
		return nil
	})

	if err := m.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("order length = %d, want 3", len(order))
	}
	if order[0] != 1 || order[1] != 2 || order[2] != 3 {
		t.Errorf("order = %v, want [1, 2, 3]", order)
	}
}

func TestManager_Execute_Error(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	m.Register("failing", 1, func(ctx context.Context) error {
		return errors.New("hook failed")
	})
	m.Register("succeeding", 2, func(ctx context.Context) error {
		return nil
	})

	err := m.Execute()
	if err == nil {
		t.Fatal("Execute() should return error")
	}
}

func TestManager_Execute_WaitForDrain(t *testing.T) {
	m := New(Config{
		Timeout: 2 * time.Second,
		Logger:  logging.NewDefault(),
	})

	hookExecuted := false
	m.Register("hook", 1, func(ctx context.Context) error {
		hookExecuted = true
		return nil
	})

	// Simulate in-flight request
	done := m.TrackRequest()

	// Start shutdown in background
	go func() {
		time.Sleep(100 * time.Millisecond)
		done() // Complete the request
	}()

	if err := m.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if !hookExecuted {
		t.Error("Hook should have been executed")
	}
}

func TestManager_GracefulHandler(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.GracefulHandler(handler)

	// Normal request
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestManager_GracefulHandler_Draining(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.GracefulHandler(handler)

	// Start draining
	m.draining.Store(true)

	// Request during drain
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("Status during drain = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}

	// Check Connection header
	if rec.Header().Get("Connection") != "close" {
		t.Error("Connection header should be 'close' during drain")
	}
}

func TestManager_GracefulHandler_TracksRequests(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	var requestInFlight int64
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.StoreInt64(&requestInFlight, m.InFlight())
		w.WriteHeader(http.StatusOK)
	})

	wrapped := m.GracefulHandler(handler)

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	wrapped.ServeHTTP(rec, req)

	if requestInFlight != 1 {
		t.Errorf("InFlight during request = %d, want 1", requestInFlight)
	}

	if m.InFlight() != 0 {
		t.Errorf("InFlight after request = %d, want 0", m.InFlight())
	}
}

func TestManager_RegisterServer(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	called := false
	m.RegisterServer("http", func(ctx context.Context) error {
		called = true
		return nil
	})

	if len(m.hooks) != 1 {
		t.Errorf("hooks count = %d, want 1", len(m.hooks))
	}
	if m.hooks[0].Priority != 10 {
		t.Errorf("server hook priority = %d, want 10", m.hooks[0].Priority)
	}

	m.Execute()
	if !called {
		t.Error("Server shutdown should be called")
	}
}

func TestManager_RegisterWorker(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	called := false
	m.RegisterWorker("temporal", func() {
		called = true
	})

	if len(m.hooks) != 1 {
		t.Errorf("hooks count = %d, want 1", len(m.hooks))
	}
	if m.hooks[0].Priority != 20 {
		t.Errorf("worker hook priority = %d, want 20", m.hooks[0].Priority)
	}

	m.Execute()
	if !called {
		t.Error("Worker stop should be called")
	}
}

func TestManager_RegisterCleanup(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	called := false
	m.RegisterCleanup("cache", func(ctx context.Context) error {
		called = true
		return nil
	})

	if m.hooks[0].Priority != 100 {
		t.Errorf("cleanup hook priority = %d, want 100", m.hooks[0].Priority)
	}

	m.Execute()
	if !called {
		t.Error("Cleanup should be called")
	}
}

func TestManager_Done(t *testing.T) {
	m := New(Config{
		Timeout: time.Second,
		Logger:  logging.NewDefault(),
	})

	done := m.Done()

	select {
	case <-done:
		t.Error("Done channel should not be closed yet")
	default:
		// Expected
	}

	m.Execute()

	select {
	case <-done:
		// Expected
	case <-time.After(time.Second):
		t.Error("Done channel should be closed after Execute")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.Logger == nil {
		t.Error("Logger should not be nil")
	}
}
