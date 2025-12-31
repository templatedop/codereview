package workflow

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	"github.com/yourorg/code-reviewer/internal/reviewer"
)

// CodeReviewInput contains the input for a code review workflow
type CodeReviewInput struct {
	// GitLab/GitHub info
	ProjectID    int    `json:"project_id"`
	MergeReqID   int    `json:"merge_req_id"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`

	// Review options
	FrameworkName string `json:"framework_name,omitempty"`
	UseAgent      bool   `json:"use_agent"`

	// Or direct file review
	FilePath    string `json:"file_path,omitempty"`
	Diff        string `json:"diff,omitempty"`
	FullContent string `json:"full_content,omitempty"`
}

// CodeReviewOutput contains the result of a code review workflow
type CodeReviewOutput struct {
	Result        *reviewer.ReviewResult `json:"result"`
	FilesReviewed int                    `json:"files_reviewed"`
	TotalIssues   int                    `json:"total_issues"`
	Posted        bool                   `json:"posted"`
	Error         string                 `json:"error,omitempty"`
}

// CodeReviewWorkflow orchestrates the code review process
func CodeReviewWorkflow(ctx workflow.Context, input CodeReviewInput) (*CodeReviewOutput, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting code review workflow", "project", input.ProjectID, "mr", input.MergeReqID)

	// Activity options with retries
	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	var output CodeReviewOutput

	// Step 1: Fetch MR diff if not provided directly
	var diffResult FetchDiffResult
	if input.Diff == "" && input.ProjectID > 0 {
		err := workflow.ExecuteActivity(ctx, FetchMRDiffActivity, FetchDiffInput{
			ProjectID:  input.ProjectID,
			MergeReqID: input.MergeReqID,
		}).Get(ctx, &diffResult)
		if err != nil {
			output.Error = err.Error()
			return &output, err
		}
	} else {
		// Use provided diff
		diffResult = FetchDiffResult{
			Files: []FileDiff{{
				Path:    input.FilePath,
				Diff:    input.Diff,
				Content: input.FullContent,
			}},
		}
	}

	logger.Info("Fetched diff", "files", len(diffResult.Files))

	// Step 2: Load framework context if specified
	var frameworkCtx FrameworkContextResult
	if input.FrameworkName != "" {
		err := workflow.ExecuteActivity(ctx, LoadFrameworkActivity, LoadFrameworkInput{
			Name: input.FrameworkName,
		}).Get(ctx, &frameworkCtx)
		if err != nil {
			logger.Warn("Failed to load framework, continuing without it", "error", err)
		}
	}

	// Step 3: Review each file (can be parallelized)
	var allResults []FileReviewResult

	for _, file := range diffResult.Files {
		var result FileReviewResult

		reviewInput := ReviewFileInput{
			FilePath:      file.Path,
			Diff:          file.Diff,
			FullContent:   file.Content,
			FrameworkName: input.FrameworkName,
			UseAgent:      input.UseAgent,
		}

		err := workflow.ExecuteActivity(ctx, ReviewFileActivity, reviewInput).Get(ctx, &result)
		if err != nil {
			result.Error = err.Error()
		}

		allResults = append(allResults, result)
		output.FilesReviewed++
		if result.Result != nil {
			output.TotalIssues += len(result.Result.Issues)
		}
	}

	// Step 4: Aggregate results
	output.Result = aggregateResults(allResults)

	// Step 5: Post comments to GitLab if configured
	if input.ProjectID > 0 && input.MergeReqID > 0 {
		err := workflow.ExecuteActivity(ctx, PostCommentsActivity, PostCommentsInput{
			ProjectID:  input.ProjectID,
			MergeReqID: input.MergeReqID,
			Results:    allResults,
		}).Get(ctx, nil)
		if err != nil {
			logger.Warn("Failed to post comments", "error", err)
		} else {
			output.Posted = true
		}
	}

	logger.Info("Code review completed",
		"files", output.FilesReviewed,
		"issues", output.TotalIssues,
		"posted", output.Posted)

	return &output, nil
}

// aggregateResults combines results from multiple files
func aggregateResults(results []FileReviewResult) *reviewer.ReviewResult {
	aggregate := &reviewer.ReviewResult{
		Issues:         []reviewer.Issue{},
		Recommendation: "APPROVE",
	}

	var totalRisk int
	var summaries []string

	for _, r := range results {
		if r.Result == nil {
			continue
		}

		aggregate.Issues = append(aggregate.Issues, r.Result.Issues...)
		totalRisk += int(r.Result.RiskScore)

		if r.Result.Summary != "" {
			summaries = append(summaries, r.Result.Summary)
		}

		if r.Result.Recommendation == "REQUEST_CHANGES" {
			aggregate.Recommendation = "REQUEST_CHANGES"
		} else if r.Result.Recommendation == "COMMENT" && aggregate.Recommendation == "APPROVE" {
			aggregate.Recommendation = "COMMENT"
		}
	}

	if len(results) > 0 {
		aggregate.RiskScore = reviewer.FlexInt(totalRisk / len(results))
	}

	// Build summary
	if len(summaries) > 0 {
		aggregate.Summary = summaries[0]
		if len(summaries) > 1 {
			aggregate.Summary += " (+ other files reviewed)"
		}
	}

	return aggregate
}
