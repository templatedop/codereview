// Package tracing provides OpenTelemetry distributed tracing.
package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Config holds tracing configuration.
type Config struct {
	Enabled      bool
	Provider     string // otlp, jaeger, zipkin
	Endpoint     string
	ServiceName  string
	Environment  string
	Version      string
	SampleRate   float64
	Insecure     bool
	BatchTimeout time.Duration
}

// DefaultConfig returns default tracing configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:      false,
		Provider:     "otlp",
		Endpoint:     "localhost:4317",
		ServiceName:  "code-reviewer",
		Environment:  "development",
		Version:      "0.1.0",
		SampleRate:   1.0,
		Insecure:     true,
		BatchTimeout: 5 * time.Second,
	}
}

// Tracer wraps the OpenTelemetry tracer.
type Tracer struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
	config   Config
}

// New creates a new tracer.
func New(cfg Config) (*Tracer, error) {
	if !cfg.Enabled {
		return &Tracer{
			tracer: otel.Tracer(cfg.ServiceName),
			config: cfg,
		}, nil
	}

	ctx := context.Background()

	// Create exporter
	exporter, err := createExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create exporter: %w", err)
	}

	// Create resource
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.Version),
			semconv.DeploymentEnvironment(cfg.Environment),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("create resource: %w", err)
	}

	// Create sampler
	var sampler sdktrace.Sampler
	switch {
	case cfg.SampleRate >= 1.0:
		sampler = sdktrace.AlwaysSample()
	case cfg.SampleRate <= 0:
		sampler = sdktrace.NeverSample()
	default:
		sampler = sdktrace.TraceIDRatioBased(cfg.SampleRate)
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter, sdktrace.WithBatchTimeout(cfg.BatchTimeout)),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)

	// Set global provider
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Tracer{
		provider: provider,
		tracer:   provider.Tracer(cfg.ServiceName),
		config:   cfg,
	}, nil
}

func createExporter(ctx context.Context, cfg Config) (*otlptrace.Exporter, error) {
	opts := []otlptracegrpc.Option{
		otlptracegrpc.WithEndpoint(cfg.Endpoint),
	}

	if cfg.Insecure {
		opts = append(opts, otlptracegrpc.WithDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())))
		opts = append(opts, otlptracegrpc.WithInsecure())
	}

	return otlptracegrpc.New(ctx, opts...)
}

// Shutdown shuts down the tracer.
func (t *Tracer) Shutdown(ctx context.Context) error {
	if t.provider != nil {
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// StartSpan starts a new span.
func (t *Tracer) StartSpan(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return t.tracer.Start(ctx, name, opts...)
}

// SpanFromContext returns the span from context.
func SpanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

// Span helper methods

// SetError sets an error on the current span.
func SetError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

// SetStatus sets the status on the current span.
func SetStatus(ctx context.Context, code codes.Code, description string) {
	span := trace.SpanFromContext(ctx)
	span.SetStatus(code, description)
}

// AddEvent adds an event to the current span.
func AddEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetAttributes sets attributes on the current span.
func SetAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attrs...)
}

// Common attributes

// WorkflowAttributes returns common workflow attributes.
func WorkflowAttributes(workflowID, runID, workflowType string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("workflow.id", workflowID),
		attribute.String("workflow.run_id", runID),
		attribute.String("workflow.type", workflowType),
	}
}

// ActivityAttributes returns common activity attributes.
func ActivityAttributes(activityType, taskQueue string) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("activity.type", activityType),
		attribute.String("activity.task_queue", taskQueue),
	}
}

// LLMAttributes returns LLM request attributes.
func LLMAttributes(model string, tokens int, duration time.Duration) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("llm.model", model),
		attribute.Int("llm.tokens", tokens),
		attribute.Int64("llm.duration_ms", duration.Milliseconds()),
	}
}

// EmbeddingAttributes returns embedding request attributes.
func EmbeddingAttributes(model string, texts int, cached int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("embedding.model", model),
		attribute.Int("embedding.texts", texts),
		attribute.Int("embedding.cached", cached),
	}
}

// AnalysisAttributes returns analysis attributes.
func AnalysisAttributes(category string, issuesFound int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("analysis.category", category),
		attribute.Int("analysis.issues_found", issuesFound),
	}
}

// HTTPAttributes returns HTTP request attributes.
func HTTPAttributes(method, path string, statusCode int) []attribute.KeyValue {
	return []attribute.KeyValue{
		attribute.String("http.method", method),
		attribute.String("http.path", path),
		attribute.Int("http.status_code", statusCode),
	}
}

// Global tracer instance
var globalTracer *Tracer

// Init initializes the global tracer.
func Init(cfg Config) (*Tracer, error) {
	t, err := New(cfg)
	if err != nil {
		return nil, err
	}
	globalTracer = t
	return t, nil
}

// Global returns the global tracer.
func Global() *Tracer {
	if globalTracer == nil {
		globalTracer, _ = New(DefaultConfig())
	}
	return globalTracer
}

// Start starts a span using the global tracer.
func Start(ctx context.Context, name string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return Global().StartSpan(ctx, name, opts...)
}
