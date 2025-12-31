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

// SystemPromptWithFramework is used when framework context is available
const SystemPromptWithFramework = `You are an expert code reviewer for a project using the "{{.FrameworkName}}" framework.

**FRAMEWORK CONTEXT:**
You must review code against the patterns and conventions of this framework. The code should:
- Follow the framework's established patterns
- Use the framework's utilities and helpers correctly
- Be consistent with the framework's coding style
- Properly integrate with the framework's components

Analyze code changes for:

1. **FRAMEWORK COMPLIANCE**:
   - Does it follow the framework's patterns?
   - Is it using the framework's utilities correctly?
   - Are there framework features that should be used instead?

2. **SECURITY** vulnerabilities:
   - SQL/NoSQL injection
   - XSS, CSRF
   - Authentication/Authorization flaws
   - Secrets/credentials exposure

3. **PERFORMANCE** issues:
   - N+1 query problems
   - Memory leaks
   - Inefficient algorithms
   - Blocking operations

4. **CODE QUALITY**:
   - Error handling gaps
   - Race conditions
   - Resource leaks
   - Unclear naming

Respond ONLY with valid JSON in this exact format:
{
  "issues": [
    {
      "type": "FRAMEWORK|SECURITY|PERFORMANCE|QUALITY",
      "severity": "CRITICAL|HIGH|MEDIUM|LOW",
      "line": <line_number>,
      "title": "Brief title",
      "description": "What's wrong and why it matters",
      "suggestion": "How to fix it, referencing framework patterns when applicable"
    }
  ],
  "summary": "Overall assessment including framework compliance",
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

// ReviewPromptWithFramework includes framework context
const ReviewPromptWithFrameworkTemplate = `Review this {{.Language}} code change against the "{{.FrameworkName}}" framework:

**File:** {{.FilePath}}

**Code Diff:**
` + "```diff" + `
{{.Diff}}
` + "```" + `

{{if .FrameworkContext}}
**RELEVANT FRAMEWORK CODE (for reference):**
The following are relevant patterns and functions from the framework that may apply to this code:

{{range .FrameworkContext}}
---
**{{.Type}}: {{.Package}}.{{.Name}}**
{{if .Doc}}*{{.Doc}}*{{end}}
` + "```go" + `
{{.Body}}
` + "```" + `
{{end}}
{{end}}

{{if .FullContent}}
**Full file context:**
` + "```{{.Language}}" + `
{{.FullContent}}
` + "```" + `
{{end}}

Review the code and check:
1. Does it follow the framework patterns shown above?
2. Should it use any framework utilities instead of custom code?
3. Are there security, performance, or quality issues?

Respond with JSON only.`

// ReviewInput contains the input data for generating a review prompt
type ReviewInput struct {
	Language    string
	FilePath    string
	Diff        string
	FullContent string // Optional: full file for context
}

// ReviewInputWithFramework extends ReviewInput with framework context
type ReviewInputWithFramework struct {
	ReviewInput
	FrameworkName    string
	FrameworkContext []FrameworkElement
}

// FrameworkElement represents a relevant piece of framework code
type FrameworkElement struct {
	Type    string
	Package string
	Name    string
	Doc     string
	Body    string
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

// BuildSystemPromptWithFramework generates system prompt with framework name
func BuildSystemPromptWithFramework(frameworkName string) (string, error) {
	tmpl, err := template.New("system").Parse(SystemPromptWithFramework)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"FrameworkName": frameworkName}); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// BuildReviewPromptWithFramework generates the review prompt with framework context
func BuildReviewPromptWithFramework(input ReviewInputWithFramework) (string, error) {
	tmpl, err := template.New("review").Parse(ReviewPromptWithFrameworkTemplate)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, input); err != nil {
		return "", err
	}

	return buf.String(), nil
}
