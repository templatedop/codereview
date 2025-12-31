package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/yourorg/code-reviewer/internal/gitlab"
	"github.com/yourorg/code-reviewer/internal/llm"
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
	flag.Parse()

	fmt.Println("Starting Code Review Worker...")
	fmt.Printf("Temporal: %s\n", *temporalHost)
	fmt.Printf("LLM: %s (model: %s)\n", *llmURL, *llmModel)

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: *temporalHost,
	})
	if err != nil {
		log.Fatalf("Unable to create Temporal client: %v", err)
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
		fmt.Printf("GitLab: %s\n", *gitlabURL)
	}

	// Create activities
	activities := workflow.NewActivities(gitlabClient, llmClient, *knowledgeDir)

	// Create worker
	w := worker.New(c, TaskQueue, worker.Options{})

	// Register workflow and activities
	w.RegisterWorkflow(workflow.CodeReviewWorkflow)
	w.RegisterActivity(activities.FetchMRDiffActivity)
	w.RegisterActivity(activities.LoadFrameworkActivity)
	w.RegisterActivity(activities.ReviewFileActivity)
	w.RegisterActivity(activities.PostCommentsActivity)

	fmt.Printf("Worker listening on queue: %s\n", TaskQueue)
	fmt.Println("Press Ctrl+C to stop")

	// Run worker
	err = w.Run(worker.InterruptCh())
	if err != nil {
		log.Fatalf("Worker failed: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
