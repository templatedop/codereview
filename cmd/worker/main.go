package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/yourorg/code-reviewer/internal/gitlab"
	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/logger"
	"github.com/yourorg/code-reviewer/internal/workflow"
)

const (
	TaskQueue = "code-review-queue"
)

func main() {
	// Flags
	temporalHost := flag.String("temporal", getEnv("TEMPORAL_HOST", "localhost:7233"), "Temporal server address")
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:11434"), "LLM server URL")
	llmModel := flag.String("model", getEnv("LLM_MODEL", "deepseek-coder:6.7b"), "LLM model name")
	gitlabURL := flag.String("gitlab-url", getEnv("GITLAB_URL", ""), "GitLab server URL")
	gitlabToken := flag.String("gitlab-token", getEnv("GITLAB_TOKEN", ""), "GitLab access token")
	knowledgeDir := flag.String("knowledge-dir", getEnv("KNOWLEDGE_DIR", ""), "Knowledge store directory")

	// Logging flags
	logLevel := flag.String("log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", getEnv("LOG_FORMAT", "text"), "Log format (text, json)")
	logOutput := flag.String("log-output", getEnv("LOG_OUTPUT", "stdout"), "Log output (stdout, stderr, or file path)")

	flag.Parse()

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:      logger.ParseLevel(*logLevel),
		Format:     *logFormat,
		Output:     *logOutput,
		TimeFormat: "2006-01-02 15:04:05",
		ShowCaller: *logLevel == "debug",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer log.Close()

	log.Info("Starting Code Review Worker...")
	log.Info("Temporal: %s", *temporalHost)
	log.Info("LLM: %s (model: %s)", *llmURL, *llmModel)

	// Create context for graceful shutdown
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: *temporalHost,
		Logger:   newTemporalLogger(log),
	})
	if err != nil {
		log.Fatal("Unable to create Temporal client: %v", err)
	}
	defer c.Close()

	// Create LLM client
	llmClient := llm.NewOllamaClient(llm.Config{
		BaseURL: *llmURL,
		Model:   *llmModel,
		Timeout: 5 * time.Minute,
	})

	// Create GitLab client (optional)
	var gitlabClient *gitlab.Client
	if *gitlabURL != "" && *gitlabToken != "" {
		gitlabClient = gitlab.NewClient(gitlab.Config{
			BaseURL: *gitlabURL,
			Token:   *gitlabToken,
		})
		log.Info("GitLab: %s", *gitlabURL)
	} else {
		log.Warn("GitLab not configured - review posting disabled")
	}

	// Create activities
	activities := workflow.NewActivities(gitlabClient, llmClient, *knowledgeDir)

	// Create worker
	w := worker.New(c, TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     5,
		MaxConcurrentWorkflowTaskExecutionSize: 10,
	})

	// Register workflow and activities
	w.RegisterWorkflow(workflow.CodeReviewWorkflow)
	w.RegisterActivity(activities.FetchMRDiffActivity)
	w.RegisterActivity(activities.LoadFrameworkActivity)
	w.RegisterActivity(activities.ReviewFileActivity)
	w.RegisterActivity(activities.PostCommentsActivity)

	log.Info("Worker listening on queue: %s", TaskQueue)
	log.Info("Press Ctrl+C to stop")

	// Setup graceful shutdown
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)

	// Run worker in background
	errCh := make(chan error, 1)
	go func() {
		errCh <- w.Run(worker.InterruptCh())
	}()

	// Wait for shutdown signal or error
	select {
	case <-shutdownCh:
		log.Info("Shutdown signal received, stopping worker...")
		cancel()

		// Give worker time to finish current tasks
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		// Wait for worker to stop
		select {
		case err := <-errCh:
			if err != nil {
				log.Error("Worker stopped with error: %v", err)
			}
		case <-shutdownCtx.Done():
			log.Warn("Shutdown timeout, forcing exit")
		}

		log.Info("Worker stopped gracefully")

	case err := <-errCh:
		if err != nil {
			log.Error("Worker failed: %v", err)
			os.Exit(1)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// temporalLogger adapts our logger for Temporal SDK
type temporalLogger struct {
	log *logger.Logger
}

func newTemporalLogger(log *logger.Logger) *temporalLogger {
	return &temporalLogger{log: log}
}

func (l *temporalLogger) Debug(msg string, keyvals ...interface{}) {
	l.log.Debug("%s %v", msg, keyvals)
}

func (l *temporalLogger) Info(msg string, keyvals ...interface{}) {
	l.log.Info("%s %v", msg, keyvals)
}

func (l *temporalLogger) Warn(msg string, keyvals ...interface{}) {
	l.log.Warn("%s %v", msg, keyvals)
}

func (l *temporalLogger) Error(msg string, keyvals ...interface{}) {
	l.log.Error("%s %v", msg, keyvals)
}
