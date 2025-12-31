package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/code-reviewer/internal/gitlab"
	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/logger"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

var log *logger.Logger

func main() {
	// Flags
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:11434"), "LLM server URL")
	model := flag.String("model", getEnv("LLM_MODEL", "deepseek-coder:1.3b"), "Model name")
	addr := flag.String("addr", getEnv("ADDR", ":8080"), "Server address")
	timeout := flag.Duration("timeout", 5*time.Minute, "LLM request timeout")

	// GitLab flags
	gitlabURL := flag.String("gitlab-url", getEnv("GITLAB_URL", ""), "GitLab server URL")
	gitlabToken := flag.String("gitlab-token", getEnv("GITLAB_TOKEN", ""), "GitLab access token")

	// Logging flags
	logLevel := flag.String("log-level", getEnv("LOG_LEVEL", "info"), "Log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", getEnv("LOG_FORMAT", "text"), "Log format (text, json)")
	logOutput := flag.String("log-output", getEnv("LOG_OUTPUT", "stdout"), "Log output (stdout, stderr, or file path)")

	flag.Parse()

	// Initialize logger
	var err error
	log, err = logger.New(logger.Config{
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

	// Initialize LLM client (use Ollama by default)
	llmClient := llm.NewOllamaClient(llm.Config{
		BaseURL: *llmURL,
		Model:   *model,
		Timeout: *timeout,
	})

	analyzer := reviewer.NewAnalyzer(llmClient)

	// Initialize GitLab client (optional)
	var gitlabClient *gitlab.Client
	if *gitlabURL != "" && *gitlabToken != "" {
		gitlabClient = gitlab.NewClient(gitlab.Config{
			BaseURL: *gitlabURL,
			Token:   *gitlabToken,
		})
		log.Info("GitLab integration enabled: %s", *gitlabURL)
	}

	// Setup routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Review endpoint
	mux.HandleFunc("/api/review", handleReview(analyzer, gitlabClient))

	// Batch review endpoint
	mux.HandleFunc("/api/review/batch", handleBatchReview(analyzer, gitlabClient))

	// GitLab webhook endpoint
	mux.HandleFunc("/webhook", handleWebhook(analyzer, gitlabClient))

	// Server
	server := &http.Server{
		Addr:         *addr,
		Handler:      logMiddleware(mux),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 10 * time.Minute,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Info("Shutdown signal received, stopping server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Error("Server shutdown error: %v", err)
		}
		log.Info("Server stopped gracefully")
	}()

	log.Info("Code Review Server starting on %s", *addr)
	log.Info("LLM: %s (model: %s)", *llmURL, *model)
	log.Info("Endpoints:")
	log.Info("  POST /api/review       - Review single file")
	log.Info("  POST /api/review/batch - Review multiple files")
	log.Info("  POST /webhook          - GitLab webhook")
	log.Info("  GET  /health           - Health check")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal("Server error: %v", err)
	}
}

// ReviewRequest represents a single file review request
type ReviewRequest struct {
	FilePath    string `json:"file_path"`
	Diff        string `json:"diff"`
	FullContent string `json:"full_content,omitempty"`
	// GitLab integration fields
	ProjectID  int `json:"project_id,omitempty"`
	MergeReqID int `json:"merge_request_id,omitempty"`
	PostToMR   bool `json:"post_to_mr,omitempty"`
}

// BatchReviewRequest represents multiple files to review
type BatchReviewRequest struct {
	Files      []ReviewRequest `json:"files"`
	ProjectID  int             `json:"project_id,omitempty"`
	MergeReqID int             `json:"merge_request_id,omitempty"`
	PostToMR   bool            `json:"post_to_mr,omitempty"`
}

// BatchReviewResponse contains results for multiple files
type BatchReviewResponse struct {
	Results   []FileResult `json:"results"`
	Summary   Summary      `json:"summary"`
	PostedToMR bool        `json:"posted_to_mr,omitempty"`
}

type FileResult struct {
	FilePath string                 `json:"file_path"`
	Result   *reviewer.ReviewResult `json:"result,omitempty"`
	Error    string                 `json:"error,omitempty"`
}

type Summary struct {
	TotalFiles     int `json:"total_files"`
	FilesReviewed  int `json:"files_reviewed"`
	TotalIssues    int `json:"total_issues"`
	CriticalIssues int `json:"critical_issues"`
	HighIssues     int `json:"high_issues"`
	MediumIssues   int `json:"medium_issues"`
	LowIssues      int `json:"low_issues"`
}

func handleReview(analyzer *reviewer.Analyzer, gitlabClient *gitlab.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
			return
		}

		if req.FilePath == "" || req.Diff == "" {
			http.Error(w, "file_path and diff are required", http.StatusBadRequest)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
		defer cancel()

		result, err := analyzer.ReviewDiff(ctx, req.FilePath, req.Diff, req.FullContent)
		if err != nil {
			log.Error("Review failed for %s: %v", req.FilePath, err)
			http.Error(w, fmt.Sprintf("Review failed: %v", err), http.StatusInternalServerError)
			return
		}

		// Post to GitLab if requested
		if req.PostToMR && gitlabClient != nil && req.ProjectID > 0 && req.MergeReqID > 0 {
			poster := gitlab.NewReviewPoster(gitlabClient, gitlab.ReviewPosterConfig{
				PostInlineComments: true,
				Logger:             log,
			})
			review := gitlab.FileReview{
				FilePath: req.FilePath,
				Result:   result,
			}
			if err := poster.PostSingleReview(ctx, req.ProjectID, req.MergeReqID, review); err != nil {
				log.Warn("Failed to post review to GitLab: %v", err)
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func handleBatchReview(analyzer *reviewer.Analyzer, gitlabClient *gitlab.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		var req BatchReviewRequest
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request: %v", err), http.StatusBadRequest)
			return
		}

		response := BatchReviewResponse{
			Results: make([]FileResult, 0, len(req.Files)),
		}

		var gitlabReviews []gitlab.FileReview

		for _, file := range req.Files {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)

			result, err := analyzer.ReviewDiff(ctx, file.FilePath, file.Diff, file.FullContent)
			cancel()

			fileResult := FileResult{FilePath: file.FilePath}
			if err != nil {
				log.Error("Review failed for %s: %v", file.FilePath, err)
				fileResult.Error = err.Error()
			} else {
				fileResult.Result = result
				response.Summary.FilesReviewed++

				for _, issue := range result.Issues {
					response.Summary.TotalIssues++
					switch issue.Severity {
					case "CRITICAL":
						response.Summary.CriticalIssues++
					case "HIGH":
						response.Summary.HighIssues++
					case "MEDIUM":
						response.Summary.MediumIssues++
					case "LOW":
						response.Summary.LowIssues++
					}
				}

				gitlabReviews = append(gitlabReviews, gitlab.FileReview{
					FilePath: file.FilePath,
					Result:   result,
				})
			}
			response.Results = append(response.Results, fileResult)
		}

		response.Summary.TotalFiles = len(req.Files)

		// Post to GitLab if requested
		if req.PostToMR && gitlabClient != nil && req.ProjectID > 0 && req.MergeReqID > 0 {
			poster := gitlab.NewReviewPoster(gitlabClient, gitlab.ReviewPosterConfig{
				PostInlineComments: true,
				Logger:             log,
			})
			if err := poster.PostReviews(r.Context(), req.ProjectID, req.MergeReqID, gitlabReviews); err != nil {
				log.Warn("Failed to post reviews to GitLab: %v", err)
			} else {
				response.PostedToMR = true
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

// GitLabWebhookEvent represents a GitLab webhook payload
type GitLabWebhookEvent struct {
	ObjectKind       string `json:"object_kind"`
	Project          struct {
		ID int `json:"id"`
	} `json:"project"`
	ObjectAttributes struct {
		IID          int    `json:"iid"`
		Action       string `json:"action"`
		SourceBranch string `json:"source_branch"`
		TargetBranch string `json:"target_branch"`
	} `json:"object_attributes"`
}

func handleWebhook(analyzer *reviewer.Analyzer, gitlabClient *gitlab.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if gitlabClient == nil {
			http.Error(w, "GitLab not configured", http.StatusServiceUnavailable)
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read body", http.StatusBadRequest)
			return
		}

		var event GitLabWebhookEvent
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, fmt.Sprintf("Invalid webhook: %v", err), http.StatusBadRequest)
			return
		}

		// Only handle merge request events
		if event.ObjectKind != "merge_request" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ignored", "reason": "not a merge request event"})
			return
		}

		// Only handle open/update actions
		action := event.ObjectAttributes.Action
		if action != "open" && action != "update" && action != "reopen" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ignored", "reason": "action not relevant"})
			return
		}

		projectID := event.Project.ID
		mrIID := event.ObjectAttributes.IID

		log.Info("Received MR webhook: project=%d mr=%d action=%s", projectID, mrIID, action)

		// Fetch MR changes
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		changes, err := gitlabClient.GetMergeRequestChanges(ctx, projectID, mrIID)
		if err != nil {
			log.Error("Failed to fetch MR changes: %v", err)
			http.Error(w, fmt.Sprintf("Failed to fetch changes: %v", err), http.StatusInternalServerError)
			return
		}

		var reviews []gitlab.FileReview

		for _, change := range changes.Changes {
			if change.DeletedFile {
				continue
			}

			result, err := analyzer.ReviewDiff(ctx, change.NewPath, change.Diff, "")
			review := gitlab.FileReview{FilePath: change.NewPath}
			if err != nil {
				log.Error("Review failed for %s: %v", change.NewPath, err)
				review.Error = err.Error()
			} else {
				review.Result = result
			}
			reviews = append(reviews, review)
		}

		// Post reviews to GitLab
		poster := gitlab.NewReviewPoster(gitlabClient, gitlab.ReviewPosterConfig{
			PostInlineComments: true,
			Logger:             log,
		})
		if err := poster.PostReviews(ctx, projectID, mrIID, reviews); err != nil {
			log.Error("Failed to post reviews: %v", err)
			http.Error(w, fmt.Sprintf("Failed to post reviews: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "success",
			"files_reviewed": len(reviews),
		})
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Debug("→ %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Info("← %s %s (%v)", r.Method, r.URL.Path, time.Since(start))
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
