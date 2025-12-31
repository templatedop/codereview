package llm

import (
	"bytes"
	"text/template"
)

// SystemPrompt is the system prompt for code review
const SystemPrompt = `You are an expert code reviewer. Analyze code changes for:

1. **SECURITY** vulnerabilities:
   - SQL/NoSQL injection
   - XSS, CSRF
   - Authentication/Authorization flaws
   - Secrets/credentials exposure
   - Path traversal
   - Insecure deserialization

2. **PERFORMANCE** issues:
   - N+1 query problems
   - Memory leaks
   - Inefficient algorithms (O(n²) when O(n) possible)
   - Missing indexes hints
   - Unnecessary allocations
   - Blocking operations in hot paths

3. **CODE QUALITY**:
   - Error handling gaps
   - Race conditions
   - Resource leaks (unclosed connections, files)
   - Code duplication
   - Unclear naming
   - Missing validation

Respond ONLY with valid JSON in this exact format:
{
  "issues": [
    {
      "type": "SECURITY|PERFORMANCE|QUALITY",
      "severity": "CRITICAL|HIGH|MEDIUM|LOW",
      "line": <line_number_in_new_code>,
      "title": "Brief title",
      "description": "What's wrong and why it matters",
      "suggestion": "How to fix it with code example if applicable"
    }
  ],
  "summary": "One paragraph overall assessment",
  "risk_score": <1-10>,
  "recommendation": "APPROVE|REQUEST_CHANGES|COMMENT"
}

If no issues found, return empty issues array with APPROVE recommendation.`

// ReviewPromptTemplate is the template for generating review prompts
const ReviewPromptTemplate = `Review this {{.Language}} code change:

**File:** {{.FilePath}}

**Code Diff:**
` + "```diff" + `
{{.Diff}}
` + "```" + `

{{if .FullContent}}
**Full file context (for reference):**
` + "```{{.Language}}" + `
{{.FullContent}}
` + "```" + `
{{end}}

Analyze and respond with JSON only.`

// ReviewInput contains the input data for generating a review prompt
type ReviewInput struct {
	Language    string
	FilePath    string
	Diff        string
	FullContent string // Optional: full file for context
}

// BuildReviewPrompt generates the review prompt from the input
func BuildReviewPrompt(input ReviewInput) (string, error) {
	tmpl, err := template.New("review").Parse(ReviewPromptTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, input); err != nil {
		return "", err
	}

	return buf.String(), nil
}
