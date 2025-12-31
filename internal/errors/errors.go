// Package errors provides custom error types for the code reviewer.
package errors

import (
	"errors"
	"fmt"
	"time"
)

// Common sentinel errors
var (
	ErrNotConfigured    = errors.New("component not configured")
	ErrInvalidInput     = errors.New("invalid input")
	ErrRateLimited      = errors.New("rate limited")
	ErrTimeout          = errors.New("operation timed out")
	ErrConnectionFailed = errors.New("connection failed")
	ErrModelError       = errors.New("model error")
	ErrParseError       = errors.New("parse error")
)

// ErrorKind represents the type of error.
type ErrorKind string

const (
	KindConfiguration ErrorKind = "configuration"
	KindValidation    ErrorKind = "validation"
	KindNetwork       ErrorKind = "network"
	KindRateLimit     ErrorKind = "rate_limit"
	KindModel         ErrorKind = "model"
	KindParse         ErrorKind = "parse"
	KindInternal      ErrorKind = "internal"
)

// ReviewError is a structured error type for code review operations.
type ReviewError struct {
	Kind       ErrorKind
	Op         string // Operation being performed
	Component  string // Component that errored
	Err        error  // Underlying error
	Retryable  bool
	RetryAfter time.Duration // Suggested retry delay
}

// Error implements the error interface.
func (e *ReviewError) Error() string {
	if e.Component != "" {
		return fmt.Sprintf("%s: %s: %v", e.Component, e.Op, e.Err)
	}
	return fmt.Sprintf("%s: %v", e.Op, e.Err)
}

// Unwrap implements errors.Unwrap.
func (e *ReviewError) Unwrap() error {
	return e.Err
}

// Is implements errors.Is.
func (e *ReviewError) Is(target error) bool {
	if t, ok := target.(*ReviewError); ok {
		return e.Kind == t.Kind
	}
	return errors.Is(e.Err, target)
}

// NewReviewError creates a new ReviewError.
func NewReviewError(kind ErrorKind, op, component string, err error) *ReviewError {
	return &ReviewError{
		Kind:      kind,
		Op:        op,
		Component: component,
		Err:       err,
		Retryable: kind == KindNetwork || kind == KindRateLimit,
	}
}

// Configuration errors
func NotConfigured(component string) *ReviewError {
	return &ReviewError{
		Kind:      KindConfiguration,
		Op:        "initialize",
		Component: component,
		Err:       ErrNotConfigured,
		Retryable: false,
	}
}

// Validation errors
func InvalidInput(op string, msg string) *ReviewError {
	return &ReviewError{
		Kind:      KindValidation,
		Op:        op,
		Err:       fmt.Errorf("%w: %s", ErrInvalidInput, msg),
		Retryable: false,
	}
}

// Network errors
func NetworkError(op, component string, err error) *ReviewError {
	return &ReviewError{
		Kind:       KindNetwork,
		Op:         op,
		Component:  component,
		Err:        fmt.Errorf("%w: %v", ErrConnectionFailed, err),
		Retryable:  true,
		RetryAfter: time.Second,
	}
}

// Rate limit errors
func RateLimitError(component string, retryAfter time.Duration) *ReviewError {
	return &ReviewError{
		Kind:       KindRateLimit,
		Op:         "request",
		Component:  component,
		Err:        ErrRateLimited,
		Retryable:  true,
		RetryAfter: retryAfter,
	}
}

// Model errors
func ModelError(op string, err error) *ReviewError {
	return &ReviewError{
		Kind:      KindModel,
		Op:        op,
		Err:       fmt.Errorf("%w: %v", ErrModelError, err),
		Retryable: false,
	}
}

// Parse errors
func ParseError(op string, err error) *ReviewError {
	return &ReviewError{
		Kind:      KindParse,
		Op:        op,
		Err:       fmt.Errorf("%w: %v", ErrParseError, err),
		Retryable: false,
	}
}

// IsRetryable checks if an error is retryable.
func IsRetryable(err error) bool {
	var re *ReviewError
	if errors.As(err, &re) {
		return re.Retryable
	}
	return false
}

// GetRetryAfter returns the suggested retry delay.
func GetRetryAfter(err error) time.Duration {
	var re *ReviewError
	if errors.As(err, &re) && re.RetryAfter > 0 {
		return re.RetryAfter
	}
	return time.Second // Default
}

// IsKind checks if an error is of a specific kind.
func IsKind(err error, kind ErrorKind) bool {
	var re *ReviewError
	if errors.As(err, &re) {
		return re.Kind == kind
	}
	return false
}

// MultiError combines multiple errors.
type MultiError struct {
	Errors []error
}

// Error implements the error interface.
func (m *MultiError) Error() string {
	if len(m.Errors) == 0 {
		return "no errors"
	}
	if len(m.Errors) == 1 {
		return m.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred; first: %v", len(m.Errors), m.Errors[0])
}

// Add adds an error to the multi-error.
func (m *MultiError) Add(err error) {
	if err != nil {
		m.Errors = append(m.Errors, err)
	}
}

// HasErrors returns true if there are any errors.
func (m *MultiError) HasErrors() bool {
	return len(m.Errors) > 0
}

// ErrorOrNil returns nil if there are no errors.
func (m *MultiError) ErrorOrNil() error {
	if m.HasErrors() {
		return m
	}
	return nil
}

// Wrap wraps an error with additional context.
func Wrap(err error, msg string) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", msg, err)
}

// Wrapf wraps an error with formatted context.
func Wrapf(err error, format string, args ...interface{}) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", fmt.Sprintf(format, args...), err)
}
