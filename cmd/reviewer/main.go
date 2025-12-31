package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

func main() {
	// Flags
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:8000"), "DeepSeek server URL")
	model := flag.String("model", getEnv("LLM_MODEL", "deepseek-coder"), "Model name")
	filePath := flag.String("file", "", "File path being reviewed")
	diffFile := flag.String("diff", "", "Path to diff file (or use stdin)")
	contextFile := flag.String("context", "", "Path to full file for additional context (optional)")
	outputJSON := flag.Bool("json", false, "Output raw JSON only")
	timeout := flag.Duration("timeout", 5*time.Minute, "Request timeout")
	flag.Parse()

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "Error: -file is required")
		flag.Usage()
		os.Exit(1)
	}

	// Read diff
	var diff string
	if *diffFile != "" {
		data, err := os.ReadFile(*diffFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading diff file: %v\n", err)
			os.Exit(1)
		}
		diff = string(data)
	} else {
		// Check if stdin has data
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			// Data is being piped
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
			diff = string(data)
		}
	}

	if diff == "" {
		fmt.Fprintln(os.Stderr, "Error: no diff provided (use -diff flag or pipe to stdin)")
		os.Exit(1)
	}

	// Read optional context file
	var fullContent string
	if *contextFile != "" {
		data, err := os.ReadFile(*contextFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read context file: %v\n", err)
		} else {
			fullContent = string(data)
		}
	}

	// Initialize client
	client := llm.NewClient(llm.Config{
		BaseURL: *llmURL,
		Model:   *model,
		Timeout: *timeout,
	})

	analyzer := reviewer.NewAnalyzer(client)

	// Run review
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !*outputJSON {
		fmt.Fprintf(os.Stderr, "Analyzing %s...\n", *filePath)
		fmt.Fprintf(os.Stderr, "LLM Server: %s (model: %s)\n", *llmURL, *model)
	}

	result, err := analyzer.ReviewDiff(ctx, *filePath, diff, fullContent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Output result
	output, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(output))

	// Pretty print summary unless JSON-only mode
	if !*outputJSON {
		printSummary(result)
	}

	// Exit with non-zero if critical issues found
	for _, issue := range result.Issues {
		if issue.Severity == "CRITICAL" {
			os.Exit(2)
		}
	}
}

func printSummary(result *reviewer.ReviewResult) {
	fmt.Fprintln(os.Stderr, "\n"+strings.Repeat("=", 60))
	fmt.Fprintf(os.Stderr, "Risk Score: %d/10 | Recommendation: %s\n",
		result.RiskScore, result.Recommendation)
	fmt.Fprintf(os.Stderr, "Issues Found: %d\n", len(result.Issues))

	if len(result.Issues) > 0 {
		fmt.Fprintln(os.Stderr, "")
		for _, issue := range result.Issues {
			emoji := severityEmoji(issue.Severity)
			fmt.Fprintf(os.Stderr, "  %s [%s] %s (Line %d)\n",
				emoji, issue.Type, issue.Title, issue.Line)
			fmt.Fprintf(os.Stderr, "     %s\n", issue.Description)
		}
	}

	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintf(os.Stderr, "Summary: %s\n", result.Summary)
	fmt.Fprintln(os.Stderr, strings.Repeat("=", 60))
}

func severityEmoji(severity string) string {
	switch severity {
	case "CRITICAL":
		return "[CRITICAL]"
	case "HIGH":
		return "[HIGH]"
	case "MEDIUM":
		return "[MEDIUM]"
	case "LOW":
		return "[LOW]"
	default:
		return "[INFO]"
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
