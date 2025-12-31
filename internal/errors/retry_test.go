package errors

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRetry_Success(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), DefaultRetryConfig(), func() error {
		attempts++
		return nil
	})

	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	attempts := 0
	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  5,
		InitialDelay: 10 * time.Millisecond,
		Multiplier:   2.0,
		RetryIf:      func(error) bool { return true },
	}, func() error {
		attempts++
		if attempts < 3 {
			return NetworkError("test", "component", errors.New("temporary"))
		}
		return nil
	})

	if err != nil {
		t.Fatalf("Retry() error = %v", err)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_MaxAttempts(t *testing.T) {
	attempts := 0
	expectedErr := NetworkError("test", "component", errors.New("persistent"))

	err := Retry(context.Background(), RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 10 * time.Millisecond,
		Multiplier:   2.0,
		RetryIf:      func(error) bool { return true },
	}, func() error {
		attempts++
		return expectedErr
	})

	if err == nil {
		t.Fatal("Retry() should return error")
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	attempts := 0
	expectedErr := InvalidInput("test", "bad input")

	err := Retry(context.Background(), DefaultRetryConfig(), func() error {
		attempts++
		return expectedErr
	})

	if err == nil {
		t.Fatal("Retry() should return error")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (non-retryable)", attempts)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	attempts := 0
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	err := Retry(ctx, RetryConfig{
		MaxAttempts:  10,
		InitialDelay: 100 * time.Millisecond,
		RetryIf:      func(error) bool { return true },
	}, func() error {
		attempts++
		return NetworkError("test", "component", errors.New("fail"))
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Expected context.Canceled, got %v", err)
	}
}

func TestRetryWithResult(t *testing.T) {
	attempts := 0
	result, err := RetryWithResult(context.Background(), DefaultRetryConfig(), func() (int, error) {
		attempts++
		if attempts < 2 {
			return 0, NetworkError("test", "component", errors.New("temp"))
		}
		return 42, nil
	})

	if err != nil {
		t.Fatalf("RetryWithResult() error = %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42", result)
	}
}

func TestCircuitBreaker_Closed(t *testing.T) {
	cb := NewCircuitBreaker(3, time.Second)

	// Should allow requests when closed
	if !cb.AllowRequest() {
		t.Error("Closed circuit should allow requests")
	}

	// Execute successful operations
	for i := 0; i < 5; i++ {
		err := cb.Execute(func() error { return nil })
		if err != nil {
			t.Errorf("Execute() error = %v", err)
		}
	}

	if cb.State() != CircuitClosed {
		t.Errorf("State() = %v, want CircuitClosed", cb.State())
	}
}

func TestCircuitBreaker_Open(t *testing.T) {
	cb := NewCircuitBreaker(3, 100*time.Millisecond)

	// Trigger failures to open circuit
	for i := 0; i < 3; i++ {
		cb.Execute(func() error {
			return NetworkError("test", "component", errors.New("fail"))
		})
	}

	if cb.State() != CircuitOpen {
		t.Errorf("State() = %v, want CircuitOpen", cb.State())
	}

	// Should not allow requests when open
	if cb.AllowRequest() {
		t.Error("Open circuit should not allow requests")
	}

	// Execute should return circuit breaker error
	err := cb.Execute(func() error { return nil })
	if err == nil {
		t.Error("Execute() on open circuit should return error")
	}
}

func TestCircuitBreaker_HalfOpen(t *testing.T) {
	cb := NewCircuitBreaker(2, 50*time.Millisecond)

	// Trigger failures to open circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return NetworkError("test", "component", errors.New("fail"))
		})
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("State() = %v, want CircuitOpen", cb.State())
	}

	// Wait for reset timeout
	time.Sleep(60 * time.Millisecond)

	// Should transition to half-open and allow request
	if !cb.AllowRequest() {
		t.Error("Should allow request after reset timeout")
	}

	// Successful request should close circuit
	cb.Execute(func() error { return nil })

	if cb.State() != CircuitClosed {
		t.Errorf("State() = %v, want CircuitClosed after success", cb.State())
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	cb := NewCircuitBreaker(2, time.Second)

	// Open the circuit
	for i := 0; i < 2; i++ {
		cb.Execute(func() error {
			return NetworkError("test", "component", errors.New("fail"))
		})
	}

	if cb.State() != CircuitOpen {
		t.Fatalf("State() = %v, want CircuitOpen", cb.State())
	}

	// Reset should close it
	cb.Reset()

	if cb.State() != CircuitClosed {
		t.Errorf("State() after Reset() = %v, want CircuitClosed", cb.State())
	}
	if !cb.AllowRequest() {
		t.Error("Should allow requests after Reset()")
	}
}

func TestExponentialBackoff(t *testing.T) {
	tests := []struct {
		attempt   int
		baseDelay time.Duration
		maxDelay  time.Duration
		want      time.Duration
	}{
		{0, time.Second, time.Minute, time.Second},
		{1, time.Second, time.Minute, 2 * time.Second},
		{2, time.Second, time.Minute, 4 * time.Second},
		{10, time.Second, 30 * time.Second, 30 * time.Second}, // Capped at max
	}

	for _, tt := range tests {
		got := ExponentialBackoff(tt.attempt, tt.baseDelay, tt.maxDelay)
		if got != tt.want {
			t.Errorf("ExponentialBackoff(%d, %v, %v) = %v, want %v",
				tt.attempt, tt.baseDelay, tt.maxDelay, got, tt.want)
		}
	}
}

func TestJitterDelay(t *testing.T) {
	delay := time.Second

	// With no jitter
	if got := JitterDelay(delay, 0); got != delay {
		t.Errorf("JitterDelay with 0 jitter = %v, want %v", got, delay)
	}

	// With jitter - should be within range
	for i := 0; i < 100; i++ {
		got := JitterDelay(delay, 0.5)
		if got < delay || got > delay+time.Duration(float64(delay)*0.5) {
			t.Errorf("JitterDelay with 0.5 jitter = %v, outside expected range", got)
		}
	}
}

func TestDefaultRetryConfig(t *testing.T) {
	cfg := DefaultRetryConfig()

	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", cfg.MaxAttempts)
	}
	if cfg.InitialDelay != 100*time.Millisecond {
		t.Errorf("InitialDelay = %v, want 100ms", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 10*time.Second {
		t.Errorf("MaxDelay = %v, want 10s", cfg.MaxDelay)
	}
	if cfg.RetryIf == nil {
		t.Error("RetryIf should not be nil")
	}
}
