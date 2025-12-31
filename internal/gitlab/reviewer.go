package gitlab

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/yourorg/code-reviewer/internal/logging"
	"github.com/yourorg/code-reviewer/internal/postgres"
)

// ReviewerConfig holds configuration for the GitLab reviewer.
type ReviewerConfig struct {
	// Enabled enables GitLab integration
	Enabled bool
	// MinSeverity is the minimum severity to post (critical, high, medium, low)
	MinSeverity string
	// PostSummary posts a summary comment
	PostSummary bool
	// PostInlineComments posts inline comments on specific lines
	PostInlineComments bool
	// DryRun logs comments instead of posting
	DryRun bool
	// CollapseThreshold collapses issues if count exceeds this
	CollapseThreshold int
	// Logger for logging
	Logger *logging.Logger
}

// DefaultReviewerConfig returns default reviewer configuration.
func DefaultReviewerConfig() ReviewerConfig {
	return ReviewerConfig{
		Enabled:            true,
		MinSeverity:        "low",
		PostSummary:        true,
		PostInlineComments: true,
		DryRun:             false,
		CollapseThreshold:  20,
		Logger:             logging.Default(),
	}
}

// Reviewer posts code review comments to GitLab.
type Reviewer struct {
	client *Client
	config ReviewerConfig
}

// NewReviewer creates a new GitLab reviewer.
func NewReviewer(client *Client, config ReviewerConfig) *Reviewer {
	if config.Logger == nil {
		config.Logger = logging.Default()
	}
	return &Reviewer{
		client: client,
		config: config,
	}
}

// ReviewInput contains input for posting a review.
type ReviewInput struct {
	ProjectID int
	MRIID     int
	BaseSHA   string
	HeadSHA   string
	StartSHA  string

	PerformanceIssues []postgres.Issue
	SecurityIssues    []postgres.Issue
	StandardsIssues   []postgres.Issue

	// File path mappings for inline comments
	FilePaths map[string]string
}

// ReviewResult contains the result of posting a review.
type ReviewResult struct {
	SummaryPosted    bool
	InlineComments   int
	SkippedComments  int
	Errors           []error
}

// PostReview posts the code review to GitLab.
func (r *Reviewer) PostReview(ctx context.Context, input ReviewInput) (*ReviewResult, error) {
	if !r.config.Enabled {
		return &ReviewResult{}, nil
	}

	result := &ReviewResult{}
	minSev := parseSeverity(r.config.MinSeverity)

	// Filter issues by severity
	var allIssues []categorizedIssue
	for _, issue := range input.PerformanceIssues {
		if parseSeverity(issue.Severity) >= minSev {
			allIssues = append(allIssues, categorizedIssue{Issue: issue, Category: "Performance"})
		}
	}
	for _, issue := range input.SecurityIssues {
		if parseSeverity(issue.Severity) >= minSev {
			allIssues = append(allIssues, categorizedIssue{Issue: issue, Category: "Security"})
		}
	}
	for _, issue := range input.StandardsIssues {
		if parseSeverity(issue.Severity) >= minSev {
			allIssues = append(allIssues, categorizedIssue{Issue: issue, Category: "Standards"})
		}
	}

	if len(allIssues) == 0 {
		r.config.Logger.Info("no issues to post", "project_id", input.ProjectID, "mr_iid", input.MRIID)
		return result, nil
	}

	// Post summary comment
	if r.config.PostSummary {
		summary := r.formatSummary(allIssues)
		if r.config.DryRun {
			r.config.Logger.Info("dry run: would post summary",
				"project_id", input.ProjectID,
				"mr_iid", input.MRIID,
				"summary_length", len(summary),
			)
		} else {
			_, err := r.client.CreateMRNote(ctx, input.ProjectID, input.MRIID, summary)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("post summary: %w", err))
				r.config.Logger.Error("failed to post summary", "error", err.Error())
			} else {
				result.SummaryPosted = true
				r.config.Logger.Info("posted summary comment",
					"project_id", input.ProjectID,
					"mr_iid", input.MRIID,
				)
			}
		}
	}

	// Post inline comments
	if r.config.PostInlineComments && len(allIssues) <= r.config.CollapseThreshold {
		for _, issue := range allIssues {
			if issue.Line <= 0 {
				result.SkippedComments++
				continue
			}

			filePath := issue.File
			if filePath == "" {
				result.SkippedComments++
				continue
			}

			comment := r.formatInlineComment(issue)
			position := &DiscussionPosition{
				BaseSHA:      input.BaseSHA,
				StartSHA:     input.StartSHA,
				HeadSHA:      input.HeadSHA,
				PositionType: "text",
				NewPath:      filePath,
				NewLine:      issue.Line,
			}

			if r.config.DryRun {
				r.config.Logger.Debug("dry run: would post inline comment",
					"file", filePath,
					"line", issue.Line,
					"severity", issue.Severity,
				)
			} else {
				_, err := r.client.CreateDiscussion(ctx, input.ProjectID, input.MRIID, comment, position)
				if err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("post inline comment at %s:%d: %w", filePath, issue.Line, err))
					r.config.Logger.Warn("failed to post inline comment",
						"file", filePath,
						"line", issue.Line,
						"error", err.Error(),
					)
				} else {
					result.InlineComments++
				}
			}
		}

		r.config.Logger.Info("posted inline comments",
			"project_id", input.ProjectID,
			"mr_iid", input.MRIID,
			"posted", result.InlineComments,
			"skipped", result.SkippedComments,
		)
	}

	return result, nil
}

type categorizedIssue struct {
	postgres.Issue
	Category string
}

func (r *Reviewer) formatSummary(issues []categorizedIssue) string {
	var sb strings.Builder

	sb.WriteString("## 🔍 Code Review Analysis\n\n")
	sb.WriteString(fmt.Sprintf("*Automated review completed at %s*\n\n", time.Now().Format(time.RFC3339)))

	// Count by category and severity
	byCat := make(map[string]int)
	bySev := make(map[string]int)
	for _, issue := range issues {
		byCat[issue.Category]++
		bySev[issue.Severity]++
	}

	// Summary table
	sb.WriteString("### Summary\n\n")
	sb.WriteString("| Category | Count |\n")
	sb.WriteString("|----------|-------|\n")
	for _, cat := range []string{"Performance", "Security", "Standards"} {
		if count := byCat[cat]; count > 0 {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", cat, count))
		}
	}
	sb.WriteString("\n")

	// Severity breakdown
	sb.WriteString("| Severity | Count |\n")
	sb.WriteString("|----------|-------|\n")
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW"} {
		if count := bySev[sev]; count > 0 {
			icon := severityIcon(sev)
			sb.WriteString(fmt.Sprintf("| %s %s | %d |\n", icon, sev, count))
		}
	}
	sb.WriteString("\n")

	// Details
	sb.WriteString("### Details\n\n")

	// Group by category
	for _, cat := range []string{"Security", "Performance", "Standards"} {
		var catIssues []categorizedIssue
		for _, issue := range issues {
			if issue.Category == cat {
				catIssues = append(catIssues, issue)
			}
		}
		if len(catIssues) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("#### %s Issues\n\n", cat))
		for _, issue := range catIssues {
			icon := severityIcon(issue.Severity)
			sb.WriteString(fmt.Sprintf("<details>\n<summary>%s <b>%s</b>", icon, issue.Severity))
			if issue.File != "" && issue.Line > 0 {
				sb.WriteString(fmt.Sprintf(" - %s:%d", issue.File, issue.Line))
			}
			sb.WriteString("</summary>\n\n")

			if issue.Problem != "" {
				sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", issue.Problem))
			}
			if issue.Description != "" {
				sb.WriteString(fmt.Sprintf("%s\n\n", issue.Description))
			}
			if issue.Impact != "" {
				sb.WriteString(fmt.Sprintf("**Impact:** %s\n\n", issue.Impact))
			}
			if issue.Suggestion != "" {
				sb.WriteString(fmt.Sprintf("**Suggestion:** %s\n\n", issue.Suggestion))
			}
			if issue.Reference != "" {
				sb.WriteString(fmt.Sprintf("**Reference:** %s\n\n", issue.Reference))
			}
			if issue.CWE != "" {
				sb.WriteString(fmt.Sprintf("**CWE:** %s\n\n", issue.CWE))
			}

			sb.WriteString("</details>\n\n")
		}
	}

	sb.WriteString("---\n")
	sb.WriteString("*This review was generated automatically by Code Reviewer*\n")

	return sb.String()
}

func (r *Reviewer) formatInlineComment(issue categorizedIssue) string {
	var sb strings.Builder

	icon := severityIcon(issue.Severity)
	sb.WriteString(fmt.Sprintf("%s **[%s] %s Issue**\n\n", icon, issue.Severity, issue.Category))

	if issue.Problem != "" {
		sb.WriteString(fmt.Sprintf("**Problem:** %s\n\n", issue.Problem))
	}
	if issue.Description != "" {
		sb.WriteString(fmt.Sprintf("%s\n\n", issue.Description))
	}
	if issue.Impact != "" {
		sb.WriteString(fmt.Sprintf("**Impact:** %s\n\n", issue.Impact))
	}
	if issue.Suggestion != "" {
		sb.WriteString(fmt.Sprintf("💡 **Suggestion:** %s\n\n", issue.Suggestion))
	}
	if issue.Reference != "" {
		sb.WriteString(fmt.Sprintf("📚 **Reference:** %s\n", issue.Reference))
	}
	if issue.CWE != "" {
		sb.WriteString(fmt.Sprintf("🔒 **CWE:** %s\n", issue.CWE))
	}

	return sb.String()
}

func severityIcon(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "🔴"
	case "HIGH":
		return "🟠"
	case "MEDIUM":
		return "🟡"
	case "LOW":
		return "🔵"
	default:
		return "⚪"
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
