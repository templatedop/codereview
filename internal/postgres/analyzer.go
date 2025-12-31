package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/yourorg/code-reviewer/internal/rag"
)

// LLMClient interface for LLM interactions.
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Analyzer provides PostgreSQL-specific code analysis with RAG.
type Analyzer struct {
	llm       LLMClient
	retriever *rag.Retriever
	extractor *SQLExtractor
}

// NewAnalyzer creates a new PostgreSQL analyzer.
func NewAnalyzer(llm LLMClient, retriever *rag.Retriever) *Analyzer {
	return &Analyzer{
		llm:       llm,
		retriever: retriever,
		extractor: NewSQLExtractor(),
	}
}

// AnalysisInput contains input for analysis.
type AnalysisInput struct {
	SQLFragments []SQLFragment
	FileContent  string
	FilePath     string
	Language     string
	OrgStandards string
}

// Issue represents a detected issue.
type Issue struct {
	Severity    string            `json:"severity"`
	Category    string            `json:"category"`
	Type        string            `json:"type,omitempty"`
	Line        int               `json:"line"`
	EndLine     int               `json:"end_line,omitempty"`
	Query       string            `json:"query,omitempty"`
	Code        string            `json:"code,omitempty"`
	Problem     string            `json:"problem"`
	Description string            `json:"description,omitempty"`
	Impact      string            `json:"impact,omitempty"`
	Suggestion  string            `json:"suggestion"`
	Reference   string            `json:"reference,omitempty"`
	CWE         string            `json:"cwe,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// PerformanceResult contains performance analysis results.
type PerformanceResult struct {
	Issues          []Issue              `json:"issues"`
	IndexSuggestions []IndexSuggestion   `json:"indexes_to_create,omitempty"`
	QueryRewrites   []QueryRewrite       `json:"queries_to_rewrite,omitempty"`
}

// IndexSuggestion suggests an index creation.
type IndexSuggestion struct {
	Table      string `json:"table"`
	Columns    string `json:"columns"`
	Type       string `json:"type,omitempty"`
	Reason     string `json:"reason"`
	CreateStmt string `json:"create_statement"`
}

// QueryRewrite suggests a query optimization.
type QueryRewrite struct {
	Original  string `json:"original"`
	Optimized string `json:"optimized"`
	Reason    string `json:"reason"`
}

// SecurityResult contains security analysis results.
type SecurityResult struct {
	Vulnerabilities []Issue `json:"vulnerabilities"`
}

// StandardsResult contains standards analysis results.
type StandardsResult struct {
	Violations     []Issue         `json:"violations"`
	MigrationSafety MigrationSafety `json:"migration_safety,omitempty"`
}

// MigrationSafety contains migration safety analysis.
type MigrationSafety struct {
	IsSafe             bool     `json:"is_safe"`
	BlockingOperations []string `json:"blocking_operations,omitempty"`
	Recommendations    []string `json:"recommendations,omitempty"`
}

// AnalyzePerformance analyzes SQL for performance issues.
func (a *Analyzer) AnalyzePerformance(ctx context.Context, input AnalysisInput) (*PerformanceResult, error) {
	if len(input.SQLFragments) == 0 {
		return &PerformanceResult{}, nil
	}

	// Build query for RAG retrieval
	queries := make([]string, 0, len(input.SQLFragments))
	for _, frag := range input.SQLFragments {
		queries = append(queries, frag.Query)
	}
	queryContext := strings.Join(queries, "\n")

	// Retrieve relevant performance documentation
	ragResp, err := a.retriever.RetrieveForCategory(ctx, "PostgreSQL query optimization indexing performance "+queryContext, "postgres_performance", 5)
	if err != nil {
		// Continue without RAG context
		ragResp = &rag.RetrievalResponse{}
	}

	// Build prompt
	promptData := performancePromptData{
		RAGContext:   ragResp.FormattedContext,
		SQLFragments: formatFragmentsForPrompt(input.SQLFragments),
		FileContent:  truncateContent(input.FileContent, 2000),
		FilePath:     input.FilePath,
	}

	prompt, err := renderTemplate(performanceUserPromptTmpl, promptData)
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	// Call LLM
	response, err := a.llm.Complete(ctx, PerformanceSystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	// Parse response
	result := &PerformanceResult{}
	if err := parseJSONResponse(response, result); err != nil {
		// Try to extract partial results
		result.Issues = extractIssuesFromText(response, "performance")
	}

	return result, nil
}

// AnalyzeSecurity analyzes SQL for security vulnerabilities.
func (a *Analyzer) AnalyzeSecurity(ctx context.Context, input AnalysisInput) (*SecurityResult, error) {
	if len(input.SQLFragments) == 0 && input.FileContent == "" {
		return &SecurityResult{}, nil
	}

	// Build query for RAG retrieval
	queryContext := "SQL injection security permissions authentication"
	for _, frag := range input.SQLFragments {
		if frag.IsDynamic {
			queryContext += " dynamic SQL string concatenation"
			break
		}
	}

	// Retrieve relevant security documentation
	ragResp, err := a.retriever.RetrieveForCategory(ctx, queryContext, "postgres_security", 5)
	if err != nil {
		ragResp = &rag.RetrievalResponse{}
	}

	// Build prompt
	promptData := securityPromptData{
		RAGContext:   ragResp.FormattedContext,
		SQLFragments: formatFragmentsForPrompt(input.SQLFragments),
		FileContent:  truncateContent(input.FileContent, 2000),
		FilePath:     input.FilePath,
		Language:     input.Language,
	}

	prompt, err := renderTemplate(securityUserPromptTmpl, promptData)
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	// Call LLM
	response, err := a.llm.Complete(ctx, SecuritySystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	// Parse response
	result := &SecurityResult{}
	if err := parseJSONResponse(response, result); err != nil {
		result.Vulnerabilities = extractIssuesFromText(response, "security")
	}

	return result, nil
}

// AnalyzeStandards analyzes SQL for standards compliance.
func (a *Analyzer) AnalyzeStandards(ctx context.Context, input AnalysisInput) (*StandardsResult, error) {
	if len(input.SQLFragments) == 0 && input.FileContent == "" {
		return &StandardsResult{}, nil
	}

	// Retrieve relevant standards documentation
	ragResp, err := a.retriever.RetrieveForCategory(ctx, "PostgreSQL naming conventions schema design best practices migration", "postgres_standards", 5)
	if err != nil {
		ragResp = &rag.RetrievalResponse{}
	}

	// Build prompt
	promptData := standardsPromptData{
		RAGContext:   ragResp.FormattedContext,
		OrgStandards: input.OrgStandards,
		SQLFragments: formatFragmentsForPrompt(input.SQLFragments),
		FileContent:  truncateContent(input.FileContent, 2000),
		FilePath:     input.FilePath,
	}

	prompt, err := renderTemplate(standardsUserPromptTmpl, promptData)
	if err != nil {
		return nil, fmt.Errorf("render prompt: %w", err)
	}

	// Call LLM
	response, err := a.llm.Complete(ctx, StandardsSystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	// Parse response
	result := &StandardsResult{}
	if err := parseJSONResponse(response, result); err != nil {
		result.Violations = extractIssuesFromText(response, "standards")
	}

	return result, nil
}

// AnalyzeAll runs all three analysis pipelines in parallel conceptually.
func (a *Analyzer) AnalyzeAll(ctx context.Context, input AnalysisInput) (*CombinedResult, error) {
	// Extract SQL if not provided
	if len(input.SQLFragments) == 0 && input.FileContent != "" {
		input.SQLFragments = a.extractor.Extract(input.FileContent, input.Language, input.FilePath)
	}

	result := &CombinedResult{}

	// Run all analyses
	if perfResult, err := a.AnalyzePerformance(ctx, input); err == nil {
		result.Performance = perfResult
	}

	if secResult, err := a.AnalyzeSecurity(ctx, input); err == nil {
		result.Security = secResult
	}

	if stdResult, err := a.AnalyzeStandards(ctx, input); err == nil {
		result.Standards = stdResult
	}

	// Aggregate issues
	result.AllIssues = a.aggregateIssues(result)

	return result, nil
}

// CombinedResult contains all analysis results.
type CombinedResult struct {
	Performance *PerformanceResult `json:"performance,omitempty"`
	Security    *SecurityResult    `json:"security,omitempty"`
	Standards   *StandardsResult   `json:"standards,omitempty"`
	AllIssues   []Issue            `json:"all_issues"`
}

func (a *Analyzer) aggregateIssues(result *CombinedResult) []Issue {
	var all []Issue

	if result.Performance != nil {
		all = append(all, result.Performance.Issues...)
	}
	if result.Security != nil {
		all = append(all, result.Security.Vulnerabilities...)
	}
	if result.Standards != nil {
		all = append(all, result.Standards.Violations...)
	}

	// Sort by severity
	severityOrder := map[string]int{
		"CRITICAL": 0,
		"HIGH":     1,
		"MEDIUM":   2,
		"LOW":      3,
		"INFO":     4,
		"WARNING":  3,
		"ERROR":    1,
	}

	for i := 0; i < len(all)-1; i++ {
		for j := i + 1; j < len(all); j++ {
			si := severityOrder[strings.ToUpper(all[i].Severity)]
			sj := severityOrder[strings.ToUpper(all[j].Severity)]
			if sj < si {
				all[i], all[j] = all[j], all[i]
			}
		}
	}

	return all
}

// Prompt data structures
type performancePromptData struct {
	RAGContext   string
	SQLFragments string
	FileContent  string
	FilePath     string
}

type securityPromptData struct {
	RAGContext   string
	SQLFragments string
	FileContent  string
	FilePath     string
	Language     string
}

type standardsPromptData struct {
	RAGContext   string
	OrgStandards string
	SQLFragments string
	FileContent  string
	FilePath     string
}

// Helper functions

func formatFragmentsForPrompt(fragments []SQLFragment) string {
	var sb strings.Builder
	for i, f := range fragments {
		sb.WriteString(fmt.Sprintf("--- Query %d (Line %d) ---\n", i+1, f.Line))
		sb.WriteString(f.Query)
		sb.WriteString("\n")
		if f.IsDynamic {
			sb.WriteString("[WARNING: This appears to be dynamic SQL]\n")
		}
		if len(f.Tables) > 0 {
			sb.WriteString(fmt.Sprintf("Tables: %s\n", strings.Join(f.Tables, ", ")))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func truncateContent(content string, maxChars int) string {
	if len(content) <= maxChars {
		return content
	}
	return content[:maxChars] + "\n... [truncated]"
}

func renderTemplate(tmplStr string, data interface{}) (string, error) {
	tmpl, err := template.New("prompt").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func parseJSONResponse(response string, result interface{}) error {
	// Find JSON in response
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || end <= start {
		return fmt.Errorf("no JSON found in response")
	}

	jsonStr := response[start : end+1]

	// Clean up common LLM JSON issues
	jsonStr = strings.ReplaceAll(jsonStr, "'", "\"")

	return json.Unmarshal([]byte(jsonStr), result)
}

func extractIssuesFromText(text, category string) []Issue {
	// Simple fallback parser for when JSON parsing fails
	var issues []Issue

	lines := strings.Split(text, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), "issue") ||
		   strings.Contains(strings.ToLower(line), "problem") ||
		   strings.Contains(strings.ToLower(line), "vulnerability") {
			issues = append(issues, Issue{
				Severity:    "MEDIUM",
				Category:    category,
				Description: line,
				Problem:     line,
			})
		}
	}

	return issues
}

// Prompt templates

const performanceUserPromptTmpl = `Analyze the following SQL queries for performance issues.

FILE: {{.FilePath}}

SQL QUERIES FOUND:
{{.SQLFragments}}

{{if .FileContent}}
FULL FILE CONTEXT (truncated):
{{.FileContent}}
{{end}}

Please analyze these queries for performance issues and respond with a JSON object containing:
- issues: array of performance issues found
- indexes_to_create: suggested indexes
- queries_to_rewrite: suggested query optimizations

Focus on:
1. Missing indexes (columns in WHERE, JOIN, ORDER BY)
2. Full table scans
3. N+1 query patterns
4. Expensive operations
5. Suboptimal query patterns`

const securityUserPromptTmpl = `Analyze the following code for SQL security vulnerabilities.

FILE: {{.FilePath}}
LANGUAGE: {{.Language}}

SQL QUERIES FOUND:
{{.SQLFragments}}

{{if .FileContent}}
FULL FILE CONTEXT (truncated):
{{.FileContent}}
{{end}}

Please analyze for security vulnerabilities and respond with a JSON object containing:
- vulnerabilities: array of security issues found

Focus on:
1. SQL injection vulnerabilities
2. Hardcoded credentials
3. Overly permissive grants
4. Missing parameterization
5. Dynamic SQL construction`

const standardsUserPromptTmpl = `Analyze the following SQL for standards compliance.

FILE: {{.FilePath}}

{{if .OrgStandards}}
ORGANIZATION STANDARDS:
{{.OrgStandards}}
{{end}}

SQL QUERIES FOUND:
{{.SQLFragments}}

{{if .FileContent}}
FULL FILE CONTEXT (truncated):
{{.FileContent}}
{{end}}

Please analyze for standards violations and respond with a JSON object containing:
- violations: array of standards violations
- migration_safety: object with is_safe, blocking_operations, recommendations

Focus on:
1. Naming conventions
2. Schema design best practices
3. Migration safety
4. Anti-patterns`
