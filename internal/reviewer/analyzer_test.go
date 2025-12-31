package reviewer

import (
	"context"
	"encoding/json"
	"testing"
)

// Mock LLM client for testing
type mockLLMClient struct {
	response string
	err      error
}

func (m *mockLLMClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func TestFlexInt_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{"integer", `5`, 5},
		{"string number", `"10"`, 10},
		{"string with spaces", `" 7 "`, 7},
		{"empty string", `""`, 0},
		{"zero", `0`, 0},
		{"negative int", `-3`, -3},
		{"string with text", `"high: 8"`, 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fi FlexInt
			err := json.Unmarshal([]byte(tt.input), &fi)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if int(fi) != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, int(fi))
			}
		})
	}
}

func TestExtractJSON(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean json",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with markdown",
			input:    "Here is the result:\n```json\n{\"key\": \"value\"}\n```\nDone!",
			expected: `{"key": "value"}`,
		},
		{
			name:     "json with prefix text",
			input:    "Analysis complete. {\"issues\": []}",
			expected: `{"issues": []}`,
		},
		{
			name:     "nested braces",
			input:    `{"outer": {"inner": "value"}}`,
			expected: `{"outer": {"inner": "value"}}`,
		},
		{
			name:     "braces in strings",
			input:    `{"code": "if (x) { y }"}`,
			expected: `{"code": "if (x) { y }"}`,
		},
		{
			name:     "no json",
			input:    "No JSON here",
			expected: "",
		},
		{
			name:     "empty object",
			input:    `{}`,
			expected: `{}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJSON(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRemoveJSONComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "line comment",
			input:    "{\n  \"key\": \"value\" // comment\n}",
			expected: "{\n  \"key\": \"value\" \n}",
		},
		{
			name:     "block comment",
			input:    `{"key": /* comment */ "value"}`,
			expected: `{"key":  "value"}`,
		},
		{
			name:     "comment-like in string",
			input:    `{"url": "http://example.com"}`,
			expected: `{"url": "http://example.com"}`,
		},
		{
			name:     "multiple comments",
			input:    "{\n  // first\n  \"a\": 1, // inline\n  /* block */ \"b\": 2\n}",
			expected: "{\n  \n  \"a\": 1, \n   \"b\": 2\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := removeJSONComments(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"app.js", "javascript"},
		{"component.tsx", "typescript"},
		{"Main.java", "java"},
		{"lib.rs", "rust"},
		{"config.yaml", "yaml"},
		{"data.json", "json"},
		{"index.html", "html"},
		{"style.css", "css"},
		{"unknown.xyz", "code"},
		{"path/to/file.go", "go"},
		{"src/components/Button.tsx", "typescript"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			result := detectLanguage(tt.path)
			if result != tt.expected {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestParseReviewResponse(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantErr     bool
		issueCount  int
		riskScore   int
	}{
		{
			name: "valid response",
			input: `{
				"issues": [
					{"type": "security", "severity": "high", "line": 10, "description": "SQL injection"}
				],
				"summary": "Found 1 issue",
				"risk_score": 8
			}`,
			wantErr:    false,
			issueCount: 1,
			riskScore:  8,
		},
		{
			name: "response with markdown wrapper",
			input: "```json\n{\"issues\": [], \"summary\": \"No issues\", \"risk_score\": 0}\n```",
			wantErr:    false,
			issueCount: 0,
			riskScore:  0,
		},
		{
			name: "response with comments",
			input: `{
				// Security analysis
				"issues": [],
				"summary": "Clean",
				"risk_score": 1
			}`,
			wantErr:    false,
			issueCount: 0,
			riskScore:  1,
		},
		{
			name: "risk_score as string",
			input: `{"issues": [], "summary": "OK", "risk_score": "5"}`,
			wantErr:    false,
			issueCount: 0,
			riskScore:  5,
		},
		{
			name:    "no json",
			input:   "I couldn't analyze the code.",
			wantErr: true,
		},
		{
			name:    "invalid json",
			input:   `{"issues": [}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseReviewResponse(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Issues) != tt.issueCount {
				t.Errorf("expected %d issues, got %d", tt.issueCount, len(result.Issues))
			}
			if int(result.RiskScore) != tt.riskScore {
				t.Errorf("expected risk_score %d, got %d", tt.riskScore, int(result.RiskScore))
			}
		})
	}
}

func TestNewAnalyzer(t *testing.T) {
	client := &mockLLMClient{}
	analyzer := NewAnalyzer(client)
	if analyzer == nil {
		t.Fatal("expected non-nil analyzer")
	}
	if analyzer.llm != client {
		t.Error("llm client not set correctly")
	}
}

func TestAnalyzer_ReviewDiff(t *testing.T) {
	mockResponse := `{
		"issues": [
			{
				"type": "security",
				"severity": "high",
				"line": 15,
				"title": "SQL Injection",
				"description": "User input used directly in query",
				"suggestion": "Use parameterized queries"
			}
		],
		"summary": "Found SQL injection vulnerability",
		"risk_score": 9,
		"recommendation": "Fix immediately"
	}`

	client := &mockLLMClient{response: mockResponse}
	analyzer := NewAnalyzer(client)

	ctx := context.Background()
	result, err := analyzer.ReviewDiff(ctx, "db.go", "+query := \"SELECT * FROM users WHERE id=\" + userID", "package main")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(result.Issues))
	}

	if result.Issues[0].Type != "security" {
		t.Errorf("expected type 'security', got %q", result.Issues[0].Type)
	}

	if int(result.RiskScore) != 9 {
		t.Errorf("expected risk_score 9, got %d", int(result.RiskScore))
	}
}

func TestReviewResult_JSON(t *testing.T) {
	result := ReviewResult{
		Issues: []Issue{
			{
				Type:        "security",
				Severity:    "high",
				Line:        42,
				Title:       "Test Issue",
				Description: "Description",
				Suggestion:  "Fix it",
			},
		},
		Summary:        "Summary",
		RiskScore:      FlexInt(7),
		Recommendation: "Recommendation",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ReviewResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if len(decoded.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(decoded.Issues))
	}
	if int(decoded.RiskScore) != 7 {
		t.Errorf("expected risk_score 7, got %d", int(decoded.RiskScore))
	}
}
