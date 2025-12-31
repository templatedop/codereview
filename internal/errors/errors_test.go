package errors

import (
	"errors"
	"testing"
	"time"
)

func TestReviewError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *ReviewError
		contains string
	}{
		{
			name: "with component",
			err: &ReviewError{
				Kind:      KindNetwork,
				Op:        "connect",
				Component: "embedder",
				Err:       errors.New("connection refused"),
			},
			contains: "embedder: connect: connection refused",
		},
		{
			name: "without component",
			err: &ReviewError{
				Kind: KindValidation,
				Op:   "validate",
				Err:  errors.New("empty input"),
			},
			contains: "validate: empty input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.contains {
				t.Errorf("Error() = %q, want %q", got, tt.contains)
			}
		})
	}
}

func TestReviewError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	err := &ReviewError{
		Kind: KindInternal,
		Op:   "test",
		Err:  inner,
	}

	if !errors.Is(err, inner) {
		t.Error("Unwrap() should allow errors.Is to find inner error")
	}
}

func TestReviewError_Is(t *testing.T) {
	err := &ReviewError{
		Kind: KindNetwork,
		Op:   "test",
		Err:  ErrConnectionFailed,
	}

	// Should match same kind
	target := &ReviewError{Kind: KindNetwork}
	if !errors.Is(err, target) {
		t.Error("Is() should match same kind")
	}

	// Should match wrapped error
	if !errors.Is(err, ErrConnectionFailed) {
		t.Error("Is() should match wrapped error")
	}
}

func TestNotConfigured(t *testing.T) {
	err := NotConfigured("analyzer")

	if err.Kind != KindConfiguration {
		t.Errorf("Kind = %v, want %v", err.Kind, KindConfiguration)
	}
	if err.Component != "analyzer" {
		t.Errorf("Component = %q, want %q", err.Component, "analyzer")
	}
	if err.Retryable {
		t.Error("NotConfigured should not be retryable")
	}
}

func TestInvalidInput(t *testing.T) {
	err := InvalidInput("parse", "empty query")

	if err.Kind != KindValidation {
		t.Errorf("Kind = %v, want %v", err.Kind, KindValidation)
	}
	if !errors.Is(err, ErrInvalidInput) {
		t.Error("Should wrap ErrInvalidInput")
	}
}

func TestNetworkError(t *testing.T) {
	err := NetworkError("connect", "embedder", errors.New("timeout"))

	if err.Kind != KindNetwork {
		t.Errorf("Kind = %v, want %v", err.Kind, KindNetwork)
	}
	if !err.Retryable {
		t.Error("NetworkError should be retryable")
	}
	if err.RetryAfter <= 0 {
		t.Error("NetworkError should have RetryAfter")
	}
}

func TestRateLimitError(t *testing.T) {
	err := RateLimitError("openai", 30*time.Second)

	if err.Kind != KindRateLimit {
		t.Errorf("Kind = %v, want %v", err.Kind, KindRateLimit)
	}
	if !err.Retryable {
		t.Error("RateLimitError should be retryable")
	}
	if err.RetryAfter != 30*time.Second {
		t.Errorf("RetryAfter = %v, want %v", err.RetryAfter, 30*time.Second)
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "network error",
			err:  NetworkError("connect", "test", errors.New("fail")),
			want: true,
		},
		{
			name: "rate limit",
			err:  RateLimitError("test", time.Second),
			want: true,
		},
		{
			name: "validation error",
			err:  InvalidInput("test", "bad input"),
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("plain error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetRetryAfter(t *testing.T) {
	err := RateLimitError("test", 5*time.Second)
	if got := GetRetryAfter(err); got != 5*time.Second {
		t.Errorf("GetRetryAfter() = %v, want %v", got, 5*time.Second)
	}

	// Default for errors without RetryAfter
	plainErr := errors.New("plain error")
	if got := GetRetryAfter(plainErr); got != time.Second {
		t.Errorf("GetRetryAfter() default = %v, want %v", got, time.Second)
	}
}

func TestIsKind(t *testing.T) {
	err := NetworkError("connect", "test", errors.New("fail"))

	if !IsKind(err, KindNetwork) {
		t.Error("IsKind() should match network kind")
	}
	if IsKind(err, KindValidation) {
		t.Error("IsKind() should not match validation kind")
	}
}

func TestMultiError(t *testing.T) {
	me := &MultiError{}

	if me.HasErrors() {
		t.Error("Empty MultiError should not have errors")
	}
	if me.ErrorOrNil() != nil {
		t.Error("Empty MultiError should return nil")
	}

	me.Add(errors.New("error 1"))
	me.Add(nil) // Should be ignored
	me.Add(errors.New("error 2"))

	if !me.HasErrors() {
		t.Error("MultiError should have errors")
	}
	if len(me.Errors) != 2 {
		t.Errorf("len(Errors) = %d, want 2", len(me.Errors))
	}
	if me.ErrorOrNil() == nil {
		t.Error("Non-empty MultiError should return error")
	}
}

func TestWrap(t *testing.T) {
	inner := errors.New("inner error")
	wrapped := Wrap(inner, "context")

	if wrapped == nil {
		t.Fatal("Wrap() returned nil")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("Wrapped error should match inner error")
	}

	// Wrap nil should return nil
	if Wrap(nil, "context") != nil {
		t.Error("Wrap(nil) should return nil")
	}
}

func TestWrapf(t *testing.T) {
	inner := errors.New("inner")
	wrapped := Wrapf(inner, "operation %s failed", "test")

	if wrapped == nil {
		t.Fatal("Wrapf() returned nil")
	}
	if !errors.Is(wrapped, inner) {
		t.Error("Wrapped error should match inner error")
	}
}
