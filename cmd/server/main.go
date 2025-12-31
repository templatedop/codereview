package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

func main() {
	// Flags
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:8000"), "DeepSeek server URL")
	model := flag.String("model", getEnv("LLM_MODEL", "deepseek-coder"), "Model name")
	addr := flag.String("addr", getEnv("ADDR", ":8080"), "Server address")
	timeout := flag.Duration("timeout", 5*time.Minute, "LLM request timeout")
	flag.Parse()

	// Initialize LLM client
	client := llm.NewClient(llm.Config{
		BaseURL: *llmURL,
		Model:   *model,
		Timeout: *timeout,
	})

	analyzer := reviewer.NewAnalyzer(client)

	// Setup routes
	mux := http.NewServeMux()

	// Health check
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Review endpoint
	mux.HandleFunc("/api/review", handleReview(analyzer))

	// Batch review endpoint
	mux.HandleFunc("/api/review/batch", handleBatchReview(analyzer))

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
		log.Println("Shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		server.Shutdown(ctx)
	}()

	log.Printf("Code Review Server starting on %s", *addr)
	log.Printf("LLM: %s (model: %s)", *llmURL, *model)
	log.Printf("Endpoints:")
	log.Printf("  POST /api/review       - Review single file")
	log.Printf("  POST /api/review/batch - Review multiple files")
	log.Printf("  GET  /health           - Health check")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}
}

// ReviewRequest represents a single file review request
type ReviewRequest struct {
	FilePath    string `json:"file_path"`
	Diff        string `json:"diff"`
	FullContent string `json:"full_content,omitempty"`
}

// BatchReviewRequest represents multiple files to review
type BatchReviewRequest struct {
	Files []ReviewRequest `json:"files"`
}

// BatchReviewResponse contains results for multiple files
type BatchReviewResponse struct {
	Results []FileResult `json:"results"`
	Summary Summary      `json:"summary"`
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

func handleReview(analyzer *reviewer.Analyzer) http.HandlerFunc {
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
			http.Error(w, fmt.Sprintf("Review failed: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(result)
	}
}

func handleBatchReview(analyzer *reviewer.Analyzer) http.HandlerFunc {
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

		for _, file := range req.Files {
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)

			result, err := analyzer.ReviewDiff(ctx, file.FilePath, file.Diff, file.FullContent)
			cancel()

			fileResult := FileResult{FilePath: file.FilePath}
			if err != nil {
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
			}
			response.Results = append(response.Results, fileResult)
		}

		response.Summary.TotalFiles = len(req.Files)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		log.Printf("→ %s %s", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
		log.Printf("← %s %s (%v)", r.Method, r.URL.Path, time.Since(start))
	})
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
