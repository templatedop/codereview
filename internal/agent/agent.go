package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yourorg/code-reviewer/internal/knowledge"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

// Tool represents a function the agent can call
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Parameters  string `json:"parameters"` // JSON schema
}

// ToolCall represents a request from the LLM to use a tool
type ToolCall struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args"`
	Result string          `json:"-"` // Filled after execution
}

// Agent orchestrates multi-step code review with tool use
type Agent struct {
	llm         reviewer.LLMClient
	store       *knowledge.Store
	tools       []Tool
	maxSteps    int
	frameworkName string
}

// NewAgent creates a new code review agent
func NewAgent(llm reviewer.LLMClient, store *knowledge.Store, frameworkName string) *Agent {
	agent := &Agent{
		llm:           llm,
		store:         store,
		frameworkName: frameworkName,
		maxSteps:      5,
	}
	agent.tools = agent.defineTools()
	return agent
}

// defineTools returns the available tools for the agent
func (a *Agent) defineTools() []Tool {
	return []Tool{
		{
			Name:        "search_framework",
			Description: "Search for relevant code patterns in the indexed framework. Use this to find how the framework handles similar cases.",
			Parameters:  `{"type": "object", "properties": {"query": {"type": "string", "description": "Search query for framework patterns"}}, "required": ["query"]}`,
		},
		{
			Name:        "analyze_security",
			Description: "Perform deep security analysis on a code snippet. Checks for SQL injection, XSS, command injection, etc.",
			Parameters:  `{"type": "object", "properties": {"code": {"type": "string", "description": "Code to analyze for security issues"}}, "required": ["code"]}`,
		},
		{
			Name:        "analyze_performance",
			Description: "Analyze code for performance issues like N+1 queries, memory leaks, inefficient algorithms.",
			Parameters:  `{"type": "object", "properties": {"code": {"type": "string", "description": "Code to analyze for performance"}}, "required": ["code"]}`,
		},
		{
			Name:        "suggest_fix",
			Description: "Generate a code fix suggestion for an identified issue.",
			Parameters:  `{"type": "object", "properties": {"issue": {"type": "string"}, "original_code": {"type": "string"}}, "required": ["issue", "original_code"]}`,
		},
		{
			Name:        "final_review",
			Description: "Generate the final review summary. Call this when analysis is complete.",
			Parameters:  `{"type": "object", "properties": {"issues": {"type": "array"}, "summary": {"type": "string"}, "recommendation": {"type": "string"}}, "required": ["issues", "summary", "recommendation"]}`,
		},
	}
}

// AgentState tracks the agent's progress
type AgentState struct {
	Step        int                   `json:"step"`
	FilePath    string                `json:"file_path"`
	Diff        string                `json:"diff"`
	FullContent string                `json:"full_content"`
	ToolCalls   []ToolCall            `json:"tool_calls"`
	Issues      []reviewer.Issue      `json:"issues"`
	Done        bool                  `json:"done"`
	FinalResult *reviewer.ReviewResult `json:"final_result,omitempty"`
}

// Review performs an agentic code review
func (a *Agent) Review(ctx context.Context, filePath, diff, fullContent string) (*reviewer.ReviewResult, error) {
	state := &AgentState{
		FilePath:    filePath,
		Diff:        diff,
		FullContent: fullContent,
		Issues:      []reviewer.Issue{},
	}

	// Agent loop
	for state.Step < a.maxSteps && !state.Done {
		state.Step++

		// Build prompt with current state and available tools
		prompt := a.buildAgentPrompt(state)
		systemPrompt := a.buildSystemPrompt()

		// Get LLM response
		response, err := a.llm.Complete(ctx, systemPrompt, prompt)
		if err != nil {
			return nil, fmt.Errorf("step %d: llm error: %w", state.Step, err)
		}

		// Parse tool calls from response
		toolCalls, err := a.parseToolCalls(response)
		if err != nil {
			// If no tool calls, try to extract final result
			if result := a.tryExtractResult(response); result != nil {
				return result, nil
			}
			continue
		}

		// Execute tool calls
		for _, tc := range toolCalls {
			result, err := a.executeTool(ctx, tc, state)
			if err != nil {
				tc.Result = fmt.Sprintf("Error: %v", err)
			} else {
				tc.Result = result
			}
			state.ToolCalls = append(state.ToolCalls, tc)

			// Check if final_review was called
			if tc.Name == "final_review" {
				state.Done = true
				if state.FinalResult != nil {
					return state.FinalResult, nil
				}
			}
		}
	}

	// If we exhausted steps, compile what we have
	return a.compileResult(state), nil
}

func (a *Agent) buildSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(`You are an expert code review agent. You analyze code step by step, using tools to gather information and identify issues.

Available tools:
`)
	for _, tool := range a.tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
	}

	sb.WriteString(`
When you need to use a tool, respond with JSON:
{"tool": "<tool_name>", "args": {<arguments>}}

When you're done analyzing, use the final_review tool with your complete findings.

Be thorough but efficient. Focus on:
1. Security vulnerabilities (critical)
2. Performance issues
3. Code quality problems
4. Framework pattern compliance
`)

	if a.frameworkName != "" {
		sb.WriteString(fmt.Sprintf("\nThe code should follow '%s' framework patterns.\n", a.frameworkName))
	}

	return sb.String()
}

func (a *Agent) buildAgentPrompt(state *AgentState) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("## Code Review Task (Step %d/%d)\n\n", state.Step, a.maxSteps))
	sb.WriteString(fmt.Sprintf("**File:** %s\n\n", state.FilePath))
	sb.WriteString("**Code Diff:**\n```diff\n")
	sb.WriteString(state.Diff)
	sb.WriteString("\n```\n\n")

	if len(state.ToolCalls) > 0 {
		sb.WriteString("## Previous Tool Results:\n\n")
		for _, tc := range state.ToolCalls {
			sb.WriteString(fmt.Sprintf("### %s\n", tc.Name))
			sb.WriteString(fmt.Sprintf("Args: %s\n", string(tc.Args)))
			sb.WriteString(fmt.Sprintf("Result: %s\n\n", tc.Result))
		}
	}

	sb.WriteString("## Your Task:\n")
	if state.Step == 1 {
		sb.WriteString("Analyze this code. Start by searching for relevant framework patterns if applicable, then check for security and performance issues.\n")
	} else if state.Step >= a.maxSteps-1 {
		sb.WriteString("This is your final step. Use final_review to submit your findings.\n")
	} else {
		sb.WriteString("Continue your analysis. Use tools as needed or call final_review if done.\n")
	}

	return sb.String()
}

func (a *Agent) parseToolCalls(response string) ([]ToolCall, error) {
	response = strings.TrimSpace(response)

	// Try to find JSON tool call
	start := strings.Index(response, "{")
	if start == -1 {
		return nil, fmt.Errorf("no tool call found")
	}

	// Find matching brace
	depth := 0
	end := -1
	for i := start; i < len(response); i++ {
		if response[i] == '{' {
			depth++
		} else if response[i] == '}' {
			depth--
			if depth == 0 {
				end = i + 1
				break
			}
		}
	}

	if end == -1 {
		return nil, fmt.Errorf("malformed JSON")
	}

	jsonStr := response[start:end]

	var call struct {
		Tool string          `json:"tool"`
		Args json.RawMessage `json:"args"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &call); err != nil {
		return nil, err
	}

	if call.Tool == "" {
		return nil, fmt.Errorf("no tool specified")
	}

	return []ToolCall{{Name: call.Tool, Args: call.Args}}, nil
}

func (a *Agent) executeTool(ctx context.Context, tc ToolCall, state *AgentState) (string, error) {
	switch tc.Name {
	case "search_framework":
		return a.toolSearchFramework(tc.Args)
	case "analyze_security":
		return a.toolAnalyzeSecurity(ctx, tc.Args, state)
	case "analyze_performance":
		return a.toolAnalyzePerformance(ctx, tc.Args, state)
	case "suggest_fix":
		return a.toolSuggestFix(ctx, tc.Args)
	case "final_review":
		return a.toolFinalReview(tc.Args, state)
	default:
		return "", fmt.Errorf("unknown tool: %s", tc.Name)
	}
}

func (a *Agent) toolSearchFramework(args json.RawMessage) (string, error) {
	if a.store == nil {
		return "No framework indexed.", nil
	}

	var params struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	elements := a.store.Search(params.Query, 3)
	if len(elements) == 0 {
		return "No matching patterns found.", nil
	}

	var sb strings.Builder
	sb.WriteString("Found relevant framework patterns:\n\n")
	for _, elem := range elements {
		sb.WriteString(fmt.Sprintf("### %s.%s (%s)\n", elem.Package, elem.Name, elem.Type))
		if elem.Doc != "" {
			sb.WriteString(fmt.Sprintf("Doc: %s\n", elem.Doc))
		}
		sb.WriteString(fmt.Sprintf("```go\n%s\n```\n\n", truncate(elem.Body, 300)))
	}

	return sb.String(), nil
}

func (a *Agent) toolAnalyzeSecurity(ctx context.Context, args json.RawMessage, state *AgentState) (string, error) {
	var params struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	// Use LLM for security analysis
	prompt := fmt.Sprintf("Analyze this code for security vulnerabilities:\n```\n%s\n```\n\nCheck for:\n- SQL injection\n- Command injection\n- XSS\n- Path traversal\n- Hardcoded secrets\n- Authentication issues\n\nReturn a brief list of findings.", params.Code)

	return a.llm.Complete(ctx, "You are a security analyst. Be concise.", prompt)
}

func (a *Agent) toolAnalyzePerformance(ctx context.Context, args json.RawMessage, state *AgentState) (string, error) {
	var params struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf("Analyze this code for performance issues:\n```\n%s\n```\n\nCheck for:\n- N+1 queries\n- Memory leaks\n- Inefficient algorithms\n- Unnecessary allocations\n- Missing caching opportunities\n\nReturn a brief list of findings.", params.Code)

	return a.llm.Complete(ctx, "You are a performance analyst. Be concise.", prompt)
}

func (a *Agent) toolSuggestFix(ctx context.Context, args json.RawMessage) (string, error) {
	var params struct {
		Issue        string `json:"issue"`
		OriginalCode string `json:"original_code"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return "", err
	}

	prompt := fmt.Sprintf("Suggest a fix for this issue:\nIssue: %s\n\nOriginal code:\n```\n%s\n```\n\nProvide the corrected code.", params.Issue, params.OriginalCode)

	return a.llm.Complete(ctx, "You are a code fixer. Provide only the fixed code.", prompt)
}

func (a *Agent) toolFinalReview(args json.RawMessage, state *AgentState) (string, error) {
	var result reviewer.ReviewResult
	if err := json.Unmarshal(args, &result); err != nil {
		// Try to parse as raw structure
		var raw struct {
			Issues []struct {
				Type        string `json:"type"`
				Severity    string `json:"severity"`
				Line        int    `json:"line"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Suggestion  string `json:"suggestion"`
			} `json:"issues"`
			Summary        string `json:"summary"`
			Recommendation string `json:"recommendation"`
		}
		if err := json.Unmarshal(args, &raw); err != nil {
			return "", fmt.Errorf("invalid final_review format: %w", err)
		}

		for _, issue := range raw.Issues {
			result.Issues = append(result.Issues, reviewer.Issue{
				Type:        issue.Type,
				Severity:    issue.Severity,
				Line:        issue.Line,
				Title:       issue.Title,
				Description: issue.Description,
				Suggestion:  issue.Suggestion,
			})
		}
		result.Summary = raw.Summary
		result.Recommendation = raw.Recommendation
		result.RiskScore = reviewer.FlexInt(len(result.Issues) * 2) // Simple risk calculation
	}

	state.FinalResult = &result
	return "Review completed.", nil
}

func (a *Agent) tryExtractResult(response string) *reviewer.ReviewResult {
	// Try to parse response as final result JSON
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil
	}

	var result reviewer.ReviewResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil
	}

	if result.Summary != "" || len(result.Issues) > 0 {
		return &result
	}
	return nil
}

func (a *Agent) compileResult(state *AgentState) *reviewer.ReviewResult {
	if state.FinalResult != nil {
		return state.FinalResult
	}

	// Compile from collected issues
	return &reviewer.ReviewResult{
		Issues:         state.Issues,
		Summary:        "Review completed after maximum steps.",
		RiskScore:      reviewer.FlexInt(len(state.Issues) * 2),
		Recommendation: "REQUEST_CHANGES",
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

func extractJSON(s string) string {
	start := strings.Index(s, "{")
	if start == -1 {
		return ""
	}
	depth := 0
	for i := start; i < len(s); i++ {
		if s[i] == '{' {
			depth++
		} else if s[i] == '}' {
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return ""
}
