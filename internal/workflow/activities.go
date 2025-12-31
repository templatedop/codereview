package workflow

import (
	"context"
	"fmt"

	"github.com/yourorg/code-reviewer/internal/agent"
	"github.com/yourorg/code-reviewer/internal/gitlab"
	"github.com/yourorg/code-reviewer/internal/knowledge"
	"github.com/yourorg/code-reviewer/internal/llm"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

// Activities holds the dependencies for workflow activities
type Activities struct {
	GitLabClient *gitlab.Client
	LLMClient    reviewer.LLMClient
	KnowledgeDir string
}

// NewActivities creates a new Activities instance
func NewActivities(gitlabClient *gitlab.Client, llmClient reviewer.LLMClient, knowledgeDir string) *Activities {
	return &Activities{
		GitLabClient: gitlabClient,
		LLMClient:    llmClient,
		KnowledgeDir: knowledgeDir,
	}
}

// --- Fetch Diff Activity ---

type FetchDiffInput struct {
	ProjectID  int
	MergeReqID int
}

type FileDiff struct {
	Path    string
	Diff    string
	Content string
}

type FetchDiffResult struct {
	Files []FileDiff
}

func (a *Activities) FetchMRDiffActivity(ctx context.Context, input FetchDiffInput) (*FetchDiffResult, error) {
	if a.GitLabClient == nil {
		return nil, fmt.Errorf("GitLab client not configured")
	}

	changes, err := a.GitLabClient.GetMergeRequestChanges(ctx, input.ProjectID, input.MergeReqID)
	if err != nil {
		return nil, fmt.Errorf("fetch MR changes: %w", err)
	}

	result := &FetchDiffResult{
		Files: make([]FileDiff, 0, len(changes.Changes)),
	}

	for _, change := range changes.Changes {
		if change.DeletedFile {
			continue // Skip deleted files
		}

		result.Files = append(result.Files, FileDiff{
			Path: change.NewPath,
			Diff: change.Diff,
		})
	}

	return result, nil
}

// --- Load Framework Activity ---

type LoadFrameworkInput struct {
	Name string
}

type FrameworkContextResult struct {
	Name     string
	Loaded   bool
	Elements int
}

func (a *Activities) LoadFrameworkActivity(ctx context.Context, input LoadFrameworkInput) (*FrameworkContextResult, error) {
	store := knowledge.NewStore(a.KnowledgeDir)

	framework, err := store.LoadFramework(input.Name)
	if err != nil {
		return &FrameworkContextResult{Name: input.Name, Loaded: false}, err
	}

	return &FrameworkContextResult{
		Name:     input.Name,
		Loaded:   true,
		Elements: framework.ElementCount,
	}, nil
}

// --- Review File Activity ---

type ReviewFileInput struct {
	FilePath      string
	Diff          string
	FullContent   string
	FrameworkName string
	UseAgent      bool
}

type FileReviewResult struct {
	FilePath string
	Result   *reviewer.ReviewResult
	Error    string
}

func (a *Activities) ReviewFileActivity(ctx context.Context, input ReviewFileInput) (*FileReviewResult, error) {
	result := &FileReviewResult{FilePath: input.FilePath}

	// Load framework store if needed
	var store *knowledge.Store
	if input.FrameworkName != "" {
		store = knowledge.NewStore(a.KnowledgeDir)
		_, err := store.LoadFramework(input.FrameworkName)
		if err != nil {
			// Continue without framework
			store = nil
		}
	}

	if input.UseAgent && a.LLMClient != nil {
		// Use agentic review
		ag := agent.NewAgent(a.LLMClient, store, input.FrameworkName)
		reviewResult, err := ag.Review(ctx, input.FilePath, input.Diff, input.FullContent)
		if err != nil {
			result.Error = err.Error()
			return result, nil
		}
		result.Result = reviewResult
	} else {
		// Use standard review
		analyzer := reviewer.NewAnalyzer(a.LLMClient)

		if store != nil && input.FrameworkName != "" {
			// Framework-aware review
			relevantElements := store.Search(input.Diff, 5)
			var elements []llm.FrameworkElement
			for _, elem := range relevantElements {
				elements = append(elements, llm.FrameworkElement{
					Type:    elem.Type,
					Package: elem.Package,
					Name:    elem.Name,
					Doc:     elem.Doc,
					Body:    elem.Body,
				})
			}

			frameworkCtx := reviewer.FrameworkContext{
				Name:     input.FrameworkName,
				Elements: elements,
			}

			reviewResult, err := analyzer.ReviewDiffWithFramework(ctx, input.FilePath, input.Diff, input.FullContent, frameworkCtx)
			if err != nil {
				result.Error = err.Error()
				return result, nil
			}
			result.Result = reviewResult
		} else {
			// Standard review
			reviewResult, err := analyzer.ReviewDiff(ctx, input.FilePath, input.Diff, input.FullContent)
			if err != nil {
				result.Error = err.Error()
				return result, nil
			}
			result.Result = reviewResult
		}
	}

	return result, nil
}

// --- Post Comments Activity ---

type PostCommentsInput struct {
	ProjectID  int
	MergeReqID int
	Results    []FileReviewResult
}

func (a *Activities) PostCommentsActivity(ctx context.Context, input PostCommentsInput) error {
	if a.GitLabClient == nil {
		return fmt.Errorf("GitLab client not configured")
	}

	// Build summary comment
	var totalIssues int
	var criticalCount, highCount, mediumCount, lowCount int

	for _, r := range input.Results {
		if r.Result == nil {
			continue
		}
		for _, issue := range r.Result.Issues {
			totalIssues++
			switch issue.Severity {
			case "CRITICAL":
				criticalCount++
			case "HIGH":
				highCount++
			case "MEDIUM":
				mediumCount++
			case "LOW":
				lowCount++
			}
		}
	}

	// Post summary comment
	summary := fmt.Sprintf(`## 🤖 Automated Code Review

**Files Reviewed:** %d
**Issues Found:** %d

| Severity | Count |
|----------|-------|
| 🔴 Critical | %d |
| 🟠 High | %d |
| 🟡 Medium | %d |
| 🟢 Low | %d |

`, len(input.Results), totalIssues, criticalCount, highCount, mediumCount, lowCount)

	// Add issues details
	if totalIssues > 0 {
		summary += "### Issues\n\n"
		for _, r := range input.Results {
			if r.Result == nil || len(r.Result.Issues) == 0 {
				continue
			}
			summary += fmt.Sprintf("**%s**\n", r.FilePath)
			for _, issue := range r.Result.Issues {
				emoji := "ℹ️"
				switch issue.Severity {
				case "CRITICAL":
					emoji = "🔴"
				case "HIGH":
					emoji = "🟠"
				case "MEDIUM":
					emoji = "🟡"
				case "LOW":
					emoji = "🟢"
				}
				summary += fmt.Sprintf("- %s **[%s]** %s (line %d)\n  - %s\n",
					emoji, issue.Type, issue.Title, issue.Line, issue.Description)
				if issue.Suggestion != "" {
					summary += fmt.Sprintf("  - 💡 %s\n", issue.Suggestion)
				}
			}
			summary += "\n"
		}
	}

	summary += "\n---\n*Automated review by Code Review Bot*"

	_, err := a.GitLabClient.CreateMRNote(ctx, input.ProjectID, input.MergeReqID, summary)
	return err
}

// Activity function references (for workflow)
var (
	FetchMRDiffActivity   = (*Activities).FetchMRDiffActivity
	LoadFrameworkActivity = (*Activities).LoadFrameworkActivity
	ReviewFileActivity    = (*Activities).ReviewFileActivity
	PostCommentsActivity  = (*Activities).PostCommentsActivity
)
