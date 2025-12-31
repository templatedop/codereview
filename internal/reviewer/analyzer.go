package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/yourorg/code-reviewer/internal/llm"
)

// FlexInt handles JSON numbers that might come as string or int
type FlexInt int

func (fi *FlexInt) UnmarshalJSON(b []byte) error {
	// Try int first
	var i int
	if err := json.Unmarshal(b, &i); err == nil {
		*fi = FlexInt(i)
		return nil
	}

	// Try string
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		// Remove any non-numeric characters
		s = strings.TrimSpace(s)
		if s == "" {
			*fi = 0
			return nil
		}
		i, err := strconv.Atoi(s)
		if err != nil {
			// Try to extract first number
			for _, c := range s {
				if c >= '0' && c <= '9' {
					i = i*10 + int(c-'0')
				} else if i > 0 {
					break
				}
			}
		}
		*fi = FlexInt(i)
		return nil
	}

	*fi = 0
	return nil
}

// LLMClient interface for LLM providers
type LLMClient interface {
	Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error)
}

// Analyzer performs code review analysis using the LLM
type Analyzer struct {
	llm LLMClient
}

// NewAnalyzer creates a new code review analyzer
func NewAnalyzer(llmClient LLMClient) *Analyzer {
	return &Analyzer{llm: llmClient}
}

// Issue represents a code review issue
type Issue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Line        int    `json:"line"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Suggestion  string `json:"suggestion"`
}

// ReviewResult contains the complete review output
type ReviewResult struct {
	Issues         []Issue `json:"issues"`
	Summary        string  `json:"summary"`
	RiskScore      FlexInt `json:"risk_score"`
	Recommendation string  `json:"recommendation"`
}

// ReviewDiff analyzes a code diff and returns review results
func (a *Analyzer) ReviewDiff(ctx context.Context, filePath, diff, fullContent string) (*ReviewResult, error) {
	lang := detectLanguage(filePath)

	prompt, err := llm.BuildReviewPrompt(llm.ReviewInput{
		Language:    lang,
		FilePath:    filePath,
		Diff:        diff,
		FullContent: fullContent,
	})
	if err != nil {
		return nil, fmt.Errorf("build prompt: %w", err)
	}

	response, err := a.llm.Complete(ctx, llm.SystemPrompt, prompt)
	if err != nil {
		return nil, fmt.Errorf("llm complete: %w", err)
	}

	// Parse JSON response
	result, err := parseReviewResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return result, nil
}

// parseReviewResponse extracts the ReviewResult from the LLM response
func parseReviewResponse(response string) (*ReviewResult, error) {
	// Clean up response - LLM might wrap in markdown
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Remove JavaScript-style comments that LLMs sometimes add
	response = removeJSONComments(response)

	var result ReviewResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w (response: %s)", err, response)
	}

	return &result, nil
}

// removeJSONComments strips // and /* */ style comments from JSON
func removeJSONComments(s string) string {
	var result strings.Builder
	inString := false
	inLineComment := false
	inBlockComment := false
	i := 0

	for i < len(s) {
		// Handle string literals (don't strip comments inside strings)
		if !inLineComment && !inBlockComment && s[i] == '"' {
			// Check if escaped
			escaped := false
			j := i - 1
			for j >= 0 && s[j] == '\\' {
				escaped = !escaped
				j--
			}
			if !escaped {
				inString = !inString
			}
			result.WriteByte(s[i])
			i++
			continue
		}

		// Inside a string, just copy
		if inString {
			result.WriteByte(s[i])
			i++
			continue
		}

		// Check for line comment start
		if !inBlockComment && i+1 < len(s) && s[i] == '/' && s[i+1] == '/' {
			inLineComment = true
			i += 2
			continue
		}

		// Check for block comment start
		if !inLineComment && i+1 < len(s) && s[i] == '/' && s[i+1] == '*' {
			inBlockComment = true
			i += 2
			continue
		}

		// Check for line comment end
		if inLineComment && s[i] == '\n' {
			inLineComment = false
			result.WriteByte(s[i])
			i++
			continue
		}

		// Check for block comment end
		if inBlockComment && i+1 < len(s) && s[i] == '*' && s[i+1] == '/' {
			inBlockComment = false
			i += 2
			continue
		}

		// Skip comment content
		if inLineComment || inBlockComment {
			i++
			continue
		}

		// Normal character
		result.WriteByte(s[i])
		i++
	}

	return result.String()
}

// detectLanguage determines the programming language from the file path
func detectLanguage(filePath string) string {
	switch {
	case strings.HasSuffix(filePath, ".go"):
		return "go"
	case strings.HasSuffix(filePath, ".py"):
		return "python"
	case strings.HasSuffix(filePath, ".js"):
		return "javascript"
	case strings.HasSuffix(filePath, ".ts"):
		return "typescript"
	case strings.HasSuffix(filePath, ".jsx"):
		return "javascript"
	case strings.HasSuffix(filePath, ".tsx"):
		return "typescript"
	case strings.HasSuffix(filePath, ".java"):
		return "java"
	case strings.HasSuffix(filePath, ".rs"):
		return "rust"
	case strings.HasSuffix(filePath, ".rb"):
		return "ruby"
	case strings.HasSuffix(filePath, ".php"):
		return "php"
	case strings.HasSuffix(filePath, ".cs"):
		return "csharp"
	case strings.HasSuffix(filePath, ".cpp"), strings.HasSuffix(filePath, ".cc"), strings.HasSuffix(filePath, ".cxx"):
		return "cpp"
	case strings.HasSuffix(filePath, ".c"), strings.HasSuffix(filePath, ".h"):
		return "c"
	case strings.HasSuffix(filePath, ".swift"):
		return "swift"
	case strings.HasSuffix(filePath, ".kt"), strings.HasSuffix(filePath, ".kts"):
		return "kotlin"
	case strings.HasSuffix(filePath, ".scala"):
		return "scala"
	case strings.HasSuffix(filePath, ".sh"), strings.HasSuffix(filePath, ".bash"):
		return "bash"
	case strings.HasSuffix(filePath, ".sql"):
		return "sql"
	case strings.HasSuffix(filePath, ".yaml"), strings.HasSuffix(filePath, ".yml"):
		return "yaml"
	case strings.HasSuffix(filePath, ".json"):
		return "json"
	case strings.HasSuffix(filePath, ".xml"):
		return "xml"
	case strings.HasSuffix(filePath, ".html"), strings.HasSuffix(filePath, ".htm"):
		return "html"
	case strings.HasSuffix(filePath, ".css"):
		return "css"
	case strings.HasSuffix(filePath, ".scss"), strings.HasSuffix(filePath, ".sass"):
		return "scss"
	default:
		return "code"
	}
}
