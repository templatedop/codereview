package workflow

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/yourorg/code-reviewer/internal/postgres"
)

// PostgresReviewInput contains input for the PostgreSQL review workflow.
type PostgresReviewInput struct {
	// ProjectID is the GitLab/GitHub project ID.
	ProjectID int64 `json:"project_id"`
	// MergeRequestID is the merge request ID.
	MergeRequestID int64 `json:"merge_request_id"`
	// Files contains the files to analyze.
	Files []FileInfo `json:"files"`
	// Options contains analysis options.
	Options PostgresReviewOptions `json:"options"`
}

// FileInfo contains information about a file to analyze.
type FileInfo struct {
	Path        string `json:"path"`
	Content     string `json:"content"`
	Diff        string `json:"diff"`
	Language    string `json:"language"`
	IsNew       bool   `json:"is_new"`
	IsDeleted   bool   `json:"is_deleted"`
	IsMigration bool   `json:"is_migration"`
}

// PostgresReviewOptions contains options for the review.
type PostgresReviewOptions struct {
	// EnableDBLab enables Database Lab validation.
	EnableDBLab bool `json:"enable_db_lab"`
	// OrgStandards contains organization-specific standards.
	OrgStandards string `json:"org_standards"`
	// MinSeverity is the minimum severity to report.
	MinSeverity string `json:"min_severity"`
	// SkipPerformance skips performance analysis.
	SkipPerformance bool `json:"skip_performance"`
	// SkipSecurity skips security analysis.
	SkipSecurity bool `json:"skip_security"`
	// SkipStandards skips standards analysis.
	SkipStandards bool `json:"skip_standards"`
	// PostComments enables posting comments to GitLab/GitHub.
	PostComments bool `json:"post_comments"`
}

// PostgresReviewResult contains the final review result.
type PostgresReviewResult struct {
	Status           string                    `json:"status"`
	TotalIssues      int                       `json:"total_issues"`
	CriticalCount    int                       `json:"critical_count"`
	HighCount        int                       `json:"high_count"`
	MediumCount      int                       `json:"medium_count"`
	LowCount         int                       `json:"low_count"`
	PerformanceIssues []postgres.Issue         `json:"performance_issues,omitempty"`
	SecurityIssues    []postgres.Issue         `json:"security_issues,omitempty"`
	StandardsIssues   []postgres.Issue         `json:"standards_issues,omitempty"`
	IndexSuggestions  []postgres.IndexSuggestion `json:"index_suggestions,omitempty"`
	QueryRewrites     []postgres.QueryRewrite  `json:"query_rewrites,omitempty"`
	MigrationSafety   *postgres.MigrationSafety `json:"migration_safety,omitempty"`
	FilesAnalyzed     int                       `json:"files_analyzed"`
	SQLQueriesFound   int                       `json:"sql_queries_found"`
	ProcessingTime    time.Duration             `json:"processing_time"`
}

// PostgresReviewWorkflow is the main Temporal workflow for PostgreSQL code review.
func PostgresReviewWorkflow(ctx workflow.Context, input PostgresReviewInput) (*PostgresReviewResult, error) {
	logger := workflow.GetLogger(ctx)
	startTime := workflow.Now(ctx)

	// Configure activity options
	activityOpts := workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOpts)

	// Step 1: Extract SQL from all files
	logger.Info("Extracting SQL from files", "file_count", len(input.Files))
	var sqlFragments []postgres.SQLFragment
	err := workflow.ExecuteActivity(ctx, "ExtractSQLActivity", ExtractSQLInput{
		Files: input.Files,
	}).Get(ctx, &sqlFragments)
	if err != nil {
		logger.Error("Failed to extract SQL", "error", err)
		return nil, err
	}

	logger.Info("SQL extraction complete", "fragment_count", len(sqlFragments))

	if len(sqlFragments) == 0 && !hasMigrationFiles(input.Files) {
		logger.Info("No SQL found and no migration files")
		return &PostgresReviewResult{
			Status:         "NO_SQL_FOUND",
			FilesAnalyzed:  len(input.Files),
			ProcessingTime: workflow.Now(ctx).Sub(startTime),
		}, nil
	}

	// Step 2: Run parallel analysis pipelines
	var performanceResult *postgres.PerformanceResult
	var securityResult *postgres.SecurityResult
	var standardsResult *postgres.StandardsResult

	// Create analysis input
	analysisInput := AnalysisActivityInput{
		SQLFragments: sqlFragments,
		Files:        input.Files,
		OrgStandards: input.Options.OrgStandards,
	}

	// Launch parallel activities using workflow.Go
	perfDone := workflow.NewChannel(ctx)
	secDone := workflow.NewChannel(ctx)
	stdDone := workflow.NewChannel(ctx)

	// Performance analysis
	if !input.Options.SkipPerformance {
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := workflow.ExecuteActivity(ctx, "PerformanceAnalysisActivity", analysisInput).Get(ctx, &performanceResult)
			if err != nil {
				logger.Warn("Performance analysis failed", "error", err)
			}
			perfDone.Send(ctx, true)
		})
	} else {
		perfDone.Send(ctx, true)
	}

	// Security analysis
	if !input.Options.SkipSecurity {
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := workflow.ExecuteActivity(ctx, "SecurityAnalysisActivity", analysisInput).Get(ctx, &securityResult)
			if err != nil {
				logger.Warn("Security analysis failed", "error", err)
			}
			secDone.Send(ctx, true)
		})
	} else {
		secDone.Send(ctx, true)
	}

	// Standards analysis
	if !input.Options.SkipStandards {
		workflow.Go(ctx, func(ctx workflow.Context) {
			err := workflow.ExecuteActivity(ctx, "StandardsAnalysisActivity", analysisInput).Get(ctx, &standardsResult)
			if err != nil {
				logger.Warn("Standards analysis failed", "error", err)
			}
			stdDone.Send(ctx, true)
		})
	} else {
		stdDone.Send(ctx, true)
	}

	// Wait for all analyses to complete
	var done bool
	perfDone.Receive(ctx, &done)
	secDone.Receive(ctx, &done)
	stdDone.Receive(ctx, &done)

	logger.Info("All analyses complete")

	// Step 3: Optional Database Lab validation
	if input.Options.EnableDBLab && performanceResult != nil && len(performanceResult.Issues) > 0 {
		logger.Info("Running Database Lab validation")
		var dbLabResult *DatabaseLabValidationResult
		err := workflow.ExecuteActivity(ctx, "DatabaseLabValidationActivity", DatabaseLabInput{
			Queries: extractQueries(sqlFragments),
		}).Get(ctx, &dbLabResult)
		if err != nil {
			logger.Warn("Database Lab validation failed", "error", err)
		} else {
			// Enhance performance results with actual EXPLAIN data
			performanceResult = enhanceWithDBLabResults(performanceResult, dbLabResult)
		}
	}

	// Step 4: Aggregate results
	result := aggregatePostgresResults(performanceResult, securityResult, standardsResult)
	result.FilesAnalyzed = len(input.Files)
	result.SQLQueriesFound = len(sqlFragments)
	result.ProcessingTime = workflow.Now(ctx).Sub(startTime)

	// Step 5: Post comments if enabled
	if input.Options.PostComments && result.TotalIssues > 0 {
		logger.Info("Posting review comments", "issue_count", result.TotalIssues)
		err := workflow.ExecuteActivity(ctx, "PostPostgresReviewActivity", PostReviewInput{
			ProjectID:      input.ProjectID,
			MergeRequestID: input.MergeRequestID,
			Result:         result,
			MinSeverity:    input.Options.MinSeverity,
		}).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to post comments", "error", err)
			// Don't fail the workflow, just log
		}
	}

	logger.Info("PostgreSQL review complete",
		"total_issues", result.TotalIssues,
		"critical", result.CriticalCount,
		"high", result.HighCount,
		"processing_time", result.ProcessingTime)

	return result, nil
}

// hasMigrationFiles checks if there are migration files to analyze.
func hasMigrationFiles(files []FileInfo) bool {
	for _, f := range files {
		if f.IsMigration {
			return true
		}
	}
	return false
}

// extractQueries extracts query strings from fragments.
func extractQueries(fragments []postgres.SQLFragment) []string {
	queries := make([]string, len(fragments))
	for i, f := range fragments {
		queries[i] = f.Query
	}
	return queries
}

// enhanceWithDBLabResults enhances performance results with Database Lab validation.
func enhanceWithDBLabResults(perf *postgres.PerformanceResult, dblab *DatabaseLabValidationResult) *postgres.PerformanceResult {
	if dblab == nil || len(dblab.Plans) == 0 {
		return perf
	}

	// Add validated issues based on actual execution plans
	for _, plan := range dblab.Plans {
		if plan.Analysis == nil {
			continue
		}

		// Add issues for sequential scans on large tables
		for _, table := range plan.Analysis.SeqScanTables {
			found := false
			for _, issue := range perf.Issues {
				if issue.Query == plan.Query {
					found = true
					issue.Impact = formatDBLabImpact(plan)
					break
				}
			}
			if !found && plan.Analysis.ActualRows > 10000 {
				perf.Issues = append(perf.Issues, postgres.Issue{
					Severity:  "HIGH",
					Category:  "INDEX",
					Query:     plan.Query,
					Problem:   "Sequential scan detected on table: " + table,
					Impact:    formatDBLabImpact(plan),
					Suggestion: "Consider adding an index to support this query pattern",
				})
			}
		}

		// Add suggested indexes from DB Lab analysis
		for _, warning := range plan.Analysis.Warnings {
			perf.Issues = append(perf.Issues, postgres.Issue{
				Severity:  "MEDIUM",
				Category:  "PERFORMANCE",
				Query:     plan.Query,
				Problem:   warning,
				Impact:    formatDBLabImpact(plan),
			})
		}
	}

	return perf
}

func formatDBLabImpact(plan *postgres.ExplainPlan) string {
	return formatf("Execution time: %.2fms, Rows examined: %d, Shared blocks hit: %d, read: %d",
		plan.ExecutionTime, plan.RowsExamined, plan.SharedHit, plan.SharedRead)
}

// aggregatePostgresResults combines all analysis results.
func aggregatePostgresResults(perf *postgres.PerformanceResult, sec *postgres.SecurityResult, std *postgres.StandardsResult) *PostgresReviewResult {
	result := &PostgresReviewResult{
		Status: "COMPLETED",
	}

	if perf != nil {
		result.PerformanceIssues = perf.Issues
		result.IndexSuggestions = perf.IndexSuggestions
		result.QueryRewrites = perf.QueryRewrites
	}

	if sec != nil {
		result.SecurityIssues = sec.Vulnerabilities
	}

	if std != nil {
		result.StandardsIssues = std.Violations
		result.MigrationSafety = &std.MigrationSafety
	}

	// Count by severity
	allIssues := append(append(result.PerformanceIssues, result.SecurityIssues...), result.StandardsIssues...)
	for _, issue := range allIssues {
		result.TotalIssues++
		switch issue.Severity {
		case "CRITICAL":
			result.CriticalCount++
		case "HIGH", "ERROR":
			result.HighCount++
		case "MEDIUM", "WARNING":
			result.MediumCount++
		case "LOW", "INFO":
			result.LowCount++
		}
	}

	return result
}

// formatf is a simple sprintf helper that doesn't need the fmt import in workflow code.
func formatf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// Re-export task queue constant for PostgreSQL workflows.
const PostgresReviewTaskQueue = "postgres-review-queue"
