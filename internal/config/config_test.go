package config

import (
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServiceName != "code-reviewer" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "code-reviewer")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.LLM.Model != "qwen2.5-coder:32b" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "qwen2.5-coder:32b")
	}
	if cfg.Embedder.Dimension != 768 {
		t.Errorf("Embedder.Dimension = %d, want 768", cfg.Embedder.Dimension)
	}
	if cfg.RAG.ChunkSize != 500 {
		t.Errorf("RAG.ChunkSize = %d, want 500", cfg.RAG.ChunkSize)
	}
}

func TestLoadFromEnv(t *testing.T) {
	// Set test environment variables
	os.Setenv("SERVICE_NAME", "test-service")
	os.Setenv("SERVER_PORT", "9000")
	os.Setenv("LLM_MODEL", "test-model")
	os.Setenv("LLM_TIMEOUT", "10m")
	os.Setenv("EMBEDDER_DIMENSION", "1024")
	os.Setenv("RAG_CHUNK_SIZE", "1000")
	os.Setenv("LOG_LEVEL", "debug")
	defer func() {
		os.Unsetenv("SERVICE_NAME")
		os.Unsetenv("SERVER_PORT")
		os.Unsetenv("LLM_MODEL")
		os.Unsetenv("LLM_TIMEOUT")
		os.Unsetenv("EMBEDDER_DIMENSION")
		os.Unsetenv("RAG_CHUNK_SIZE")
		os.Unsetenv("LOG_LEVEL")
	}()

	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatalf("LoadFromEnv() error = %v", err)
	}

	if cfg.ServiceName != "test-service" {
		t.Errorf("ServiceName = %q, want %q", cfg.ServiceName, "test-service")
	}
	if cfg.Server.Port != 9000 {
		t.Errorf("Server.Port = %d, want 9000", cfg.Server.Port)
	}
	if cfg.LLM.Model != "test-model" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "test-model")
	}
	if cfg.LLM.Timeout != 10*time.Minute {
		t.Errorf("LLM.Timeout = %v, want 10m", cfg.LLM.Timeout)
	}
	if cfg.Embedder.Dimension != 1024 {
		t.Errorf("Embedder.Dimension = %d, want 1024", cfg.Embedder.Dimension)
	}
	if cfg.RAG.ChunkSize != 1000 {
		t.Errorf("RAG.ChunkSize = %d, want 1000", cfg.RAG.ChunkSize)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q, want %q", cfg.Logging.Level, "debug")
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *Config) {},
			wantErr: false,
		},
		{
			name: "invalid port",
			modify: func(c *Config) {
				c.Server.Port = 70000
			},
			wantErr: true,
		},
		{
			name: "missing LLM base URL",
			modify: func(c *Config) {
				c.LLM.BaseURL = ""
			},
			wantErr: true,
		},
		{
			name: "missing LLM model",
			modify: func(c *Config) {
				c.LLM.Model = ""
			},
			wantErr: true,
		},
		{
			name: "invalid embedder dimension",
			modify: func(c *Config) {
				c.Embedder.Dimension = 0
			},
			wantErr: true,
		},
		{
			name: "chunk overlap >= chunk size",
			modify: func(c *Config) {
				c.RAG.ChunkOverlap = 600
				c.RAG.ChunkSize = 500
			},
			wantErr: true,
		},
		{
			name: "invalid min similarity",
			modify: func(c *Config) {
				c.RAG.MinSimilarity = 1.5
			},
			wantErr: true,
		},
		{
			name: "database lab enabled without URL",
			modify: func(c *Config) {
				c.DatabaseLab.Enabled = true
				c.DatabaseLab.BaseURL = ""
			},
			wantErr: true,
		},
		{
			name: "invalid log level",
			modify: func(c *Config) {
				c.Logging.Level = "invalid"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.modify(cfg)

			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_IsDevelopment(t *testing.T) {
	cfg := DefaultConfig()

	cfg.Environment = "development"
	if !cfg.IsDevelopment() {
		t.Error("IsDevelopment() = false for 'development'")
	}

	cfg.Environment = "dev"
	if !cfg.IsDevelopment() {
		t.Error("IsDevelopment() = false for 'dev'")
	}

	cfg.Environment = "production"
	if cfg.IsDevelopment() {
		t.Error("IsDevelopment() = true for 'production'")
	}
}

func TestConfig_IsProduction(t *testing.T) {
	cfg := DefaultConfig()

	cfg.Environment = "production"
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false for 'production'")
	}

	cfg.Environment = "prod"
	if !cfg.IsProduction() {
		t.Error("IsProduction() = false for 'prod'")
	}

	cfg.Environment = "development"
	if cfg.IsProduction() {
		t.Error("IsProduction() = true for 'development'")
	}
}

func TestGetEnvHelpers(t *testing.T) {
	// Test string
	os.Setenv("TEST_STRING", "hello")
	if got := getEnvString("TEST_STRING", "default"); got != "hello" {
		t.Errorf("getEnvString() = %q, want %q", got, "hello")
	}
	if got := getEnvString("NONEXISTENT", "default"); got != "default" {
		t.Errorf("getEnvString() = %q, want %q", got, "default")
	}
	os.Unsetenv("TEST_STRING")

	// Test int
	os.Setenv("TEST_INT", "42")
	if got := getEnvInt("TEST_INT", 0); got != 42 {
		t.Errorf("getEnvInt() = %d, want 42", got)
	}
	os.Setenv("TEST_INT", "invalid")
	if got := getEnvInt("TEST_INT", 10); got != 10 {
		t.Errorf("getEnvInt() = %d, want 10 (default on invalid)", got)
	}
	os.Unsetenv("TEST_INT")

	// Test float
	os.Setenv("TEST_FLOAT", "3.14")
	if got := getEnvFloat("TEST_FLOAT", 0); got != 3.14 {
		t.Errorf("getEnvFloat() = %f, want 3.14", got)
	}
	os.Unsetenv("TEST_FLOAT")

	// Test bool
	os.Setenv("TEST_BOOL", "true")
	if got := getEnvBool("TEST_BOOL", false); !got {
		t.Error("getEnvBool() = false, want true")
	}
	os.Unsetenv("TEST_BOOL")

	// Test duration
	os.Setenv("TEST_DURATION", "5m")
	if got := getEnvDuration("TEST_DURATION", time.Second); got != 5*time.Minute {
		t.Errorf("getEnvDuration() = %v, want 5m", got)
	}
	os.Unsetenv("TEST_DURATION")
}
