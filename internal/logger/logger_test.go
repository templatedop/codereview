package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", DEBUG},
		{"DEBUG", DEBUG},
		{"info", INFO},
		{"INFO", INFO},
		{"warn", WARN},
		{"WARN", WARN},
		{"warning", WARN},
		{"error", ERROR},
		{"ERROR", ERROR},
		{"fatal", FATAL},
		{"FATAL", FATAL},
		{"unknown", INFO}, // default
		{"", INFO},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLevel_String(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DEBUG, "DEBUG"},
		{INFO, "INFO"},
		{WARN, "WARN"},
		{ERROR, "ERROR"},
		{FATAL, "FATAL"},
		{Level(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := tt.level.String()
			if result != tt.expected {
				t.Errorf("Level(%d).String() = %q, want %q", tt.level, result, tt.expected)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Level != INFO {
		t.Errorf("expected default level INFO, got %v", cfg.Level)
	}
	if cfg.Format != "text" {
		t.Errorf("expected default format 'text', got %q", cfg.Format)
	}
	if cfg.Output != "stdout" {
		t.Errorf("expected default output 'stdout', got %q", cfg.Output)
	}
}

func TestNew_Stdout(t *testing.T) {
	log, err := New(Config{
		Level:  INFO,
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer log.Close()

	if log.output != os.Stdout {
		t.Error("expected output to be stdout")
	}
}

func TestNew_Stderr(t *testing.T) {
	log, err := New(Config{
		Level:  INFO,
		Format: "text",
		Output: "stderr",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer log.Close()

	if log.output != os.Stderr {
		t.Error("expected output to be stderr")
	}
}

func TestNew_File(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "logger-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	logPath := filepath.Join(tmpDir, "test.log")

	log, err := New(Config{
		Level:  INFO,
		Format: "text",
		Output: logPath,
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	log.Info("Test message")
	log.Close()

	// Verify file was created and contains message
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "Test message") {
		t.Errorf("log file should contain 'Test message', got: %s", content)
	}
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer

	log, err := New(Config{
		Level:      WARN,
		Format:     "text",
		Output:     "stdout",
		TimeFormat: "15:04:05",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	log.output = &buf

	log.Debug("debug message")
	log.Info("info message")
	log.Warn("warn message")
	log.Error("error message")

	output := buf.String()

	if strings.Contains(output, "debug message") {
		t.Error("debug message should be filtered out")
	}
	if strings.Contains(output, "info message") {
		t.Error("info message should be filtered out")
	}
	if !strings.Contains(output, "warn message") {
		t.Error("warn message should be included")
	}
	if !strings.Contains(output, "error message") {
		t.Error("error message should be included")
	}
}

func TestLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer

	log, err := New(Config{
		Level:      DEBUG,
		Format:     "text",
		Output:     "stdout",
		TimeFormat: "15:04:05",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	log.output = &buf

	log.Info("Test message with %s", "args")

	output := buf.String()

	if !strings.Contains(output, "[INFO]") {
		t.Error("output should contain [INFO]")
	}
	if !strings.Contains(output, "Test message with args") {
		t.Error("output should contain formatted message")
	}
}

func TestLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer

	log, err := New(Config{
		Level:      DEBUG,
		Format:     "json",
		Output:     "stdout",
		TimeFormat: "15:04:05",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	log.output = &buf

	log.Info("Test message")

	output := buf.String()

	if !strings.Contains(output, `"level":"INFO"`) {
		t.Error("JSON output should contain level field")
	}
	if !strings.Contains(output, `"msg":"Test message"`) {
		t.Error("JSON output should contain msg field")
	}
	if !strings.HasPrefix(output, "{") || !strings.Contains(output, "}") {
		t.Error("JSON output should be JSON object")
	}
}

func TestLogger_SetLevel(t *testing.T) {
	log, err := New(Config{
		Level:  INFO,
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer log.Close()

	if log.GetLevel() != INFO {
		t.Errorf("expected level INFO, got %v", log.GetLevel())
	}

	log.SetLevel(DEBUG)
	if log.GetLevel() != DEBUG {
		t.Errorf("expected level DEBUG, got %v", log.GetLevel())
	}
}

func TestLogger_With(t *testing.T) {
	log, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer log.Close()

	// With should return a logger
	newLog := log.With(map[string]interface{}{"key": "value"})
	if newLog == nil {
		t.Error("With should return a logger")
	}
}

func TestLogger_StdLogger(t *testing.T) {
	var buf bytes.Buffer

	log, err := New(Config{
		Level:  DEBUG,
		Format: "text",
		Output: "stdout",
	})
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	log.output = &buf

	stdLog := log.StdLogger(INFO)
	stdLog.Println("Standard log message")

	output := buf.String()
	if !strings.Contains(output, "Standard log message") {
		t.Error("StdLogger output should contain message")
	}
	if !strings.Contains(output, "[INFO]") {
		t.Error("StdLogger output should have INFO level")
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{`with "quotes"`, `with \"quotes\"`},
		{"with\nnewline", `with\nnewline`},
		{"with\ttab", `with\ttab`},
		{`back\slash`, `back\\slash`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := escapeJSON(tt.input)
			if result != tt.expected {
				t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestDefaultLoggerFunctions(t *testing.T) {
	// Test that default logger functions don't panic
	SetLevel(DEBUG)
	if GetLevel() != DEBUG {
		t.Errorf("expected DEBUG, got %v", GetLevel())
	}

	// These should not panic
	Debug("debug %s", "msg")
	Info("info %s", "msg")
	Warn("warn %s", "msg")
	Error("error %s", "msg")
}
