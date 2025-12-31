package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/yourorg/code-reviewer/internal/knowledge"
	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

func main() {
	// Flags
	llmURL := flag.String("llm-url", getEnv("LLM_URL", "http://localhost:11434"), "LLM server URL")
	model := flag.String("model", getEnv("LLM_MODEL", "deepseek-coder:6.7b"), "Model name")
	useOllama := flag.Bool("ollama", getEnvBool("USE_OLLAMA", true), "Use Ollama API (default: true)")
	filePath := flag.String("file", "", "File to review (can be local file or just filename with -diff)")
	diffFile := flag.String("diff", "", "Path to diff file (optional - if not provided, reviews whole file)")
	contextFile := flag.String("context", "", "Path to full file for additional context (optional)")
	frameworkName := flag.String("framework", "", "Name of indexed framework to use for context-aware review")
	dataDir := flag.String("data-dir", "", "Directory where frameworks are stored")
	outputJSON := flag.Bool("json", false, "Output raw JSON only")
	timeout := flag.Duration("timeout", 5*time.Minute, "Request timeout")
	flag.Parse()

	// Allow positional argument for file
	if *filePath == "" && flag.NArg() > 0 {
		*filePath = flag.Arg(0)
	}

	if *filePath == "" {
		fmt.Fprintln(os.Stderr, "Error: file path is required")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Usage:")
		fmt.Fprintln(os.Stderr, "  code-reviewer [flags] <file>")
		fmt.Fprintln(os.Stderr, "  code-reviewer -file <file>")
		fmt.Fprintln(os.Stderr, "  code-reviewer -file <file> -diff <diff-file>")
		fmt.Fprintln(os.Stderr, "  code-reviewer -framework <name> <file>   # Framework-aware review")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Examples:")
		fmt.Fprintln(os.Stderr, "  code-reviewer mycode.go                          # Review entire file")
		fmt.Fprintln(os.Stderr, "  code-reviewer -framework gin handler.go          # Review with Gin framework context")
		fmt.Fprintln(os.Stderr, "  code-reviewer -file src/handler.go               # Review entire file")
		fmt.Fprintln(os.Stderr, "  code-reviewer -file main.go -diff changes.patch")
		fmt.Fprintln(os.Stderr, "")
		flag.Usage()
		os.Exit(1)
	}

	var diff string
	var fullContent string

	// Check if file exists locally (for direct file review)
	fileExists := false
	if _, err := os.Stat(*filePath); err == nil {
		fileExists = true
	}

	// If diff file provided, use it
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

	// If no diff provided but file exists, review the whole file
	if diff == "" && fileExists {
		data, err := os.ReadFile(*filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		fileContent := string(data)
		fullContent = fileContent

		// Create a synthetic diff (all lines as additions)
		diff = createSyntheticDiff(*filePath, fileContent)
	}

	if diff == "" {
		fmt.Fprintln(os.Stderr, "Error: no input provided")
		fmt.Fprintln(os.Stderr, "Provide either:")
		fmt.Fprintln(os.Stderr, "  - A local file to review: code-reviewer myfile.go")
		fmt.Fprintln(os.Stderr, "  - A diff file: code-reviewer -file name.go -diff changes.patch")
		fmt.Fprintln(os.Stderr, "  - Piped input: git diff | code-reviewer -file name.go")
		os.Exit(1)
	}

	// Read optional context file (overrides auto-read)
	if *contextFile != "" {
		data, err := os.ReadFile(*contextFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not read context file: %v\n", err)
		} else {
			fullContent = string(data)
		}
	}

	// Load framework if specified
	var frameworkCtx *reviewer.FrameworkContext
	if *frameworkName != "" {
		store := knowledge.NewStore(*dataDir)
		framework, err := store.LoadFramework(*frameworkName)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading framework '%s': %v\n", *frameworkName, err)
			fmt.Fprintln(os.Stderr, "Use 'indexer list' to see available frameworks.")
			os.Exit(1)
		}

		// Search for relevant framework code based on the diff content
		relevantElements := store.Search(diff+" "+fullContent, 5)

		// Convert to framework context
		var elements []llm.FrameworkElement
		for _, elem := range relevantElements {
			elements = append(elements, llm.FrameworkElement{
				Type:    elem.Type,
				Package: elem.Package,
				Name:    elem.Name,
				Doc:     elem.Doc,
				Body:    truncateBody(elem.Body, 500), // Limit body size
			})
		}

		frameworkCtx = &reviewer.FrameworkContext{
			Name:     framework.Name,
			Elements: elements,
		}
	}

	// Initialize client based on backend type
	var client reviewer.LLMClient
	if *useOllama {
		client = llm.NewOllamaClient(llm.Config{
			BaseURL: *llmURL,
			Model:   *model,
			Timeout: *timeout,
		})
	} else {
		client = llm.NewClient(llm.Config{
			BaseURL: *llmURL,
			Model:   *model,
			Timeout: *timeout,
		})
	}

	analyzer := reviewer.NewAnalyzer(client)

	// Run review
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	if !*outputJSON {
		backend := "OpenAI-compatible"
		if *useOllama {
			backend = "Ollama"
		}
		reviewType := "diff"
		if fileExists && *diffFile == "" {
			reviewType = "full file"
		}
		fmt.Fprintf(os.Stderr, "Analyzing %s (%s)...\n", *filePath, reviewType)
		fmt.Fprintf(os.Stderr, "LLM Server: %s (model: %s, backend: %s)\n", *llmURL, *model, backend)
		if frameworkCtx != nil {
			fmt.Fprintf(os.Stderr, "Framework: %s (%d relevant patterns loaded)\n",
				frameworkCtx.Name, len(frameworkCtx.Elements))
		}
	}

	var result *reviewer.ReviewResult
	var err error

	if frameworkCtx != nil {
		result, err = analyzer.ReviewDiffWithFramework(ctx, *filePath, diff, fullContent, *frameworkCtx)
	} else {
		result, err = analyzer.ReviewDiff(ctx, *filePath, diff, fullContent)
	}

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

// createSyntheticDiff creates a diff that shows all lines as additions
func createSyntheticDiff(filePath, content string) string {
	var sb strings.Builder
	fileName := filepath.Base(filePath)

	sb.WriteString(fmt.Sprintf("diff --git a/%s b/%s\n", fileName, fileName))
	sb.WriteString("new file mode 100644\n")
	sb.WriteString(fmt.Sprintf("--- /dev/null\n"))
	sb.WriteString(fmt.Sprintf("+++ b/%s\n", fileName))

	lines := strings.Split(content, "\n")
	sb.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(lines)))

	for _, line := range lines {
		sb.WriteString("+" + line + "\n")
	}

	return sb.String()
}

// truncateBody limits the size of code body for prompt
func truncateBody(body string, maxLen int) string {
	if len(body) <= maxLen {
		return body
	}
	return body[:maxLen] + "\n// ... truncated"
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

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v == "true" || v == "1" || v == "yes"
}
