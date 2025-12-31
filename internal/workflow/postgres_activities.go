package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/code-reviewer/internal/postgres"
	"github.com/yourorg/code-reviewer/internal/rag"
)

// PostgresActivities contains all PostgreSQL review activities.
type PostgresActivities struct {
	analyzer  *postgres.Analyzer
	extractor *postgres.SQLExtractor
	dbLabClient *postgres.DatabaseLabClient
	retriever *rag.Retriever
}

// NewPostgresActivities creates a new activities instance.
func NewPostgresActivities(
	analyzer *postgres.Analyzer,
	dbLabClient *postgres.DatabaseLabClient,
	retriever *rag.Retriever,
) *PostgresActivities {
	return &PostgresActivities{
		analyzer:    analyzer,
		extractor:   postgres.NewSQLExtractor(),
		dbLabClient: dbLabClient,
		retriever:   retriever,
	}
}

// ExtractSQLInput contains input for SQL extraction.
type ExtractSQLInput struct {
	Files []FileInfo `json:"files"`
}

// ExtractSQLActivity extracts SQL fragments from source files.
func (a *PostgresActivities) ExtractSQLActivity(ctx context.Context, input ExtractSQLInput) ([]postgres.SQLFragment, error) {
	var allFragments []postgres.SQLFragment

	for _, file := range input.Files {
		if file.IsDeleted {
			continue
		}

		// Skip non-code files
		if !isCodeFile(file.Path) && !isSQLFile(file.Path) {
			continue
		}

		// Extract from content or diff
		var fragments []postgres.SQLFragment
		if file.Content != "" {
			fragments = a.extractor.Extract(file.Content, file.Language, file.Path)
		} else if file.Diff != "" {
			fragments = a.extractor.ExtractFromDiff(file.Diff, file.Language, file.Path)
		}

		// Mark migration files
		if file.IsMigration || isMigrationFile(file.Path) {
			for i := range fragments {
				fragments[i].File = file.Path
			}
		}

		allFragments = append(allFragments, fragments...)
	}

	return allFragments, nil
}

// AnalysisActivityInput contains input for analysis activities.
type AnalysisActivityInput struct {
	SQLFragments []postgres.SQLFragment `json:"sql_fragments"`
	Files        []FileInfo             `json:"files"`
	OrgStandards string                 `json:"org_standards"`
}

// PerformanceAnalysisActivity performs performance analysis.
func (a *PostgresActivities) PerformanceAnalysisActivity(ctx context.Context, input AnalysisActivityInput) (*postgres.PerformanceResult, error) {
	if a.analyzer == nil {
		return nil, fmt.Errorf("analyzer not configured")
	}

	analysisInput := postgres.AnalysisInput{
		SQLFragments: input.SQLFragments,
	}

	// Add file contents for context
	for _, file := range input.Files {
		if file.Content != "" && (isCodeFile(file.Path) || isSQLFile(file.Path)) {
			analysisInput.FileContent += fmt.Sprintf("\n--- %s ---\n%s\n", file.Path, file.Content)
			if analysisInput.Language == "" {
				analysisInput.Language = file.Language
			}
		}
	}

	return a.analyzer.AnalyzePerformance(ctx, analysisInput)
}

// SecurityAnalysisActivity performs security analysis.
func (a *PostgresActivities) SecurityAnalysisActivity(ctx context.Context, input AnalysisActivityInput) (*postgres.SecurityResult, error) {
	if a.analyzer == nil {
		return nil, fmt.Errorf("analyzer not configured")
	}

	analysisInput := postgres.AnalysisInput{
		SQLFragments: input.SQLFragments,
	}

	// Add file contents for context
	for _, file := range input.Files {
		if file.Content != "" && (isCodeFile(file.Path) || isSQLFile(file.Path)) {
			analysisInput.FileContent += fmt.Sprintf("\n--- %s ---\n%s\n", file.Path, file.Content)
			if analysisInput.Language == "" {
				analysisInput.Language = file.Language
			}
			analysisInput.FilePath = file.Path
		}
	}

	return a.analyzer.AnalyzeSecurity(ctx, analysisInput)
}

// StandardsAnalysisActivity performs standards analysis.
func (a *PostgresActivities) StandardsAnalysisActivity(ctx context.Context, input AnalysisActivityInput) (*postgres.StandardsResult, error) {
	if a.analyzer == nil {
		return nil, fmt.Errorf("analyzer not configured")
	}

	analysisInput := postgres.AnalysisInput{
		SQLFragments: input.SQLFragments,
		OrgStandards: input.OrgStandards,
	}

	// Add file contents for context
	for _, file := range input.Files {
		if file.Content != "" && (isCodeFile(file.Path) || isSQLFile(file.Path)) {
			analysisInput.FileContent += fmt.Sprintf("\n--- %s ---\n%s\n", file.Path, file.Content)
			if analysisInput.Language == "" {
				analysisInput.Language = file.Language
			}
			analysisInput.FilePath = file.Path
		}
	}

	return a.analyzer.AnalyzeStandards(ctx, analysisInput)
}

// DatabaseLabInput contains input for Database Lab validation.
type DatabaseLabInput struct {
	Queries []string `json:"queries"`
}

// DatabaseLabValidationResult contains Database Lab validation results.
type DatabaseLabValidationResult struct {
	Plans      []*postgres.ExplainPlan `json:"plans"`
	Validated  int                     `json:"validated"`
	Failed     int                     `json:"failed"`
}

// DatabaseLabValidationActivity validates queries using Database Lab.
func (a *PostgresActivities) DatabaseLabValidationActivity(ctx context.Context, input DatabaseLabInput) (*DatabaseLabValidationResult, error) {
	if a.dbLabClient == nil || !a.dbLabClient.IsEnabled() {
		return nil, fmt.Errorf("database lab not configured")
	}

	// Filter to only SELECT queries for EXPLAIN
	var selectQueries []string
	for _, q := range input.Queries {
		upper := strings.ToUpper(strings.TrimSpace(q))
		if strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH") {
			selectQueries = append(selectQueries, q)
		}
	}

	if len(selectQueries) == 0 {
		return &DatabaseLabValidationResult{}, nil
	}

	plans, err := a.dbLabClient.ValidateQueries(ctx, selectQueries)
	if err != nil {
		return nil, fmt.Errorf("validate queries: %w", err)
	}

	result := &DatabaseLabValidationResult{
		Plans:     plans,
		Validated: len(plans),
		Failed:    len(selectQueries) - len(plans),
	}

	return result, nil
}

// PostReviewInput contains input for posting review comments.
type PostReviewInput struct {
	ProjectID      int64                `json:"project_id"`
	MergeRequestID int64                `json:"merge_request_id"`
	Result         *PostgresReviewResult `json:"result"`
	MinSeverity    string               `json:"min_severity"`
}

// PostPostgresReviewActivity posts review comments to GitLab/GitHub.
func (a *PostgresActivities) PostPostgresReviewActivity(ctx context.Context, input PostReviewInput) error {
	// This would integrate with the existing GitLab client
	// For now, just log the results

	// Filter by minimum severity
	minSev := parseSeverity(input.MinSeverity)

	var comments []ReviewComment

	// Add performance issues
	for _, issue := range input.Result.PerformanceIssues {
		if parseSeverity(issue.Severity) >= minSev {
			comments = append(comments, formatIssueComment(issue, "Performance"))
		}
	}

	// Add security issues
	for _, issue := range input.Result.SecurityIssues {
		if parseSeverity(issue.Severity) >= minSev {
			comments = append(comments, formatIssueComment(issue, "Security"))
		}
	}

	// Add standards issues
	for _, issue := range input.Result.StandardsIssues {
		if parseSeverity(issue.Severity) >= minSev {
			comments = append(comments, formatIssueComment(issue, "Standards"))
		}
	}

	// TODO: Integrate with GitLab client to actually post comments
	_ = comments

	return nil
}

// ReviewComment represents a review comment to post.
type ReviewComment struct {
	FilePath string
	Line     int
	Body     string
	Severity string
}

func formatIssueComment(issue postgres.Issue, category string) ReviewComment {
	var body strings.Builder
	body.WriteString(fmt.Sprintf("**[%s] %s Issue**\n\n", issue.Severity, category))

	if issue.Problem != "" {
		body.WriteString(fmt.Sprintf("**Problem:** %s\n\n", issue.Problem))
	}
	if issue.Description != "" {
		body.WriteString(fmt.Sprintf("%s\n\n", issue.Description))
	}
	if issue.Impact != "" {
		body.WriteString(fmt.Sprintf("**Impact:** %s\n\n", issue.Impact))
	}
	if issue.Suggestion != "" {
		body.WriteString(fmt.Sprintf("**Suggestion:** %s\n\n", issue.Suggestion))
	}
	if issue.Reference != "" {
		body.WriteString(fmt.Sprintf("**Reference:** %s\n", issue.Reference))
	}
	if issue.CWE != "" {
		body.WriteString(fmt.Sprintf("**CWE:** %s\n", issue.CWE))
	}

	return ReviewComment{
		Line:     issue.Line,
		Body:     body.String(),
		Severity: issue.Severity,
	}
}

func parseSeverity(s string) int {
	switch strings.ToUpper(s) {
	case "CRITICAL":
		return 4
	case "HIGH", "ERROR":
		return 3
	case "MEDIUM", "WARNING":
		return 2
	case "LOW", "INFO":
		return 1
	default:
		return 0
	}
}

// Helper functions

func isCodeFile(path string) bool {
	extensions := []string{".go", ".py", ".java", ".js", ".ts", ".tsx", ".rb", ".php", ".rs", ".scala", ".kt"}
	lower := strings.ToLower(path)
	for _, ext := range extensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func isSQLFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".sql") || strings.HasSuffix(lower, ".pgsql")
}

func isMigrationFile(path string) bool {
	lower := strings.ToLower(path)
	return strings.Contains(lower, "migration") ||
		strings.Contains(lower, "migrate") ||
		strings.Contains(lower, "/db/") ||
		strings.Contains(lower, "/schema/") ||
		strings.Contains(lower, "/flyway/") ||
		strings.Contains(lower, "/liquibase/")
}

// RAG Activities

// RAGSearchInput contains input for RAG search.
type RAGSearchInput struct {
	Query      string   `json:"query"`
	Categories []string `json:"categories"`
	TopK       int      `json:"top_k"`
}

// RAGSearchResult contains RAG search results.
type RAGSearchResult struct {
	Context   string                  `json:"context"`
	Documents []rag.RetrievedDocument `json:"documents"`
}

// RAGSearchActivity searches the knowledge base.
func (a *PostgresActivities) RAGSearchActivity(ctx context.Context, input RAGSearchInput) (*RAGSearchResult, error) {
	if a.retriever == nil {
		return nil, fmt.Errorf("retriever not configured")
	}

	if input.TopK <= 0 {
		input.TopK = 5
	}

	response, err := a.retriever.RetrieveForPostgres(ctx, input.Query, input.Categories)
	if err != nil {
		return nil, fmt.Errorf("retrieve: %w", err)
	}

	return &RAGSearchResult{
		Context:   response.FormattedContext,
		Documents: response.Documents,
	}, nil
}

// Ingestion Activities

// IngestKnowledgeInput contains input for knowledge ingestion.
type IngestKnowledgeInput struct {
	Sources []postgres.IngestSource `json:"sources"`
}

// IngestKnowledgeResult contains ingestion results.
type IngestKnowledgeResult struct {
	DocumentsIndexed int      `json:"documents_indexed"`
	Errors           []string `json:"errors,omitempty"`
}

// IngestKnowledgeActivity ingests knowledge documents.
func IngestKnowledgeActivity(ctx context.Context, input IngestKnowledgeInput) (*IngestKnowledgeResult, error) {
	// This activity would use the Ingester
	// Implemented in the worker setup
	return &IngestKnowledgeResult{}, nil
}
