package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/yourorg/code-reviewer/internal/workflow"
)

const (
	TaskQueue = "code-review-queue"
)

func main() {
	// Flags
	temporalHost := flag.String("temporal", getEnv("TEMPORAL_HOST", "localhost:7233"), "Temporal server address")

	// Review options
	filePath := flag.String("file", "", "Local file to review")
	diffFile := flag.String("diff", "", "Diff file to review")
	frameworkName := flag.String("framework", "", "Framework name for context")
	useAgent := flag.Bool("agent", false, "Use agentic review (multi-step)")

	// GitLab options
	projectID := flag.Int("project", 0, "GitLab project ID")
	mergeReqID := flag.Int("mr", 0, "GitLab merge request ID")

	// Output
	waitResult := flag.Bool("wait", true, "Wait for result")
	timeout := flag.Duration("timeout", 10*time.Minute, "Workflow timeout")

	flag.Parse()

	if *filePath == "" && *projectID == 0 {
		fmt.Fprintln(os.Stderr, "Error: either -file or -project/-mr is required")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  trigger -file mycode.go                  # Review local file")
		fmt.Fprintln(os.Stderr, "  trigger -file mycode.go -agent           # Use agentic review")
		fmt.Fprintln(os.Stderr, "  trigger -file mycode.go -framework gin   # With framework context")
		fmt.Fprintln(os.Stderr, "  trigger -project 123 -mr 456             # Review GitLab MR")
		os.Exit(1)
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: *temporalHost,
	})
	if err != nil {
		log.Fatalf("Unable to create Temporal client: %v", err)
	}
	defer c.Close()

	// Build input
	input := workflow.CodeReviewInput{
		ProjectID:     *projectID,
		MergeReqID:    *mergeReqID,
		FrameworkName: *frameworkName,
		UseAgent:      *useAgent,
	}

	// Read local file if provided
	if *filePath != "" {
		input.FilePath = *filePath

		if *diffFile != "" {
			data, err := os.ReadFile(*diffFile)
			if err != nil {
				log.Fatalf("Error reading diff file: %v", err)
			}
			input.Diff = string(data)
		} else {
			// Read file content
			data, err := os.ReadFile(*filePath)
			if err != nil {
				log.Fatalf("Error reading file: %v", err)
			}
			input.FullContent = string(data)
			input.Diff = createSyntheticDiff(*filePath, string(data))
		}
	}

	// Start workflow
	workflowID := fmt.Sprintf("code-review-%d", time.Now().UnixNano())
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}

	fmt.Printf("Starting workflow: %s\n", workflowID)
	fmt.Printf("File: %s\n", input.FilePath)
	if input.FrameworkName != "" {
		fmt.Printf("Framework: %s\n", input.FrameworkName)
	}
	if input.UseAgent {
		fmt.Println("Mode: Agentic (multi-step)")
	}

	we, err := c.ExecuteWorkflow(context.Background(), options, workflow.CodeReviewWorkflow, input)
	if err != nil {
		log.Fatalf("Unable to start workflow: %v", err)
	}

	fmt.Printf("Workflow started: %s\n", we.GetID())

	if *waitResult {
		fmt.Println("Waiting for result...")

		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		var result workflow.CodeReviewOutput
		err = we.Get(ctx, &result)
		if err != nil {
			log.Fatalf("Workflow failed: %v", err)
		}

		// Print result
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println("\n=== Result ===")
		fmt.Println(string(output))

		if result.Result != nil {
			fmt.Printf("\nFiles Reviewed: %d\n", result.FilesReviewed)
			fmt.Printf("Total Issues: %d\n", result.TotalIssues)
			fmt.Printf("Recommendation: %s\n", result.Result.Recommendation)
			fmt.Printf("Risk Score: %d/10\n", result.Result.RiskScore)

			if len(result.Result.Issues) > 0 {
				fmt.Println("\nIssues:")
				for _, issue := range result.Result.Issues {
					fmt.Printf("  [%s] %s (line %d): %s\n",
						issue.Severity, issue.Type, issue.Line, issue.Title)
				}
			}
		}
	}
}

func createSyntheticDiff(filePath, content string) string {
	var result string
	result += fmt.Sprintf("diff --git a/%s b/%s\n", filePath, filePath)
	result += "new file mode 100644\n"
	result += "--- /dev/null\n"
	result += fmt.Sprintf("+++ b/%s\n", filePath)

	lines := 0
	for _, c := range content {
		if c == '\n' {
			lines++
		}
	}
	result += fmt.Sprintf("@@ -0,0 +1,%d @@\n", lines+1)

	for _, line := range splitLines(content) {
		result += "+" + line + "\n"
	}

	return result
}

func splitLines(s string) []string {
	var lines []string
	var current string
	for _, c := range s {
		if c == '\n' {
			lines = append(lines, current)
			current = ""
		} else {
			current += string(c)
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return lines
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
