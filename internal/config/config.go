// Package config provides centralized configuration management for the code reviewer.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	// Service identification
	ServiceName    string `json:"service_name"`
	Environment    string `json:"environment"` // dev, staging, prod
	Version        string `json:"version"`

	// Server settings
	Server ServerConfig `json:"server"`

	// Temporal settings
	Temporal TemporalConfig `json:"temporal"`

	// LLM settings
	LLM LLMConfig `json:"llm"`

	// Embedder settings
	Embedder EmbedderConfig `json:"embedder"`

	// RAG settings
	RAG RAGConfig `json:"rag"`

	// Vector store settings
	VectorStore VectorStoreConfig `json:"vector_store"`

	// Database Lab settings
	DatabaseLab DatabaseLabConfig `json:"database_lab"`

	// Analysis settings
	Analysis AnalysisConfig `json:"analysis"`

	// Logging settings
	Logging LoggingConfig `json:"logging"`

	// Metrics settings
	Metrics MetricsConfig `json:"metrics"`

	// GitLab integration settings
	GitLab GitLabConfig `json:"gitlab"`

	// Tracing settings
	Tracing TracingConfig `json:"tracing"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Host            string        `json:"host"`
	Port            int           `json:"port"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

// TemporalConfig holds Temporal workflow configuration.
type TemporalConfig struct {
	Host            string        `json:"host"`
	Namespace       string        `json:"namespace"`
	TaskQueue       string        `json:"task_queue"`
	WorkflowTimeout time.Duration `json:"workflow_timeout"`
	ActivityTimeout time.Duration `json:"activity_timeout"`

	// Retry policy
	RetryInitialInterval    time.Duration `json:"retry_initial_interval"`
	RetryBackoffCoefficient float64       `json:"retry_backoff_coefficient"`
	RetryMaxInterval        time.Duration `json:"retry_max_interval"`
	RetryMaxAttempts        int           `json:"retry_max_attempts"`
}

// LLMConfig holds LLM client configuration.
type LLMConfig struct {
	Provider    string        `json:"provider"` // ollama, openai, anthropic
	BaseURL     string        `json:"base_url"`
	APIKey      string        `json:"-"` // Don't serialize API key
	Model       string        `json:"model"`
	Timeout     time.Duration `json:"timeout"`
	MaxTokens   int           `json:"max_tokens"`
	Temperature float64       `json:"temperature"`

	// Rate limiting
	RequestsPerMinute int `json:"requests_per_minute"`
	TokensPerMinute   int `json:"tokens_per_minute"`
}

// EmbedderConfig holds embedder configuration.
type EmbedderConfig struct {
	Provider  string        `json:"provider"` // ollama, openai
	BaseURL   string        `json:"base_url"`
	APIKey    string        `json:"-"`
	Model     string        `json:"model"`
	Dimension int           `json:"dimension"`
	Timeout   time.Duration `json:"timeout"`
	BatchSize int           `json:"batch_size"`

	// Cache settings
	CacheEnabled   bool          `json:"cache_enabled"`
	CacheMaxSize   int           `json:"cache_max_size"`
	CacheTTL       time.Duration `json:"cache_ttl"`
	CacheStorePath string        `json:"cache_store_path"`
}

// RAGConfig holds RAG retrieval configuration.
type RAGConfig struct {
	// Chunking
	ChunkSize    int `json:"chunk_size"`
	ChunkOverlap int `json:"chunk_overlap"`
	MinChunkSize int `json:"min_chunk_size"`

	// Retrieval
	DefaultTopK      int     `json:"default_top_k"`
	MinSimilarity    float64 `json:"min_similarity"`
	MaxContextLength int     `json:"max_context_length"`

	// Deduplication
	DeduplicationThreshold float64 `json:"deduplication_threshold"`
}

// VectorStoreConfig holds vector store configuration.
type VectorStoreConfig struct {
	Type      string `json:"type"` // memory, milvus
	StorePath string `json:"store_path"`

	// Milvus settings
	MilvusHost       string `json:"milvus_host"`
	MilvusPort       int    `json:"milvus_port"`
	MilvusCollection string `json:"milvus_collection"`
}

// DatabaseLabConfig holds Database Lab configuration.
type DatabaseLabConfig struct {
	Enabled       bool          `json:"enabled"`
	BaseURL       string        `json:"base_url"`
	Token         string        `json:"-"`
	VerifyToken   string        `json:"-"`
	Timeout       time.Duration `json:"timeout"`
	CloneWaitTime time.Duration `json:"clone_wait_time"`

	// Thresholds
	SeqScanRowThreshold int `json:"seq_scan_row_threshold"`
}

// AnalysisConfig holds analysis-specific configuration.
type AnalysisConfig struct {
	// Severity filtering
	MinSeverity string `json:"min_severity"` // critical, high, medium, low

	// Content limits
	MaxFileSize      int `json:"max_file_size"`
	MinContentLength int `json:"min_content_length"`

	// Categories
	EnablePerformance bool `json:"enable_performance"`
	EnableSecurity    bool `json:"enable_security"`
	EnableStandards   bool `json:"enable_standards"`

	// Organization standards (custom rules)
	OrgStandardsPath string `json:"org_standards_path"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level      string `json:"level"` // debug, info, warn, error
	Format     string `json:"format"` // json, text
	Output     string `json:"output"` // stdout, stderr, file
	FilePath   string `json:"file_path"`
	AddSource  bool   `json:"add_source"`
}

// MetricsConfig holds metrics configuration.
type MetricsConfig struct {
	Enabled   bool   `json:"enabled"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Path      string `json:"path"`
	Namespace string `json:"namespace"`
}

// GitLabConfig holds GitLab integration configuration.
type GitLabConfig struct {
	Enabled            bool          `json:"enabled"`
	BaseURL            string        `json:"base_url"`
	Token              string        `json:"-"` // Don't serialize token
	Timeout            time.Duration `json:"timeout"`
	PostSummary        bool          `json:"post_summary"`
	PostInlineComments bool          `json:"post_inline_comments"`
	MinSeverity        string        `json:"min_severity"`
	CollapseThreshold  int           `json:"collapse_threshold"`
	DryRun             bool          `json:"dry_run"`
}

// TracingConfig holds distributed tracing configuration.
type TracingConfig struct {
	Enabled      bool    `json:"enabled"`
	Provider     string  `json:"provider"` // otlp, jaeger, zipkin
	Endpoint     string  `json:"endpoint"`
	ServiceName  string  `json:"service_name"`
	SampleRate   float64 `json:"sample_rate"` // 0.0 to 1.0
	Insecure     bool    `json:"insecure"`    // Use insecure connection
	BatchTimeout time.Duration `json:"batch_timeout"`
}

// DefaultConfig returns configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		ServiceName: "code-reviewer",
		Environment: "development",
		Version:     "0.1.0",

		Server: ServerConfig{
			Host:            "0.0.0.0",
			Port:            8080,
			ReadTimeout:     30 * time.Second,
			WriteTimeout:    10 * time.Minute,
			IdleTimeout:     120 * time.Second,
			ShutdownTimeout: 30 * time.Second,
		},

		Temporal: TemporalConfig{
			Host:                    "localhost:7233",
			Namespace:               "default",
			TaskQueue:               "code-review-queue",
			WorkflowTimeout:         30 * time.Minute,
			ActivityTimeout:         5 * time.Minute,
			RetryInitialInterval:    time.Second,
			RetryBackoffCoefficient: 2.0,
			RetryMaxInterval:        time.Minute,
			RetryMaxAttempts:        3,
		},

		LLM: LLMConfig{
			Provider:          "ollama",
			BaseURL:           "http://localhost:11434",
			Model:             "qwen2.5-coder:32b",
			Timeout:           5 * time.Minute,
			MaxTokens:         4096,
			Temperature:       0.1,
			RequestsPerMinute: 60,
			TokensPerMinute:   100000,
		},

		Embedder: EmbedderConfig{
			Provider:       "ollama",
			BaseURL:        "http://localhost:11434",
			Model:          "nomic-embed-text",
			Dimension:      768,
			Timeout:        60 * time.Second,
			BatchSize:      32,
			CacheEnabled:   true,
			CacheMaxSize:   10000,
			CacheTTL:       24 * time.Hour,
			CacheStorePath: "",
		},

		RAG: RAGConfig{
			ChunkSize:              500,
			ChunkOverlap:           50,
			MinChunkSize:           100,
			DefaultTopK:            5,
			MinSimilarity:          0.5,
			MaxContextLength:       8000,
			DeduplicationThreshold: 0.95,
		},

		VectorStore: VectorStoreConfig{
			Type:             "memory",
			StorePath:        "./data/vectorstore.json",
			MilvusHost:       "localhost",
			MilvusPort:       19530,
			MilvusCollection: "postgres_knowledge",
		},

		DatabaseLab: DatabaseLabConfig{
			Enabled:             false,
			Timeout:             60 * time.Second,
			CloneWaitTime:       2 * time.Second,
			SeqScanRowThreshold: 10000,
		},

		Analysis: AnalysisConfig{
			MinSeverity:       "low",
			MaxFileSize:       1024 * 1024, // 1MB
			MinContentLength:  50,
			EnablePerformance: true,
			EnableSecurity:    true,
			EnableStandards:   true,
		},

		Logging: LoggingConfig{
			Level:     "info",
			Format:    "json",
			Output:    "stdout",
			AddSource: false,
		},

		Metrics: MetricsConfig{
			Enabled:   true,
			Host:      "0.0.0.0",
			Port:      9090,
			Path:      "/metrics",
			Namespace: "code_reviewer",
		},

		GitLab: GitLabConfig{
			Enabled:            false,
			Timeout:            30 * time.Second,
			PostSummary:        true,
			PostInlineComments: true,
			MinSeverity:        "low",
			CollapseThreshold:  20,
			DryRun:             false,
		},

		Tracing: TracingConfig{
			Enabled:      false,
			Provider:     "otlp",
			Endpoint:     "localhost:4317",
			ServiceName:  "code-reviewer",
			SampleRate:   1.0,
			Insecure:     true,
			BatchTimeout: 5 * time.Second,
		},
	}
}

// LoadFromEnv loads configuration from environment variables.
func LoadFromEnv() (*Config, error) {
	cfg := DefaultConfig()

	// Service
	cfg.ServiceName = getEnvString("SERVICE_NAME", cfg.ServiceName)
	cfg.Environment = getEnvString("ENVIRONMENT", cfg.Environment)

	// Server
	cfg.Server.Host = getEnvString("SERVER_HOST", cfg.Server.Host)
	cfg.Server.Port = getEnvInt("SERVER_PORT", cfg.Server.Port)
	cfg.Server.ReadTimeout = getEnvDuration("SERVER_READ_TIMEOUT", cfg.Server.ReadTimeout)
	cfg.Server.WriteTimeout = getEnvDuration("SERVER_WRITE_TIMEOUT", cfg.Server.WriteTimeout)
	cfg.Server.ShutdownTimeout = getEnvDuration("SERVER_SHUTDOWN_TIMEOUT", cfg.Server.ShutdownTimeout)

	// Temporal
	cfg.Temporal.Host = getEnvString("TEMPORAL_HOST", cfg.Temporal.Host)
	cfg.Temporal.Namespace = getEnvString("TEMPORAL_NAMESPACE", cfg.Temporal.Namespace)
	cfg.Temporal.TaskQueue = getEnvString("TEMPORAL_TASK_QUEUE", cfg.Temporal.TaskQueue)
	cfg.Temporal.WorkflowTimeout = getEnvDuration("TEMPORAL_WORKFLOW_TIMEOUT", cfg.Temporal.WorkflowTimeout)
	cfg.Temporal.ActivityTimeout = getEnvDuration("TEMPORAL_ACTIVITY_TIMEOUT", cfg.Temporal.ActivityTimeout)
	cfg.Temporal.RetryMaxAttempts = getEnvInt("TEMPORAL_RETRY_MAX_ATTEMPTS", cfg.Temporal.RetryMaxAttempts)

	// LLM
	cfg.LLM.Provider = getEnvString("LLM_PROVIDER", cfg.LLM.Provider)
	cfg.LLM.BaseURL = getEnvString("LLM_BASE_URL", cfg.LLM.BaseURL)
	cfg.LLM.APIKey = getEnvString("LLM_API_KEY", cfg.LLM.APIKey)
	cfg.LLM.Model = getEnvString("LLM_MODEL", cfg.LLM.Model)
	cfg.LLM.Timeout = getEnvDuration("LLM_TIMEOUT", cfg.LLM.Timeout)
	cfg.LLM.MaxTokens = getEnvInt("LLM_MAX_TOKENS", cfg.LLM.MaxTokens)
	cfg.LLM.Temperature = getEnvFloat("LLM_TEMPERATURE", cfg.LLM.Temperature)
	cfg.LLM.RequestsPerMinute = getEnvInt("LLM_REQUESTS_PER_MINUTE", cfg.LLM.RequestsPerMinute)

	// Embedder
	cfg.Embedder.Provider = getEnvString("EMBEDDER_PROVIDER", cfg.Embedder.Provider)
	cfg.Embedder.BaseURL = getEnvString("EMBEDDER_BASE_URL", cfg.Embedder.BaseURL)
	cfg.Embedder.APIKey = getEnvString("EMBEDDER_API_KEY", cfg.Embedder.APIKey)
	cfg.Embedder.Model = getEnvString("EMBEDDER_MODEL", cfg.Embedder.Model)
	cfg.Embedder.Dimension = getEnvInt("EMBEDDER_DIMENSION", cfg.Embedder.Dimension)
	cfg.Embedder.Timeout = getEnvDuration("EMBEDDER_TIMEOUT", cfg.Embedder.Timeout)
	cfg.Embedder.BatchSize = getEnvInt("EMBEDDER_BATCH_SIZE", cfg.Embedder.BatchSize)
	cfg.Embedder.CacheEnabled = getEnvBool("EMBEDDER_CACHE_ENABLED", cfg.Embedder.CacheEnabled)
	cfg.Embedder.CacheMaxSize = getEnvInt("EMBEDDER_CACHE_MAX_SIZE", cfg.Embedder.CacheMaxSize)
	cfg.Embedder.CacheTTL = getEnvDuration("EMBEDDER_CACHE_TTL", cfg.Embedder.CacheTTL)
	cfg.Embedder.CacheStorePath = getEnvString("EMBEDDER_CACHE_STORE_PATH", cfg.Embedder.CacheStorePath)

	// RAG
	cfg.RAG.ChunkSize = getEnvInt("RAG_CHUNK_SIZE", cfg.RAG.ChunkSize)
	cfg.RAG.ChunkOverlap = getEnvInt("RAG_CHUNK_OVERLAP", cfg.RAG.ChunkOverlap)
	cfg.RAG.MinChunkSize = getEnvInt("RAG_MIN_CHUNK_SIZE", cfg.RAG.MinChunkSize)
	cfg.RAG.DefaultTopK = getEnvInt("RAG_DEFAULT_TOP_K", cfg.RAG.DefaultTopK)
	cfg.RAG.MinSimilarity = getEnvFloat("RAG_MIN_SIMILARITY", cfg.RAG.MinSimilarity)
	cfg.RAG.MaxContextLength = getEnvInt("RAG_MAX_CONTEXT_LENGTH", cfg.RAG.MaxContextLength)

	// Vector store
	cfg.VectorStore.Type = getEnvString("VECTORSTORE_TYPE", cfg.VectorStore.Type)
	cfg.VectorStore.StorePath = getEnvString("VECTORSTORE_STORE_PATH", cfg.VectorStore.StorePath)
	cfg.VectorStore.MilvusHost = getEnvString("MILVUS_HOST", cfg.VectorStore.MilvusHost)
	cfg.VectorStore.MilvusPort = getEnvInt("MILVUS_PORT", cfg.VectorStore.MilvusPort)

	// Database Lab
	cfg.DatabaseLab.Enabled = getEnvBool("DBLAB_ENABLED", cfg.DatabaseLab.Enabled)
	cfg.DatabaseLab.BaseURL = getEnvString("DBLAB_URL", cfg.DatabaseLab.BaseURL)
	cfg.DatabaseLab.Token = getEnvString("DBLAB_TOKEN", cfg.DatabaseLab.Token)
	cfg.DatabaseLab.VerifyToken = getEnvString("DBLAB_VERIFY_TOKEN", cfg.DatabaseLab.VerifyToken)
	cfg.DatabaseLab.Timeout = getEnvDuration("DBLAB_TIMEOUT", cfg.DatabaseLab.Timeout)
	cfg.DatabaseLab.SeqScanRowThreshold = getEnvInt("DBLAB_SEQ_SCAN_ROW_THRESHOLD", cfg.DatabaseLab.SeqScanRowThreshold)

	// Analysis
	cfg.Analysis.MinSeverity = getEnvString("ANALYSIS_MIN_SEVERITY", cfg.Analysis.MinSeverity)
	cfg.Analysis.MaxFileSize = getEnvInt("ANALYSIS_MAX_FILE_SIZE", cfg.Analysis.MaxFileSize)
	cfg.Analysis.MinContentLength = getEnvInt("ANALYSIS_MIN_CONTENT_LENGTH", cfg.Analysis.MinContentLength)
	cfg.Analysis.EnablePerformance = getEnvBool("ANALYSIS_ENABLE_PERFORMANCE", cfg.Analysis.EnablePerformance)
	cfg.Analysis.EnableSecurity = getEnvBool("ANALYSIS_ENABLE_SECURITY", cfg.Analysis.EnableSecurity)
	cfg.Analysis.EnableStandards = getEnvBool("ANALYSIS_ENABLE_STANDARDS", cfg.Analysis.EnableStandards)
	cfg.Analysis.OrgStandardsPath = getEnvString("ANALYSIS_ORG_STANDARDS_PATH", cfg.Analysis.OrgStandardsPath)

	// Logging
	cfg.Logging.Level = getEnvString("LOG_LEVEL", cfg.Logging.Level)
	cfg.Logging.Format = getEnvString("LOG_FORMAT", cfg.Logging.Format)
	cfg.Logging.Output = getEnvString("LOG_OUTPUT", cfg.Logging.Output)
	cfg.Logging.FilePath = getEnvString("LOG_FILE_PATH", cfg.Logging.FilePath)
	cfg.Logging.AddSource = getEnvBool("LOG_ADD_SOURCE", cfg.Logging.AddSource)

	// Metrics
	cfg.Metrics.Enabled = getEnvBool("METRICS_ENABLED", cfg.Metrics.Enabled)
	cfg.Metrics.Port = getEnvInt("METRICS_PORT", cfg.Metrics.Port)
	cfg.Metrics.Path = getEnvString("METRICS_PATH", cfg.Metrics.Path)
	cfg.Metrics.Namespace = getEnvString("METRICS_NAMESPACE", cfg.Metrics.Namespace)

	// GitLab
	cfg.GitLab.Enabled = getEnvBool("GITLAB_ENABLED", cfg.GitLab.Enabled)
	cfg.GitLab.BaseURL = getEnvString("GITLAB_URL", cfg.GitLab.BaseURL)
	cfg.GitLab.Token = getEnvString("GITLAB_TOKEN", cfg.GitLab.Token)
	cfg.GitLab.Timeout = getEnvDuration("GITLAB_TIMEOUT", cfg.GitLab.Timeout)
	cfg.GitLab.PostSummary = getEnvBool("GITLAB_POST_SUMMARY", cfg.GitLab.PostSummary)
	cfg.GitLab.PostInlineComments = getEnvBool("GITLAB_POST_INLINE_COMMENTS", cfg.GitLab.PostInlineComments)
	cfg.GitLab.MinSeverity = getEnvString("GITLAB_MIN_SEVERITY", cfg.GitLab.MinSeverity)
	cfg.GitLab.CollapseThreshold = getEnvInt("GITLAB_COLLAPSE_THRESHOLD", cfg.GitLab.CollapseThreshold)
	cfg.GitLab.DryRun = getEnvBool("GITLAB_DRY_RUN", cfg.GitLab.DryRun)

	// Tracing
	cfg.Tracing.Enabled = getEnvBool("TRACING_ENABLED", cfg.Tracing.Enabled)
	cfg.Tracing.Provider = getEnvString("TRACING_PROVIDER", cfg.Tracing.Provider)
	cfg.Tracing.Endpoint = getEnvString("TRACING_ENDPOINT", cfg.Tracing.Endpoint)
	cfg.Tracing.ServiceName = getEnvString("TRACING_SERVICE_NAME", cfg.Tracing.ServiceName)
	cfg.Tracing.SampleRate = getEnvFloat("TRACING_SAMPLE_RATE", cfg.Tracing.SampleRate)
	cfg.Tracing.Insecure = getEnvBool("TRACING_INSECURE", cfg.Tracing.Insecure)
	cfg.Tracing.BatchTimeout = getEnvDuration("TRACING_BATCH_TIMEOUT", cfg.Tracing.BatchTimeout)

	return cfg, nil
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	var errs []string

	// Server validation
	if c.Server.Port <= 0 || c.Server.Port > 65535 {
		errs = append(errs, "server.port must be between 1 and 65535")
	}

	// LLM validation
	if c.LLM.BaseURL == "" {
		errs = append(errs, "llm.base_url is required")
	}
	if c.LLM.Model == "" {
		errs = append(errs, "llm.model is required")
	}

	// Embedder validation
	if c.Embedder.Dimension <= 0 {
		errs = append(errs, "embedder.dimension must be positive")
	}

	// RAG validation
	if c.RAG.ChunkSize <= 0 {
		errs = append(errs, "rag.chunk_size must be positive")
	}
	if c.RAG.ChunkOverlap >= c.RAG.ChunkSize {
		errs = append(errs, "rag.chunk_overlap must be less than chunk_size")
	}
	if c.RAG.MinSimilarity < 0 || c.RAG.MinSimilarity > 1 {
		errs = append(errs, "rag.min_similarity must be between 0 and 1")
	}

	// Database Lab validation
	if c.DatabaseLab.Enabled && c.DatabaseLab.BaseURL == "" {
		errs = append(errs, "database_lab.base_url is required when enabled")
	}

	// Logging validation
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		errs = append(errs, "logging.level must be debug, info, warn, or error")
	}

	// GitLab validation
	if c.GitLab.Enabled && c.GitLab.BaseURL == "" {
		errs = append(errs, "gitlab.base_url is required when enabled")
	}
	if c.GitLab.Enabled && c.GitLab.Token == "" {
		errs = append(errs, "gitlab.token is required when enabled")
	}

	// Tracing validation
	if c.Tracing.Enabled && c.Tracing.Endpoint == "" {
		errs = append(errs, "tracing.endpoint is required when enabled")
	}
	if c.Tracing.SampleRate < 0 || c.Tracing.SampleRate > 1 {
		errs = append(errs, "tracing.sample_rate must be between 0 and 1")
	}

	if len(errs) > 0 {
		return fmt.Errorf("configuration errors:\n  - %s", strings.Join(errs, "\n  - "))
	}

	return nil
}

// IsDevelopment returns true if running in development mode.
func (c *Config) IsDevelopment() bool {
	return c.Environment == "development" || c.Environment == "dev"
}

// IsProduction returns true if running in production mode.
func (c *Config) IsProduction() bool {
	return c.Environment == "production" || c.Environment == "prod"
}

// Helper functions

func getEnvString(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

func getEnvFloat(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}
