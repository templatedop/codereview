// Package logging provides structured logging for the code reviewer.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
	"time"
)

// contextKey is the type for context keys.
type contextKey string

const (
	// RequestIDKey is the context key for request ID.
	RequestIDKey contextKey = "request_id"
	// WorkflowIDKey is the context key for workflow ID.
	WorkflowIDKey contextKey = "workflow_id"
	// ActivityIDKey is the context key for activity ID.
	ActivityIDKey contextKey = "activity_id"
)

// Logger wraps slog.Logger with additional functionality.
type Logger struct {
	*slog.Logger
}

// Config holds logger configuration.
type Config struct {
	Level     string // debug, info, warn, error
	Format    string // json, text
	Output    string // stdout, stderr, file
	FilePath  string // path to log file if Output is "file"
	AddSource bool   // add source file and line to logs
}

// DefaultConfig returns a default logger configuration.
func DefaultConfig() Config {
	return Config{
		Level:     "info",
		Format:    "json",
		Output:    "stdout",
		AddSource: false,
	}
}

// New creates a new Logger with the given configuration.
func New(cfg Config) (*Logger, error) {
	level := parseLevel(cfg.Level)

	var writer io.Writer
	switch strings.ToLower(cfg.Output) {
	case "stderr":
		writer = os.Stderr
	case "file":
		if cfg.FilePath == "" {
			writer = os.Stdout
		} else {
			f, err := os.OpenFile(cfg.FilePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return nil, err
			}
			writer = f
		}
	default:
		writer = os.Stdout
	}

	opts := &slog.HandlerOptions{
		Level:     level,
		AddSource: cfg.AddSource,
	}

	var handler slog.Handler
	switch strings.ToLower(cfg.Format) {
	case "text":
		handler = slog.NewTextHandler(writer, opts)
	default:
		handler = slog.NewJSONHandler(writer, opts)
	}

	return &Logger{slog.New(handler)}, nil
}

// NewDefault creates a logger with default settings.
func NewDefault() *Logger {
	logger, _ := New(DefaultConfig())
	return logger
}

// WithContext returns a logger with context values.
func (l *Logger) WithContext(ctx context.Context) *Logger {
	logger := l.Logger

	if requestID, ok := ctx.Value(RequestIDKey).(string); ok && requestID != "" {
		logger = logger.With("request_id", requestID)
	}
	if workflowID, ok := ctx.Value(WorkflowIDKey).(string); ok && workflowID != "" {
		logger = logger.With("workflow_id", workflowID)
	}
	if activityID, ok := ctx.Value(ActivityIDKey).(string); ok && activityID != "" {
		logger = logger.With("activity_id", activityID)
	}

	return &Logger{logger}
}

// WithComponent returns a logger with the component field set.
func (l *Logger) WithComponent(component string) *Logger {
	return &Logger{l.Logger.With("component", component)}
}

// WithError returns a logger with the error field set.
func (l *Logger) WithError(err error) *Logger {
	if err == nil {
		return l
	}
	return &Logger{l.Logger.With("error", err.Error())}
}

// WithDuration returns a logger with a duration field.
func (l *Logger) WithDuration(d time.Duration) *Logger {
	return &Logger{l.Logger.With("duration_ms", d.Milliseconds())}
}

// WithFields returns a logger with additional fields.
func (l *Logger) WithFields(fields map[string]any) *Logger {
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{l.Logger.With(args...)}
}

// Helpers for common logging patterns

// LogRequest logs an incoming request.
func (l *Logger) LogRequest(method, path string, statusCode int, duration time.Duration) {
	l.Info("request",
		"method", method,
		"path", path,
		"status", statusCode,
		"duration_ms", duration.Milliseconds(),
	)
}

// LogActivity logs a Temporal activity.
func (l *Logger) LogActivity(name string, duration time.Duration, err error) {
	if err != nil {
		l.Error("activity failed",
			"activity", name,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
	} else {
		l.Info("activity completed",
			"activity", name,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// LogLLMCall logs an LLM API call.
func (l *Logger) LogLLMCall(model string, tokens int, duration time.Duration, err error) {
	if err != nil {
		l.Error("llm call failed",
			"model", model,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
	} else {
		l.Info("llm call completed",
			"model", model,
			"tokens", tokens,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// LogEmbeddingCall logs an embedding API call.
func (l *Logger) LogEmbeddingCall(model string, texts int, cached int, duration time.Duration, err error) {
	if err != nil {
		l.Error("embedding call failed",
			"model", model,
			"texts", texts,
			"duration_ms", duration.Milliseconds(),
			"error", err.Error(),
		)
	} else {
		l.Debug("embedding call completed",
			"model", model,
			"texts", texts,
			"cached", cached,
			"duration_ms", duration.Milliseconds(),
		)
	}
}

// parseLevel parses a log level string.
func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ContextWithRequestID adds a request ID to the context.
func ContextWithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, RequestIDKey, requestID)
}

// ContextWithWorkflowID adds a workflow ID to the context.
func ContextWithWorkflowID(ctx context.Context, workflowID string) context.Context {
	return context.WithValue(ctx, WorkflowIDKey, workflowID)
}

// ContextWithActivityID adds an activity ID to the context.
func ContextWithActivityID(ctx context.Context, activityID string) context.Context {
	return context.WithValue(ctx, ActivityIDKey, activityID)
}

// Global logger instance
var defaultLogger = NewDefault()

// SetDefault sets the default logger.
func SetDefault(l *Logger) {
	defaultLogger = l
	slog.SetDefault(l.Logger)
}

// Default returns the default logger.
func Default() *Logger {
	return defaultLogger
}

// Convenience functions using the default logger

// Debug logs at debug level.
func Debug(msg string, args ...any) {
	defaultLogger.Debug(msg, args...)
}

// Info logs at info level.
func Info(msg string, args ...any) {
	defaultLogger.Info(msg, args...)
}

// Warn logs at warn level.
func Warn(msg string, args ...any) {
	defaultLogger.Warn(msg, args...)
}

// Error logs at error level.
func Error(msg string, args ...any) {
	defaultLogger.Error(msg, args...)
}
