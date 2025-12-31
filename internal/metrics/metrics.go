// Package metrics provides Prometheus metrics for the code reviewer.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all application metrics.
type Metrics struct {
	namespace string
	registry  *prometheus.Registry

	// Request metrics
	RequestsTotal   *prometheus.CounterVec
	RequestDuration *prometheus.HistogramVec
	RequestsInFlight prometheus.Gauge

	// Workflow metrics
	WorkflowsStarted   *prometheus.CounterVec
	WorkflowsCompleted *prometheus.CounterVec
	WorkflowDuration   *prometheus.HistogramVec
	ActivitiesTotal    *prometheus.CounterVec
	ActivityDuration   *prometheus.HistogramVec

	// LLM metrics
	LLMRequestsTotal    *prometheus.CounterVec
	LLMRequestDuration  *prometheus.HistogramVec
	LLMTokensTotal      *prometheus.CounterVec
	LLMErrors           *prometheus.CounterVec

	// Embedding metrics
	EmbeddingRequestsTotal   *prometheus.CounterVec
	EmbeddingRequestDuration *prometheus.HistogramVec
	EmbeddingCacheHits       prometheus.Counter
	EmbeddingCacheMisses     prometheus.Counter
	EmbeddingCacheSize       prometheus.Gauge

	// Vector store metrics
	VectorStoreDocuments *prometheus.GaugeVec
	VectorStoreSearches  *prometheus.CounterVec
	VectorSearchDuration *prometheus.HistogramVec

	// Analysis metrics
	AnalysisIssuesFound *prometheus.CounterVec
	AnalysisDuration    *prometheus.HistogramVec

	// Error metrics
	ErrorsTotal *prometheus.CounterVec
}

// Config holds metrics configuration.
type Config struct {
	Namespace string
	Subsystem string
}

// DefaultConfig returns default metrics configuration.
func DefaultConfig() Config {
	return Config{
		Namespace: "code_reviewer",
		Subsystem: "",
	}
}

// New creates a new Metrics instance.
func New(cfg Config) *Metrics {
	registry := prometheus.NewRegistry()

	// Register default collectors
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	factory := promauto.With(registry)

	m := &Metrics{
		namespace: cfg.Namespace,
		registry:  registry,

		// Request metrics
		RequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		}, []string{"method", "path", "status"}),

		RequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}, []string{"method", "path"}),

		RequestsInFlight: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being processed",
		}),

		// Workflow metrics
		WorkflowsStarted: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "workflows_started_total",
			Help:      "Total number of workflows started",
		}, []string{"workflow_type"}),

		WorkflowsCompleted: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "workflows_completed_total",
			Help:      "Total number of workflows completed",
		}, []string{"workflow_type", "status"}),

		WorkflowDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "workflow_duration_seconds",
			Help:      "Workflow duration in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300, 600},
		}, []string{"workflow_type"}),

		ActivitiesTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "activities_total",
			Help:      "Total number of activities executed",
		}, []string{"activity", "status"}),

		ActivityDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "activity_duration_seconds",
			Help:      "Activity duration in seconds",
			Buckets:   []float64{.1, .5, 1, 2.5, 5, 10, 30, 60},
		}, []string{"activity"}),

		// LLM metrics
		LLMRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "llm_requests_total",
			Help:      "Total number of LLM requests",
		}, []string{"model", "status"}),

		LLMRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "llm_request_duration_seconds",
			Help:      "LLM request duration in seconds",
			Buckets:   []float64{.5, 1, 2.5, 5, 10, 30, 60, 120},
		}, []string{"model"}),

		LLMTokensTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "llm_tokens_total",
			Help:      "Total number of LLM tokens used",
		}, []string{"model", "type"}),

		LLMErrors: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "llm_errors_total",
			Help:      "Total number of LLM errors",
		}, []string{"model", "error_type"}),

		// Embedding metrics
		EmbeddingRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "embedding_requests_total",
			Help:      "Total number of embedding requests",
		}, []string{"model", "status"}),

		EmbeddingRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "embedding_request_duration_seconds",
			Help:      "Embedding request duration in seconds",
			Buckets:   []float64{.01, .05, .1, .25, .5, 1, 2.5, 5},
		}, []string{"model"}),

		EmbeddingCacheHits: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "embedding_cache_hits_total",
			Help:      "Total number of embedding cache hits",
		}),

		EmbeddingCacheMisses: factory.NewCounter(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "embedding_cache_misses_total",
			Help:      "Total number of embedding cache misses",
		}),

		EmbeddingCacheSize: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "embedding_cache_size",
			Help:      "Current number of entries in embedding cache",
		}),

		// Vector store metrics
		VectorStoreDocuments: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: cfg.Namespace,
			Name:      "vectorstore_documents",
			Help:      "Number of documents in vector store",
		}, []string{"collection"}),

		VectorStoreSearches: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "vectorstore_searches_total",
			Help:      "Total number of vector store searches",
		}, []string{"collection"}),

		VectorSearchDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "vectorstore_search_duration_seconds",
			Help:      "Vector store search duration in seconds",
			Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5},
		}, []string{"collection"}),

		// Analysis metrics
		AnalysisIssuesFound: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "analysis_issues_found_total",
			Help:      "Total number of issues found in analysis",
		}, []string{"category", "severity"}),

		AnalysisDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: cfg.Namespace,
			Name:      "analysis_duration_seconds",
			Help:      "Analysis duration in seconds",
			Buckets:   []float64{1, 5, 10, 30, 60, 120, 300},
		}, []string{"category"}),

		// Error metrics
		ErrorsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: cfg.Namespace,
			Name:      "errors_total",
			Help:      "Total number of errors",
		}, []string{"component", "error_type"}),
	}

	return m
}

// Handler returns an HTTP handler for the metrics endpoint.
func (m *Metrics) Handler() http.Handler {
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
}

// Registry returns the Prometheus registry.
func (m *Metrics) Registry() *prometheus.Registry {
	return m.registry
}

// Instrumentation helpers

// RecordRequest records HTTP request metrics.
func (m *Metrics) RecordRequest(method, path string, statusCode int, duration time.Duration) {
	m.RequestsTotal.WithLabelValues(method, path, statusString(statusCode)).Inc()
	m.RequestDuration.WithLabelValues(method, path).Observe(duration.Seconds())
}

// RecordWorkflow records workflow metrics.
func (m *Metrics) RecordWorkflow(workflowType, status string, duration time.Duration) {
	m.WorkflowsCompleted.WithLabelValues(workflowType, status).Inc()
	m.WorkflowDuration.WithLabelValues(workflowType).Observe(duration.Seconds())
}

// RecordActivity records activity metrics.
func (m *Metrics) RecordActivity(activity, status string, duration time.Duration) {
	m.ActivitiesTotal.WithLabelValues(activity, status).Inc()
	m.ActivityDuration.WithLabelValues(activity).Observe(duration.Seconds())
}

// RecordLLMRequest records LLM request metrics.
func (m *Metrics) RecordLLMRequest(model, status string, tokens int, duration time.Duration) {
	m.LLMRequestsTotal.WithLabelValues(model, status).Inc()
	m.LLMRequestDuration.WithLabelValues(model).Observe(duration.Seconds())
	if tokens > 0 {
		m.LLMTokensTotal.WithLabelValues(model, "total").Add(float64(tokens))
	}
}

// RecordEmbeddingRequest records embedding request metrics.
func (m *Metrics) RecordEmbeddingRequest(model, status string, cached int, total int, duration time.Duration) {
	m.EmbeddingRequestsTotal.WithLabelValues(model, status).Inc()
	m.EmbeddingRequestDuration.WithLabelValues(model).Observe(duration.Seconds())
	m.EmbeddingCacheHits.Add(float64(cached))
	m.EmbeddingCacheMisses.Add(float64(total - cached))
}

// RecordAnalysisIssue records an analysis issue.
func (m *Metrics) RecordAnalysisIssue(category, severity string) {
	m.AnalysisIssuesFound.WithLabelValues(category, severity).Inc()
}

// RecordError records an error.
func (m *Metrics) RecordError(component, errorType string) {
	m.ErrorsTotal.WithLabelValues(component, errorType).Inc()
}

func statusString(code int) string {
	switch {
	case code >= 200 && code < 300:
		return "2xx"
	case code >= 300 && code < 400:
		return "3xx"
	case code >= 400 && code < 500:
		return "4xx"
	default:
		return "5xx"
	}
}

// HTTPMiddleware returns middleware that records HTTP metrics.
func (m *Metrics) HTTPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.RequestsInFlight.Inc()
		defer m.RequestsInFlight.Dec()

		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		m.RecordRequest(r.Method, r.URL.Path, rw.statusCode, time.Since(start))
	})
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Global metrics instance
var globalMetrics *Metrics

// Init initializes the global metrics instance.
func Init(cfg Config) *Metrics {
	globalMetrics = New(cfg)
	return globalMetrics
}

// Global returns the global metrics instance.
func Global() *Metrics {
	if globalMetrics == nil {
		globalMetrics = New(DefaultConfig())
	}
	return globalMetrics
}
