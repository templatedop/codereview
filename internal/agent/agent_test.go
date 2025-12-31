package agent

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/yourorg/code-reviewer/internal/indexer"
	"github.com/yourorg/code-reviewer/internal/knowledge"
	"github.com/yourorg/code-reviewer/internal/reviewer"
)

// Mock LLM client for testing
type mockLLMClient struct {
	responses []string
	callCount int
	err       error
}

func (m *mockLLMClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	if m.callCount < len(m.responses) {
		response := m.responses[m.callCount]
		m.callCount++
		return response, nil
	}
	return "", nil
}

func TestNewAgent(t *testing.T) {
	llm := &mockLLMClient{}
	agent := NewAgent(llm, nil, "")

	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
	if agent.llm != llm {
		t.Error("llm client not set correctly")
	}
	if agent.maxSteps != 5 {
		t.Errorf("expected maxSteps 5, got %d", agent.maxSteps)
	}
}

func TestNewAgent_WithFramework(t *testing.T) {
	llm := &mockLLMClient{}
	agent := NewAgent(llm, nil, "gin")

	if agent.frameworkName != "gin" {
		t.Errorf("expected frameworkName 'gin', got %q", agent.frameworkName)
	}
}

func TestAgent_Tools(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	if len(agent.tools) == 0 {
		t.Fatal("expected at least one tool")
	}

	expectedTools := map[string]bool{
		"search_framework":    false,
		"analyze_security":    false,
		"analyze_performance": false,
		"suggest_fix":         false,
		"final_review":        false,
	}

	for _, tool := range agent.tools {
		if _, ok := expectedTools[tool.Name]; ok {
			expectedTools[tool.Name] = true
		}
	}

	for name, found := range expectedTools {
		if !found {
			t.Errorf("missing expected tool: %s", name)
		}
	}
}

func TestTool_Fields(t *testing.T) {
	tool := Tool{
		Name:        "test_tool",
		Description: "A test tool",
		Parameters:  `{"type": "object", "properties": {}}`,
	}

	if tool.Name != "test_tool" {
		t.Errorf("expected name 'test_tool', got %q", tool.Name)
	}
	if tool.Description != "A test tool" {
		t.Errorf("expected description 'A test tool', got %q", tool.Description)
	}
	if tool.Parameters == "" {
		t.Error("expected non-empty parameters")
	}
}

func TestToolCall_JSON(t *testing.T) {
	call := ToolCall{
		Name: "search_framework",
		Args: json.RawMessage(`{"query": "handler"}`),
	}

	data, err := json.Marshal(call)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ToolCall
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Name != call.Name {
		t.Errorf("expected name %q, got %q", call.Name, decoded.Name)
	}
}

func TestAgentState(t *testing.T) {
	state := &AgentState{
		Step:        1,
		FilePath:    "test.go",
		Diff:        "+func test() {}",
		FullContent: "package main\n\nfunc test() {}",
		ToolCalls:   []ToolCall{},
		Issues:      []reviewer.Issue{},
		Done:        false,
	}

	if state.Step != 1 {
		t.Error("Step not set correctly")
	}
	if state.FilePath != "test.go" {
		t.Error("FilePath not set correctly")
	}
	if state.Done {
		t.Error("Done should be false")
	}
}

func TestAgentState_WithFinalResult(t *testing.T) {
	result := &reviewer.ReviewResult{
		Issues:    []reviewer.Issue{{Type: "security", Severity: "high"}},
		Summary:   "Test summary",
		RiskScore: 5,
	}

	state := &AgentState{
		Done:        true,
		FinalResult: result,
	}

	if !state.Done {
		t.Error("Done should be true")
	}
	if state.FinalResult == nil {
		t.Error("FinalResult should not be nil")
	}
	if len(state.FinalResult.Issues) != 1 {
		t.Error("FinalResult.Issues should have 1 issue")
	}
}

func createTestStore(t *testing.T) (*knowledge.Store, string) {
	tmpDir, err := os.MkdirTemp("", "agent-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	store := knowledge.NewStore(tmpDir)

	elements := []indexer.CodeElement{
		{Name: "HandleRequest", Type: "function", Package: "handlers", Doc: "Handles HTTP requests"},
		{Name: "Handler", Type: "interface", Package: "handlers", Doc: "Handler interface"},
	}

	if err := store.SaveFramework("test-framework", "", "", elements); err != nil {
		t.Fatalf("failed to save framework: %v", err)
	}

	// Load to initialize index
	if _, err := store.LoadFramework("test-framework"); err != nil {
		t.Fatalf("failed to load framework: %v", err)
	}

	return store, tmpDir
}

func TestAgent_ToolSearchFramework_NoStore(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	args := json.RawMessage(`{"query": "handler"}`)
	result, err := agent.toolSearchFramework(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "No framework indexed." {
		t.Errorf("expected 'No framework indexed.', got %q", result)
	}
}

func TestAgent_ToolSearchFramework_WithStore(t *testing.T) {
	store, tmpDir := createTestStore(t)
	defer os.RemoveAll(tmpDir)

	agent := NewAgent(&mockLLMClient{}, store, "test-framework")

	args := json.RawMessage(`{"query": "handler"}`)
	result, err := agent.toolSearchFramework(args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" || result == "No framework knowledge base loaded" {
		t.Error("expected search results")
	}
}

func TestAgent_ToolFinalReview(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")
	state := &AgentState{
		Issues: []reviewer.Issue{},
	}

	reviewJSON := `{
		"issues": [
			{"type": "security", "severity": "high", "line": 10, "description": "SQL injection"}
		],
		"summary": "Critical issues found",
		"risk_score": 8
	}`

	args := json.RawMessage(reviewJSON)
	result, err := agent.toolFinalReview(args, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}

	// State should have final result set
	if state.FinalResult == nil {
		t.Error("expected FinalResult to be set in state")
	}
}

func TestAgent_ExecuteTool_SearchFramework(t *testing.T) {
	store, tmpDir := createTestStore(t)
	defer os.RemoveAll(tmpDir)

	agent := NewAgent(&mockLLMClient{}, store, "test-framework")
	state := &AgentState{}

	call := ToolCall{
		Name: "search_framework",
		Args: json.RawMessage(`{"query": "test"}`),
	}

	result, err := agent.executeTool(context.Background(), call, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgent_ExecuteTool_AnalyzeSecurity(t *testing.T) {
	mockLLM := &mockLLMClient{
		responses: []string{"Found potential SQL injection on line 10"},
	}
	agent := NewAgent(mockLLM, nil, "")
	state := &AgentState{}

	call := ToolCall{
		Name: "analyze_security",
		Args: json.RawMessage(`{"code": "query := \"SELECT * FROM users WHERE id=\" + id"}`),
	}

	result, err := agent.executeTool(context.Background(), call, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgent_ExecuteTool_AnalyzePerformance(t *testing.T) {
	mockLLM := &mockLLMClient{
		responses: []string{"No performance issues found"},
	}
	agent := NewAgent(mockLLM, nil, "")
	state := &AgentState{}

	call := ToolCall{
		Name: "analyze_performance",
		Args: json.RawMessage(`{"code": "for i := 0; i < len(items); i++ { process(items[i]) }"}`),
	}

	result, err := agent.executeTool(context.Background(), call, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgent_ExecuteTool_SuggestFix(t *testing.T) {
	mockLLM := &mockLLMClient{
		responses: []string{"query, args := \"SELECT * FROM users WHERE id=?\", []interface{}{id}"},
	}
	agent := NewAgent(mockLLM, nil, "")
	state := &AgentState{}

	call := ToolCall{
		Name: "suggest_fix",
		Args: json.RawMessage(`{"issue": "SQL injection", "original_code": "query := \"SELECT * FROM users WHERE id=\" + id"}`),
	}

	result, err := agent.executeTool(context.Background(), call, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgent_ExecuteTool_FinalReview(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")
	state := &AgentState{}

	call := ToolCall{
		Name: "final_review",
		Args: json.RawMessage(`{"issues": [], "summary": "No issues", "recommendation": "Looks good"}`),
	}

	result, err := agent.executeTool(context.Background(), call, state)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestAgent_ExecuteTool_UnknownTool(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")
	state := &AgentState{}

	call := ToolCall{
		Name: "nonexistent_tool",
		Args: json.RawMessage(`{}`),
	}

	_, err := agent.executeTool(context.Background(), call, state)
	if err == nil {
		t.Error("expected error for unknown tool")
	}
}

func TestAgent_CompileResult(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	state := &AgentState{
		Issues: []reviewer.Issue{
			{Type: "security", Severity: "high", Line: 10, Description: "SQL injection"},
			{Type: "performance", Severity: "medium", Line: 20, Description: "N+1 query"},
		},
	}

	result := agent.compileResult(state)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Issues) != 2 {
		t.Errorf("expected 2 issues, got %d", len(result.Issues))
	}

	if result.Summary == "" {
		t.Error("expected non-empty summary")
	}
}

func TestAgent_CompileResult_NoIssues(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	state := &AgentState{
		Issues: []reviewer.Issue{},
	}

	result := agent.compileResult(state)

	if result == nil {
		t.Fatal("expected non-nil result")
	}

	if len(result.Issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(result.Issues))
	}
}

func TestAgent_Review_Simple(t *testing.T) {
	// Create a mock that returns something resembling a final review
	mockLLM := &mockLLMClient{
		responses: []string{
			`I'll call final_review with my findings.
{"tool_calls": [{"name": "final_review", "args": {"issues": [], "summary": "No issues found", "recommendation": "Code looks good"}}]}`,
		},
	}

	agent := NewAgent(mockLLM, nil, "")

	ctx := context.Background()
	result, err := agent.Review(ctx, "test.go", "+func test() {}", "package main\n\nfunc test() {}")

	// The function should not error
	if err != nil {
		t.Fatalf("Review returned error: %v", err)
	}

	// We should get some result
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestAgent_BuildSystemPrompt(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	prompt := agent.buildSystemPrompt()

	if prompt == "" {
		t.Error("expected non-empty system prompt")
	}

	// Should contain key elements
	if !contains(prompt, "code review") && !contains(prompt, "security") {
		t.Error("system prompt should mention code review or security")
	}
}

func TestAgent_BuildAgentPrompt(t *testing.T) {
	agent := NewAgent(&mockLLMClient{}, nil, "")

	state := &AgentState{
		Step:        1,
		FilePath:    "test.go",
		Diff:        "+func test() {}",
		FullContent: "package main",
	}

	prompt := agent.buildAgentPrompt(state)

	if prompt == "" {
		t.Error("expected non-empty agent prompt")
	}

	// Should contain file info
	if !contains(prompt, "test.go") {
		t.Error("agent prompt should contain file path")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
