package postgres

import (
	"context"
	"testing"

	"github.com/yourorg/code-reviewer/internal/rag"
)

// MockLLMClient is a mock implementation of LLMClient for testing.
type MockLLMClient struct {
	response string
	err      error
}

func (m *MockLLMClient) Complete(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

// MockEmbedder for testing
type mockEmbedder struct{}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return []float32{0.1, 0.2, 0.3}, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1, 0.2, 0.3}
	}
	return result, nil
}

func (m *mockEmbedder) Dimension() int {
	return 3
}

func TestAnalyzer_AnalyzePerformance(t *testing.T) {
	llm := &MockLLMClient{
		response: `{
			"issues": [
				{
					"severity": "HIGH",
					"category": "INDEX",
					"line": 10,
					"query": "SELECT * FROM users WHERE email = $1",
					"problem": "Missing index on email column",
					"suggestion": "CREATE INDEX idx_users_email ON users(email)"
				}
			],
			"indexes_to_create": [
				{
					"table": "users",
					"columns": "email",
					"type": "btree",
					"reason": "Frequently queried column",
					"create_statement": "CREATE INDEX CONCURRENTLY idx_users_email ON users(email)"
				}
			]
		}`,
	}

	store := rag.NewMemoryVectorStore("")
	embedder := &mockEmbedder{}
	retriever := rag.NewRetriever(embedder, store, nil)
	analyzer := NewAnalyzer(llm, retriever)

	input := AnalysisInput{
		SQLFragments: []SQLFragment{
			{
				Query:     "SELECT * FROM users WHERE email = $1",
				Line:      10,
				QueryType: SQLTypeSelect,
				Tables:    []string{"users"},
			},
		},
	}

	result, err := analyzer.AnalyzePerformance(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzePerformance() error = %v", err)
	}

	if len(result.Issues) != 1 {
		t.Errorf("AnalyzePerformance() got %d issues, want 1", len(result.Issues))
	}

	if len(result.IndexSuggestions) != 1 {
		t.Errorf("AnalyzePerformance() got %d index suggestions, want 1", len(result.IndexSuggestions))
	}

	if result.Issues[0].Severity != "HIGH" {
		t.Errorf("Issue severity = %s, want HIGH", result.Issues[0].Severity)
	}
}

func TestAnalyzer_AnalyzeSecurity(t *testing.T) {
	llm := &MockLLMClient{
		response: `{
			"vulnerabilities": [
				{
					"severity": "CRITICAL",
					"type": "SQL_INJECTION",
					"cwe": "CWE-89",
					"line": 5,
					"code": "query := fmt.Sprintf(\"SELECT * FROM users WHERE id = %s\", userInput)",
					"problem": "SQL injection vulnerability via string concatenation",
					"suggestion": "Use parameterized queries: db.Query(\"SELECT * FROM users WHERE id = $1\", userInput)"
				}
			]
		}`,
	}

	store := rag.NewMemoryVectorStore("")
	embedder := &mockEmbedder{}
	retriever := rag.NewRetriever(embedder, store, nil)
	analyzer := NewAnalyzer(llm, retriever)

	input := AnalysisInput{
		SQLFragments: []SQLFragment{
			{
				Query:     "SELECT * FROM users WHERE id = %s",
				Line:      5,
				IsDynamic: true,
			},
		},
		FileContent: `query := fmt.Sprintf("SELECT * FROM users WHERE id = %s", userInput)`,
		Language:    "go",
	}

	result, err := analyzer.AnalyzeSecurity(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzeSecurity() error = %v", err)
	}

	if len(result.Vulnerabilities) != 1 {
		t.Errorf("AnalyzeSecurity() got %d vulnerabilities, want 1", len(result.Vulnerabilities))
	}

	if result.Vulnerabilities[0].Severity != "CRITICAL" {
		t.Errorf("Vulnerability severity = %s, want CRITICAL", result.Vulnerabilities[0].Severity)
	}

	if result.Vulnerabilities[0].CWE != "CWE-89" {
		t.Errorf("Vulnerability CWE = %s, want CWE-89", result.Vulnerabilities[0].CWE)
	}
}

func TestAnalyzer_AnalyzeStandards(t *testing.T) {
	llm := &MockLLMClient{
		response: `{
			"violations": [
				{
					"severity": "WARNING",
					"category": "NAMING",
					"line": 1,
					"element": "tbl_Users",
					"problem": "Table name uses incorrect casing",
					"suggestion": "Use snake_case: users"
				}
			],
			"migration_safety": {
				"is_safe": true,
				"blocking_operations": [],
				"recommendations": []
			}
		}`,
	}

	store := rag.NewMemoryVectorStore("")
	embedder := &mockEmbedder{}
	retriever := rag.NewRetriever(embedder, store, nil)
	analyzer := NewAnalyzer(llm, retriever)

	input := AnalysisInput{
		SQLFragments: []SQLFragment{
			{
				Query:     "CREATE TABLE tbl_Users (ID INT PRIMARY KEY)",
				Line:      1,
				QueryType: SQLTypeCreate,
			},
		},
	}

	result, err := analyzer.AnalyzeStandards(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzeStandards() error = %v", err)
	}

	if len(result.Violations) != 1 {
		t.Errorf("AnalyzeStandards() got %d violations, want 1", len(result.Violations))
	}

	if !result.MigrationSafety.IsSafe {
		t.Error("MigrationSafety.IsSafe = false, want true")
	}
}

func TestAnalyzer_AnalyzeAll(t *testing.T) {
	llm := &MockLLMClient{
		response: `{"issues": [], "vulnerabilities": [], "violations": [], "migration_safety": {"is_safe": true}}`,
	}

	store := rag.NewMemoryVectorStore("")
	embedder := &mockEmbedder{}
	retriever := rag.NewRetriever(embedder, store, nil)
	analyzer := NewAnalyzer(llm, retriever)

	input := AnalysisInput{
		FileContent: `
func getUser(db *sql.DB, id int) {
	db.Query("SELECT * FROM users WHERE id = $1", id)
}`,
		Language: "go",
		FilePath: "user.go",
	}

	result, err := analyzer.AnalyzeAll(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzeAll() error = %v", err)
	}

	if result.Performance == nil {
		t.Error("AnalyzeAll() Performance is nil")
	}
	if result.Security == nil {
		t.Error("AnalyzeAll() Security is nil")
	}
	if result.Standards == nil {
		t.Error("AnalyzeAll() Standards is nil")
	}
}

func TestAnalyzer_EmptyInput(t *testing.T) {
	llm := &MockLLMClient{}
	analyzer := NewAnalyzer(llm, nil)

	input := AnalysisInput{}

	perf, err := analyzer.AnalyzePerformance(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzePerformance() with empty input error = %v", err)
	}
	if len(perf.Issues) != 0 {
		t.Errorf("AnalyzePerformance() with empty input got %d issues, want 0", len(perf.Issues))
	}

	sec, err := analyzer.AnalyzeSecurity(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzeSecurity() with empty input error = %v", err)
	}
	if len(sec.Vulnerabilities) != 0 {
		t.Errorf("AnalyzeSecurity() with empty input got %d vulnerabilities, want 0", len(sec.Vulnerabilities))
	}
}

func TestAnalyzer_MalformedJSONResponse(t *testing.T) {
	llm := &MockLLMClient{
		response: `This is not valid JSON but mentions an issue with performance`,
	}

	store := rag.NewMemoryVectorStore("")
	embedder := &mockEmbedder{}
	retriever := rag.NewRetriever(embedder, store, nil)
	analyzer := NewAnalyzer(llm, retriever)

	input := AnalysisInput{
		SQLFragments: []SQLFragment{
			{Query: "SELECT * FROM users", Line: 1},
		},
	}

	// Should not error, but extract issues from text
	result, err := analyzer.AnalyzePerformance(context.Background(), input)
	if err != nil {
		t.Fatalf("AnalyzePerformance() with malformed JSON error = %v", err)
	}

	// The fallback parser should handle this gracefully
	_ = result
}

func TestParseJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		response string
		wantErr  bool
	}{
		{
			name:     "valid JSON",
			response: `{"issues": []}`,
			wantErr:  false,
		},
		{
			name:     "JSON with prefix text",
			response: `Here is the analysis:\n{"issues": []}`,
			wantErr:  false,
		},
		{
			name:     "JSON with suffix text",
			response: `{"issues": []}\n\nLet me know if you need more details.`,
			wantErr:  false,
		},
		{
			name:     "no JSON",
			response: `This is just plain text with no JSON`,
			wantErr:  true,
		},
		{
			name:     "single quotes to double quotes",
			response: `{'issues': []}`,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result map[string]interface{}
			err := parseJSONResponse(tt.response, &result)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestExtractIssuesFromText(t *testing.T) {
	text := `
The code has several issues:
1. Performance issue with the query
2. Security vulnerability detected
3. Problem with indexing
`

	issues := extractIssuesFromText(text, "performance")

	if len(issues) == 0 {
		t.Error("extractIssuesFromText() returned no issues")
	}

	for _, issue := range issues {
		if issue.Category != "performance" {
			t.Errorf("Issue category = %s, want performance", issue.Category)
		}
	}
}

func TestFormatFragmentsForPrompt(t *testing.T) {
	fragments := []SQLFragment{
		{
			Query:     "SELECT * FROM users",
			Line:      10,
			Tables:    []string{"users"},
			IsDynamic: false,
		},
		{
			Query:     "SELECT * FROM orders WHERE id = $1",
			Line:      20,
			Tables:    []string{"orders"},
			IsDynamic: true,
		},
	}

	result := formatFragmentsForPrompt(fragments)

	if result == "" {
		t.Error("formatFragmentsForPrompt() returned empty string")
	}

	if !containsStr(result, "Query 1") {
		t.Error("formatFragmentsForPrompt() missing Query 1")
	}
	if !containsStr(result, "Query 2") {
		t.Error("formatFragmentsForPrompt() missing Query 2")
	}
	if !containsStr(result, "WARNING") {
		t.Error("formatFragmentsForPrompt() missing dynamic SQL warning")
	}
	if !containsStr(result, "Tables:") {
		t.Error("formatFragmentsForPrompt() missing tables")
	}
}

func TestTruncateContent(t *testing.T) {
	tests := []struct {
		content  string
		maxChars int
		want     string
	}{
		{"short", 100, "short"},
		{"this is longer content", 10, "this is lo\n... [truncated]"},
		{"", 10, ""},
	}

	for _, tt := range tests {
		got := truncateContent(tt.content, tt.maxChars)
		if got != tt.want {
			t.Errorf("truncateContent(%q, %d) = %q, want %q", tt.content, tt.maxChars, got, tt.want)
		}
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
