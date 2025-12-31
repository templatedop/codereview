package gitlab

import (
	"context"
	"fmt"
	"strings"

	"github.com/yourorg/code-reviewer/internal/logger"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

// ReviewPoster handles posting code reviews to GitLab
type ReviewPoster struct {
	client       *Client
	postInline   bool // Whether to post inline comments on specific lines
	summaryOnly  bool // Whether to only post summary
	log          *logger.Logger
}

// ReviewPosterConfig holds configuration for the review poster
type ReviewPosterConfig struct {
	PostInlineComments bool // Post comments on specific lines
	SummaryOnly        bool // Only post summary, not individual issues
	Logger             *logger.Logger
}

// NewReviewPoster creates a new review poster
func NewReviewPoster(client *Client, cfg ReviewPosterConfig) *ReviewPoster {
	log := cfg.Logger
	if log == nil {
		log, _ = logger.New(logger.DefaultConfig())
	}
	return &ReviewPoster{
		client:      client,
		postInline:  cfg.PostInlineComments,
		summaryOnly: cfg.SummaryOnly,
		log:         log,
	}
}

// FileReview represents a review for a single file
type FileReview struct {
	FilePath string
	Result   *reviewer.ReviewResult
	Error    string
}

// PostReviews posts all reviews for a merge request
func (p *ReviewPoster) PostReviews(ctx context.Context, projectID, mrIID int, reviews []FileReview) error {
	if p.client == nil {
		return fmt.Errorf("GitLab client not configured")
	}

	p.log.Info("Posting reviews to GitLab project=%d mr=%d files=%d", projectID, mrIID, len(reviews))

	// Calculate statistics
	stats := p.calculateStats(reviews)

	// Post individual inline comments if enabled
	if p.postInline && !p.summaryOnly {
		if err := p.postInlineComments(ctx, projectID, mrIID, reviews); err != nil {
			p.log.Warn("Failed to post inline comments: %v", err)
			// Continue to post summary anyway
		}
	}

	// Post summary comment
	summary := p.buildSummary(reviews, stats)
	if err := p.client.PostReviewSummary(ctx, projectID, mrIID, summary); err != nil {
		return fmt.Errorf("post summary: %w", err)
	}

	p.log.Info("Reviews posted successfully issues=%d", stats.TotalIssues)
	return nil
}

// ReviewStats holds aggregate statistics
type ReviewStats struct {
	TotalFiles     int
	FilesReviewed  int
	FilesWithError int
	TotalIssues    int
	CriticalCount  int
	HighCount      int
	MediumCount    int
	LowCount       int
	SecurityIssues int
	PerfIssues     int
}

func (p *ReviewPoster) calculateStats(reviews []FileReview) ReviewStats {
	stats := ReviewStats{
		TotalFiles: len(reviews),
	}

	for _, r := range reviews {
		if r.Error != "" {
			stats.FilesWithError++
			continue
		}
		if r.Result == nil {
			continue
		}

		stats.FilesReviewed++
		for _, issue := range r.Result.Issues {
			stats.TotalIssues++
			switch strings.ToUpper(issue.Severity) {
			case "CRITICAL":
				stats.CriticalCount++
			case "HIGH":
				stats.HighCount++
			case "MEDIUM":
				stats.MediumCount++
			case "LOW":
				stats.LowCount++
			}
			switch strings.ToLower(issue.Type) {
			case "security":
				stats.SecurityIssues++
			case "performance":
				stats.PerfIssues++
			}
		}
	}

	return stats
}

func (p *ReviewPoster) postInlineComments(ctx context.Context, projectID, mrIID int, reviews []FileReview) error {
	var errors []string

	for _, r := range reviews {
		if r.Result == nil || len(r.Result.Issues) == 0 {
			continue
		}

		for _, issue := range r.Result.Issues {
			if issue.Line <= 0 {
				continue // Can't post without line number
			}

			comment := p.formatInlineComment(issue)
			_, err := p.client.CreateLineComment(ctx, projectID, mrIID, r.FilePath, issue.Line, comment)
			if err != nil {
				p.log.Debug("Failed to post inline comment: %v", err)
				errors = append(errors, fmt.Sprintf("%s:%d - %v", r.FilePath, issue.Line, err))
			} else {
				p.log.Debug("Posted inline comment: %s:%d", r.FilePath, issue.Line)
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("some inline comments failed: %v", errors)
	}
	return nil
}

func (p *ReviewPoster) formatInlineComment(issue reviewer.Issue) string {
	emoji := p.severityEmoji(issue.Severity)
	comment := fmt.Sprintf("%s **[%s] %s**\n\n%s",
		emoji,
		strings.ToUpper(issue.Type),
		issue.Title,
		issue.Description,
	)

	if issue.Suggestion != "" {
		comment += fmt.Sprintf("\n\n💡 **Suggestion:** %s", issue.Suggestion)
	}

	return comment
}

func (p *ReviewPoster) severityEmoji(severity string) string {
	switch strings.ToUpper(severity) {
	case "CRITICAL":
		return "🔴"
	case "HIGH":
		return "🟠"
	case "MEDIUM":
		return "🟡"
	case "LOW":
		return "🟢"
	default:
		return "ℹ️"
	}
}

func (p *ReviewPoster) buildSummary(reviews []FileReview, stats ReviewStats) string {
	var sb strings.Builder

	sb.WriteString("## 🤖 Automated Code Review\n\n")

	// Overview
	sb.WriteString(fmt.Sprintf("**Files Reviewed:** %d / %d\n", stats.FilesReviewed, stats.TotalFiles))
	if stats.FilesWithError > 0 {
		sb.WriteString(fmt.Sprintf("**Files with Errors:** %d\n", stats.FilesWithError))
	}
	sb.WriteString(fmt.Sprintf("**Issues Found:** %d\n\n", stats.TotalIssues))

	// Severity table
	sb.WriteString("### Issues by Severity\n\n")
	sb.WriteString("| Severity | Count |\n")
	sb.WriteString("|----------|-------|\n")
	sb.WriteString(fmt.Sprintf("| 🔴 Critical | %d |\n", stats.CriticalCount))
	sb.WriteString(fmt.Sprintf("| 🟠 High | %d |\n", stats.HighCount))
	sb.WriteString(fmt.Sprintf("| 🟡 Medium | %d |\n", stats.MediumCount))
	sb.WriteString(fmt.Sprintf("| 🟢 Low | %d |\n\n", stats.LowCount))

	// Issue categories
	if stats.SecurityIssues > 0 || stats.PerfIssues > 0 {
		sb.WriteString("### Issue Categories\n\n")
		if stats.SecurityIssues > 0 {
			sb.WriteString(fmt.Sprintf("- 🔒 Security Issues: %d\n", stats.SecurityIssues))
		}
		if stats.PerfIssues > 0 {
			sb.WriteString(fmt.Sprintf("- ⚡ Performance Issues: %d\n", stats.PerfIssues))
		}
		sb.WriteString("\n")
	}

	// Detailed issues (if not summary only)
	if !p.summaryOnly && stats.TotalIssues > 0 {
		sb.WriteString("### Detailed Findings\n\n")
		for _, r := range reviews {
			if r.Result == nil || len(r.Result.Issues) == 0 {
				continue
			}

			sb.WriteString(fmt.Sprintf("#### 📄 `%s`\n\n", r.FilePath))
			for _, issue := range r.Result.Issues {
				emoji := p.severityEmoji(issue.Severity)
				sb.WriteString(fmt.Sprintf("- %s **[%s]** %s (line %d)\n",
					emoji, issue.Type, issue.Title, issue.Line))
				if issue.Description != "" {
					sb.WriteString(fmt.Sprintf("  - %s\n", issue.Description))
				}
				if issue.Suggestion != "" {
					sb.WriteString(fmt.Sprintf("  - 💡 %s\n", issue.Suggestion))
				}
			}
			sb.WriteString("\n")
		}
	}

	// Footer
	sb.WriteString("---\n")
	sb.WriteString("*Automated review by Code Review Bot*\n")

	return sb.String()
}

// PostSingleReview posts a review for a single file
func (p *ReviewPoster) PostSingleReview(ctx context.Context, projectID, mrIID int, review FileReview) error {
	return p.PostReviews(ctx, projectID, mrIID, []FileReview{review})
}
