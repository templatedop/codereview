// Command postgres-worker starts a Temporal worker for PostgreSQL code review.
package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/postgres"
	"github.com/yourorg/code-reviewer/internal/rag"
	"github.com/yourorg/code-reviewer/internal/workflow"
)

func main() {
	// Parse flags
	temporalHost := flag.String("temporal", getEnv("TEMPORAL_HOST", "localhost:7233"), "Temporal server address")
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:11434"), "LLM server URL")
	llmModel := flag.String("model", getEnv("LLM_MODEL", "qwen2.5-coder:32b"), "LLM model name")
	embedModel := flag.String("embed-model", getEnv("EMBED_MODEL", "nomic-embed-text"), "Embedding model name")
	embedDim := flag.Int("embed-dim", 768, "Embedding dimension")
	knowledgeDir := flag.String("knowledge-dir", getEnv("KNOWLEDGE_DIR", ""), "Knowledge base storage directory")
	dbLabURL := flag.String("dblab-url", getEnv("DBLAB_URL", ""), "Database Lab URL (optional)")
	dbLabToken := flag.String("dblab-token", getEnv("DBLAB_TOKEN", ""), "Database Lab token (optional)")
	taskQueue := flag.String("task-queue", workflow.PostgresReviewTaskQueue, "Temporal task queue name")
	flag.Parse()

	// Set up knowledge directory
	if *knowledgeDir == "" {
		homeDir, _ := os.UserHomeDir()
		*knowledgeDir = filepath.Join(homeDir, ".code-reviewer", "postgres-knowledge")
	}

	log.Printf("Starting PostgreSQL Review Worker")
	log.Printf("  Temporal: %s", *temporalHost)
	log.Printf("  LLM: %s (model: %s)", *llmURL, *llmModel)
	log.Printf("  Embedding: %s (dim: %d)", *embedModel, *embedDim)
	log.Printf("  Knowledge: %s", *knowledgeDir)
	log.Printf("  Task Queue: %s", *taskQueue)

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: *temporalHost,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	// Create LLM client
	llmClient := llm.NewClient(llm.Config{
		BaseURL: *llmURL,
		Model:   *llmModel,
	})

	// Create embedder
	embedder := rag.NewOllamaEmbedder(*llmURL, *embedModel, *embedDim)

	// Create vector store
	vectorStore := rag.NewMemoryVectorStore(*knowledgeDir)

	// Create retriever
	retriever := rag.NewRetriever(embedder, vectorStore, nil)

	// Create analyzer
	analyzer := postgres.NewAnalyzer(llmClient, retriever)

	// Create Database Lab client (optional)
	var dbLabClient *postgres.DatabaseLabClient
	if *dbLabURL != "" {
		dbLabClient = postgres.NewDatabaseLabClient(postgres.DatabaseLabConfig{
			BaseURL: *dbLabURL,
			Token:   *dbLabToken,
		})
		log.Printf("  Database Lab: %s", *dbLabURL)
	}

	// Create activities
	activities := workflow.NewPostgresActivities(analyzer, dbLabClient, retriever)

	// Create worker
	w := worker.New(c, *taskQueue, worker.Options{})

	// Register workflow
	w.RegisterWorkflow(workflow.PostgresReviewWorkflow)

	// Register activities
	w.RegisterActivity(activities.ExtractSQLActivity)
	w.RegisterActivity(activities.PerformanceAnalysisActivity)
	w.RegisterActivity(activities.SecurityAnalysisActivity)
	w.RegisterActivity(activities.StandardsAnalysisActivity)
	w.RegisterActivity(activities.DatabaseLabValidationActivity)
	w.RegisterActivity(activities.PostPostgresReviewActivity)
	w.RegisterActivity(activities.RAGSearchActivity)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down worker...")
	}()

	// Start worker
	log.Println("Worker started, waiting for tasks...")
	if err := w.Run(worker.InterruptCh()); err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
