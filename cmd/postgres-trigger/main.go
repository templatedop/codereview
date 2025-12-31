// Command postgres-trigger triggers PostgreSQL review workflows.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/yourorg/code-reviewer/internal/workflow"
)

func main() {
	// Parse flags
	temporalHost := flag.String("temporal", getEnv("TEMPORAL_HOST", "localhost:7233"), "Temporal server address")
	taskQueue := flag.String("task-queue", workflow.PostgresReviewTaskQueue, "Temporal task queue name")

	// File options
	filePath := flag.String("file", "", "Path to file to analyze")
	dir := flag.String("dir", "", "Directory to analyze (all SQL files)")
	diff := flag.String("diff", "", "Git diff to analyze")

	// Analysis options
	skipPerformance := flag.Bool("skip-performance", false, "Skip performance analysis")
	skipSecurity := flag.Bool("skip-security", false, "Skip security analysis")
	skipStandards := flag.Bool("skip-standards", false, "Skip standards analysis")
	enableDBLab := flag.Bool("dblab", false, "Enable Database Lab validation")
	orgStandards := flag.String("org-standards", "", "Path to organization standards file")
	minSeverity := flag.String("min-severity", "LOW", "Minimum severity to report (CRITICAL, HIGH, MEDIUM, LOW)")

	// Output options
	waitResult := flag.Bool("wait", false, "Wait for workflow completion")
	timeout := flag.Duration("timeout", 10*time.Minute, "Timeout when waiting for result")
	jsonOutput := flag.Bool("json", false, "Output result as JSON")

	flag.Parse()

	if *filePath == "" && *dir == "" && *diff == "" {
		fmt.Println("Usage: postgres-trigger -file <path> | -dir <path> | -diff <diff>")
		fmt.Println("\nOptions:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Collect files to analyze
	var files []workflow.FileInfo

	if *filePath != "" {
		content, err := os.ReadFile(*filePath)
		if err != nil {
			log.Fatalf("Failed to read file: %v", err)
		}
		files = append(files, workflow.FileInfo{
			Path:        *filePath,
			Content:     string(content),
			Language:    detectLanguage(*filePath),
			IsMigration: isMigrationFile(*filePath),
		})
	}

	if *dir != "" {
		err := filepath.Walk(*dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if !isRelevantFile(path) {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return nil // Skip unreadable files
			}
			files = append(files, workflow.FileInfo{
				Path:        path,
				Content:     string(content),
				Language:    detectLanguage(path),
				IsMigration: isMigrationFile(path),
			})
			return nil
		})
		if err != nil {
			log.Fatalf("Failed to walk directory: %v", err)
		}
	}

	if *diff != "" {
		files = append(files, workflow.FileInfo{
			Path:     "diff",
			Diff:     *diff,
			Language: "go", // Default assumption
		})
	}

	if len(files) == 0 {
		log.Fatal("No files to analyze")
	}

	// Read org standards if provided
	var standards string
	if *orgStandards != "" {
		content, err := os.ReadFile(*orgStandards)
		if err != nil {
			log.Fatalf("Failed to read org standards: %v", err)
		}
		standards = string(content)
	}

	// Create workflow input
	input := workflow.PostgresReviewInput{
		Files: files,
		Options: workflow.PostgresReviewOptions{
			EnableDBLab:     *enableDBLab,
			OrgStandards:    standards,
			MinSeverity:     *minSeverity,
			SkipPerformance: *skipPerformance,
			SkipSecurity:    *skipSecurity,
			SkipStandards:   *skipStandards,
		},
	}

	log.Printf("Analyzing %d files...", len(files))

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort: *temporalHost,
	})
	if err != nil {
		log.Fatalf("Failed to create Temporal client: %v", err)
	}
	defer c.Close()

	// Start workflow
	ctx := context.Background()
	workflowID := fmt.Sprintf("postgres-review-%d", time.Now().UnixNano())

	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: *taskQueue,
	}

	we, err := c.ExecuteWorkflow(ctx, options, workflow.PostgresReviewWorkflow, input)
	if err != nil {
		log.Fatalf("Failed to start workflow: %v", err)
	}

	log.Printf("Started workflow: %s (run ID: %s)", we.GetID(), we.GetRunID())

	if *waitResult {
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()

		var result workflow.PostgresReviewResult
		if err := we.Get(ctx, &result); err != nil {
			log.Fatalf("Workflow failed: %v", err)
		}

		if *jsonOutput {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
		} else {
			printResult(&result)
		}
	} else {
		log.Printf("Workflow started. Use Temporal UI to monitor progress.")
		log.Printf("  Workflow ID: %s", we.GetID())
		log.Printf("  Run ID: %s", we.GetRunID())
	}
}

func printResult(result *workflow.PostgresReviewResult) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("PostgreSQL Code Review Results")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Printf("\nStatus: %s\n", result.Status)
	fmt.Printf("Files Analyzed: %d\n", result.FilesAnalyzed)
	fmt.Printf("SQL Queries Found: %d\n", result.SQLQueriesFound)
	fmt.Printf("Processing Time: %s\n", result.ProcessingTime)

	fmt.Printf("\nIssues Summary:\n")
	fmt.Printf("  Total: %d\n", result.TotalIssues)
	if result.CriticalCount > 0 {
		fmt.Printf("  🔴 Critical: %d\n", result.CriticalCount)
	}
	if result.HighCount > 0 {
		fmt.Printf("  🟠 High: %d\n", result.HighCount)
	}
	if result.MediumCount > 0 {
		fmt.Printf("  🟡 Medium: %d\n", result.MediumCount)
	}
	if result.LowCount > 0 {
		fmt.Printf("  🔵 Low: %d\n", result.LowCount)
	}

	if len(result.PerformanceIssues) > 0 {
		fmt.Printf("\n📊 Performance Issues (%d):\n", len(result.PerformanceIssues))
		for i, issue := range result.PerformanceIssues {
			printIssue(i+1, issue)
		}
	}

	if len(result.SecurityIssues) > 0 {
		fmt.Printf("\n🔒 Security Issues (%d):\n", len(result.SecurityIssues))
		for i, issue := range result.SecurityIssues {
			printIssue(i+1, issue)
		}
	}

	if len(result.StandardsIssues) > 0 {
		fmt.Printf("\n📋 Standards Issues (%d):\n", len(result.StandardsIssues))
		for i, issue := range result.StandardsIssues {
			printIssue(i+1, issue)
		}
	}

	if len(result.IndexSuggestions) > 0 {
		fmt.Printf("\n📈 Index Suggestions (%d):\n", len(result.IndexSuggestions))
		for i, idx := range result.IndexSuggestions {
			fmt.Printf("  %d. Table: %s, Columns: %s\n", i+1, idx.Table, idx.Columns)
			fmt.Printf("     Reason: %s\n", idx.Reason)
			fmt.Printf("     SQL: %s\n", idx.CreateStmt)
		}
	}

	if result.MigrationSafety != nil {
		fmt.Printf("\n🔄 Migration Safety:\n")
		if result.MigrationSafety.IsSafe {
			fmt.Printf("  ✅ Safe to run\n")
		} else {
			fmt.Printf("  ⚠️  May cause issues\n")
			if len(result.MigrationSafety.BlockingOperations) > 0 {
				fmt.Printf("  Blocking operations:\n")
				for _, op := range result.MigrationSafety.BlockingOperations {
					fmt.Printf("    - %s\n", op)
				}
			}
			if len(result.MigrationSafety.Recommendations) > 0 {
				fmt.Printf("  Recommendations:\n")
				for _, rec := range result.MigrationSafety.Recommendations {
					fmt.Printf("    - %s\n", rec)
				}
			}
		}
	}

	fmt.Println()
}

func printIssue(num int, issue interface{}) {
	// Type assertion for postgres.Issue
	type issueType struct {
		Severity    string `json:"severity"`
		Category    string `json:"category"`
		Line        int    `json:"line"`
		Problem     string `json:"problem"`
		Suggestion  string `json:"suggestion"`
		Description string `json:"description"`
	}

	data, _ := json.Marshal(issue)
	var i issueType
	json.Unmarshal(data, &i)

	severityIcon := map[string]string{
		"CRITICAL": "🔴",
		"HIGH":     "🟠",
		"MEDIUM":   "🟡",
		"LOW":      "🔵",
		"INFO":     "⚪",
		"ERROR":    "🟠",
		"WARNING":  "🟡",
	}

	icon := severityIcon[i.Severity]
	if icon == "" {
		icon = "⚪"
	}

	fmt.Printf("  %d. %s [%s] ", num, icon, i.Severity)
	if i.Line > 0 {
		fmt.Printf("Line %d: ", i.Line)
	}
	fmt.Printf("%s\n", i.Problem)
	if i.Suggestion != "" {
		fmt.Printf("     💡 %s\n", i.Suggestion)
	}
}

func detectLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".java":
		return "java"
	case ".js":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".sql", ".pgsql":
		return "sql"
	default:
		return ext
	}
}

func isMigrationFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "migration") ||
		strings.Contains(lower, "migrate") ||
		strings.Contains(lower, "/db/") ||
		strings.Contains(lower, "/schema/")
}

func isRelevantFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	relevant := []string{".go", ".py", ".java", ".js", ".ts", ".tsx", ".rb", ".php", ".sql", ".pgsql"}
	for _, r := range relevant {
		if ext == r {
			return true
		}
	}
	return false
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
