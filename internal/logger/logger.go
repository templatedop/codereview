package logger

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Level represents log severity levels
type Level int

const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
	FATAL
)

func (l Level) String() string {
	switch l {
	case DEBUG:
		return "DEBUG"
	case INFO:
		return "INFO"
	case WARN:
		return "WARN"
	case ERROR:
		return "ERROR"
	case FATAL:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel converts a string to Level
func ParseLevel(s string) Level {
	switch strings.ToUpper(s) {
	case "DEBUG":
		return DEBUG
	case "INFO":
		return INFO
	case "WARN", "WARNING":
		return WARN
	case "ERROR":
		return ERROR
	case "FATAL":
		return FATAL
	default:
		return INFO
	}
}

// Config holds logger configuration
type Config struct {
	Level      Level  // Minimum log level
	Format     string // "text" or "json"
	Output     string // "stdout", "stderr", or file path
	TimeFormat string // Time format string
	ShowCaller bool   // Include caller info
}

// DefaultConfig returns default logger configuration
func DefaultConfig() Config {
	return Config{
		Level:      INFO,
		Format:     "text",
		Output:     "stdout",
		TimeFormat: "2006-01-02 15:04:05",
		ShowCaller: false,
	}
}

// Logger is a configurable logger
type Logger struct {
	config Config
	output io.Writer
	mu     sync.Mutex
	file   *os.File
}

var (
	defaultLogger *Logger
	once          sync.Once
)

// Init initializes the default logger with config
func Init(cfg Config) error {
	var err error
	once.Do(func() {
		defaultLogger, err = New(cfg)
	})
	return err
}

// New creates a new logger instance
func New(cfg Config) (*Logger, error) {
	l := &Logger{config: cfg}

	switch cfg.Output {
	case "stdout", "":
		l.output = os.Stdout
	case "stderr":
		l.output = os.Stderr
	default:
		// File output
		dir := filepath.Dir(cfg.Output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("create log directory: %w", err)
		}
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		l.file = f
		l.output = f
	}

	return l, nil
}

// Close closes the logger
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// SetLevel changes the log level
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.config.Level = level
}

// GetLevel returns the current log level
func (l *Logger) GetLevel() Level {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.config.Level
}

// log writes a log entry
func (l *Logger) log(level Level, format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if level < l.config.Level {
		return
	}

	now := time.Now()
	msg := fmt.Sprintf(format, args...)

	var entry string
	if l.config.Format == "json" {
		entry = l.formatJSON(now, level, msg)
	} else {
		entry = l.formatText(now, level, msg)
	}

	fmt.Fprintln(l.output, entry)

	if level == FATAL {
		os.Exit(1)
	}
}

func (l *Logger) formatText(t time.Time, level Level, msg string) string {
	timeStr := t.Format(l.config.TimeFormat)
	prefix := fmt.Sprintf("%s [%s]", timeStr, level.String())

	if l.config.ShowCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			file = filepath.Base(file)
			prefix = fmt.Sprintf("%s %s:%d", prefix, file, line)
		}
	}

	return fmt.Sprintf("%s %s", prefix, msg)
}

func (l *Logger) formatJSON(t time.Time, level Level, msg string) string {
	entry := fmt.Sprintf(`{"time":"%s","level":"%s","msg":"%s"`,
		t.Format(time.RFC3339),
		level.String(),
		escapeJSON(msg))

	if l.config.ShowCaller {
		_, file, line, ok := runtime.Caller(3)
		if ok {
			entry += fmt.Sprintf(`,"file":"%s","line":%d`, filepath.Base(file), line)
		}
	}

	entry += "}"
	return entry
}

func escapeJSON(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", `\n`)
	s = strings.ReplaceAll(s, "\r", `\r`)
	s = strings.ReplaceAll(s, "\t", `\t`)
	return s
}

// Debug logs at DEBUG level
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DEBUG, format, args...)
}

// Info logs at INFO level
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(INFO, format, args...)
}

// Warn logs at WARN level
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WARN, format, args...)
}

// Error logs at ERROR level
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ERROR, format, args...)
}

// Fatal logs at FATAL level and exits
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(FATAL, format, args...)
}

// With returns a new logger with context fields (for future extension)
func (l *Logger) With(fields map[string]interface{}) *Logger {
	// For now, just return the same logger
	// Can be extended to include fields in log output
	return l
}

// --- Default logger functions ---

func getDefault() *Logger {
	if defaultLogger == nil {
		defaultLogger, _ = New(DefaultConfig())
	}
	return defaultLogger
}

// Debug logs at DEBUG level using default logger
func Debug(format string, args ...interface{}) {
	getDefault().Debug(format, args...)
}

// Info logs at INFO level using default logger
func Info(format string, args ...interface{}) {
	getDefault().Info(format, args...)
}

// Warn logs at WARN level using default logger
func Warn(format string, args ...interface{}) {
	getDefault().Warn(format, args...)
}

// Error logs at ERROR level using default logger
func Error(format string, args ...interface{}) {
	getDefault().Error(format, args...)
}

// Fatal logs at FATAL level and exits using default logger
func Fatal(format string, args ...interface{}) {
	getDefault().Fatal(format, args...)
}

// SetLevel sets the log level for the default logger
func SetLevel(level Level) {
	getDefault().SetLevel(level)
}

// GetLevel gets the log level for the default logger
func GetLevel() Level {
	return getDefault().GetLevel()
}

// StdLogger returns a *log.Logger that writes to this logger
func (l *Logger) StdLogger(level Level) *log.Logger {
	return log.New(&logWriter{l: l, level: level}, "", 0)
}

type logWriter struct {
	l     *Logger
	level Level
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	msg := strings.TrimSuffix(string(p), "\n")
	w.l.log(w.level, "%s", msg)
	return len(p), nil
}
