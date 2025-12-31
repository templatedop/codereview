package errors

import (
	"context"
	"math"
	"math/rand"
	"time"
)

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxAttempts is the maximum number of attempts (including the initial attempt).
	MaxAttempts int
	// InitialDelay is the initial delay between retries.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier is the factor by which delay increases after each retry.
	Multiplier float64
	// Jitter adds randomness to delays (0.0 to 1.0).
	Jitter float64
	// RetryIf determines whether to retry for a given error.
	RetryIf func(error) bool
}

// DefaultRetryConfig returns sensible defaults.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		RetryIf:      IsRetryable,
	}
}

// Retry executes a function with retry logic.
func Retry(ctx context.Context, config RetryConfig, fn func() error) error {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 1
	}
	if config.InitialDelay <= 0 {
		config.InitialDelay = 100 * time.Millisecond
	}
	if config.Multiplier <= 0 {
		config.Multiplier = 2.0
	}
	if config.RetryIf == nil {
		config.RetryIf = IsRetryable
	}

	var lastErr error
	delay := config.InitialDelay

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check context before attempting
		if err := ctx.Err(); err != nil {
			return err
		}

		// Execute the function
		lastErr = fn()
		if lastErr == nil {
			return nil
		}

		// Check if we should retry
		if !config.RetryIf(lastErr) {
			return lastErr
		}

		// Don't wait if this was the last attempt
		if attempt >= config.MaxAttempts {
			break
		}

		// Calculate delay with jitter
		actualDelay := delay
		if config.Jitter > 0 {
			jitter := float64(delay) * config.Jitter * (rand.Float64()*2 - 1)
			actualDelay = time.Duration(float64(delay) + jitter)
		}

		// Use retry-after hint if available
		if retryAfter := GetRetryAfter(lastErr); retryAfter > actualDelay {
			actualDelay = retryAfter
		}

		// Wait for delay or context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(actualDelay):
		}

		// Increase delay for next attempt
		delay = time.Duration(float64(delay) * config.Multiplier)
		if config.MaxDelay > 0 && delay > config.MaxDelay {
			delay = config.MaxDelay
		}
	}

	return lastErr
}

// RetryWithResult executes a function that returns a value with retry logic.
func RetryWithResult[T any](ctx context.Context, config RetryConfig, fn func() (T, error)) (T, error) {
	var result T
	err := Retry(ctx, config, func() error {
		var innerErr error
		result, innerErr = fn()
		return innerErr
	})
	return result, err
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	maxFailures    int
	resetTimeout   time.Duration
	failureCount   int
	lastFailure    time.Time
	state          CircuitState
}

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// NewCircuitBreaker creates a new circuit breaker.
func NewCircuitBreaker(maxFailures int, resetTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  maxFailures,
		resetTimeout: resetTimeout,
		state:        CircuitClosed,
	}
}

// Execute runs a function through the circuit breaker.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.AllowRequest() {
		return &ReviewError{
			Kind:       KindNetwork,
			Op:         "circuit_breaker",
			Err:        ErrConnectionFailed,
			Retryable:  true,
			RetryAfter: cb.resetTimeout,
		}
	}

	err := fn()
	cb.RecordResult(err)
	return err
}

// AllowRequest checks if a request should be allowed.
func (cb *CircuitBreaker) AllowRequest() bool {
	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if reset timeout has passed
		if time.Since(cb.lastFailure) > cb.resetTimeout {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	}
	return true
}

// RecordResult records the result of an operation.
func (cb *CircuitBreaker) RecordResult(err error) {
	if err == nil {
		cb.failureCount = 0
		cb.state = CircuitClosed
		return
	}

	if !IsRetryable(err) {
		// Don't count non-retryable errors
		return
	}

	cb.failureCount++
	cb.lastFailure = time.Now()

	if cb.failureCount >= cb.maxFailures {
		cb.state = CircuitOpen
	}
}

// State returns the current state.
func (cb *CircuitBreaker) State() CircuitState {
	return cb.state
}

// Reset resets the circuit breaker.
func (cb *CircuitBreaker) Reset() {
	cb.failureCount = 0
	cb.state = CircuitClosed
}

// ExponentialBackoff calculates exponential backoff delay.
func ExponentialBackoff(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	delay := time.Duration(float64(baseDelay) * math.Pow(2, float64(attempt)))
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

// JitterDelay adds jitter to a delay.
func JitterDelay(delay time.Duration, jitterPercent float64) time.Duration {
	if jitterPercent <= 0 {
		return delay
	}
	jitter := float64(delay) * jitterPercent * rand.Float64()
	return delay + time.Duration(jitter)
}
