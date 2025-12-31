package knowledge

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/yourorg/code-reviewer/internal/indexer"
)

func TestNewStore(t *testing.T) {
	store := NewStore("/tmp/test-knowledge")

	if store.dataDir != "/tmp/test-knowledge" {
		t.Errorf("expected dataDir '/tmp/test-knowledge', got %q", store.dataDir)
	}
	if store.index == nil {
		t.Error("expected index to be initialized")
	}
}

func TestNewStore_DefaultDir(t *testing.T) {
	store := NewStore("")

	homeDir, _ := os.UserHomeDir()
	expected := filepath.Join(homeDir, ".code-reviewer", "knowledge")

	if store.dataDir != expected {
		t.Errorf("expected default dataDir %q, got %q", expected, store.dataDir)
	}
}

func TestStore_SaveAndLoadFramework(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	elements := []indexer.CodeElement{
		{
			Type:      "function",
			Name:      "HandleRequest",
			Package:   "handlers",
			FilePath:  "handlers/request.go",
			Line:      10,
			Signature: "func HandleRequest(w http.ResponseWriter, r *http.Request)",
			Doc:       "HandleRequest handles incoming HTTP requests",
		},
		{
			Type:     "struct",
			Name:     "Config",
			Package:  "config",
			FilePath: "config/config.go",
			Line:     5,
			Doc:      "Config holds application configuration",
		},
	}

	err = store.SaveFramework("test-framework", "github.com/test/repo", "/path/to/repo", elements)
	if err != nil {
		t.Fatalf("SaveFramework failed: %v", err)
	}

	// Verify file was created
	filePath := filepath.Join(tmpDir, "test-framework.json")
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatal("framework file was not created")
	}

	// Load and verify
	framework, err := store.LoadFramework("test-framework")
	if err != nil {
		t.Fatalf("LoadFramework failed: %v", err)
	}

	if framework.Name != "test-framework" {
		t.Errorf("expected name 'test-framework', got %q", framework.Name)
	}
	if framework.ElementCount != 2 {
		t.Errorf("expected 2 elements, got %d", framework.ElementCount)
	}
	if len(framework.Packages) != 2 {
		t.Errorf("expected 2 packages, got %d", len(framework.Packages))
	}
}

func TestStore_ListFrameworks(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	// Initially empty
	frameworks, err := store.ListFrameworks()
	if err != nil {
		t.Fatalf("ListFrameworks failed: %v", err)
	}
	if len(frameworks) != 0 {
		t.Errorf("expected 0 frameworks, got %d", len(frameworks))
	}

	// Save some frameworks
	store.SaveFramework("framework-a", "", "", []indexer.CodeElement{})
	store.SaveFramework("framework-b", "", "", []indexer.CodeElement{})

	frameworks, err = store.ListFrameworks()
	if err != nil {
		t.Fatalf("ListFrameworks failed: %v", err)
	}
	if len(frameworks) != 2 {
		t.Errorf("expected 2 frameworks, got %d", len(frameworks))
	}
}

func TestStore_Search(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	elements := []indexer.CodeElement{
		{Type: "function", Name: "HandleRequest", Package: "handlers", Doc: "Handle HTTP request"},
		{Type: "function", Name: "HandleResponse", Package: "handlers", Doc: "Handle HTTP response"},
		{Type: "struct", Name: "RequestConfig", Package: "config", Doc: "Configuration for requests"},
		{Type: "interface", Name: "Handler", Package: "handlers", Doc: "Handler interface"},
	}

	store.SaveFramework("test", "", "", elements)
	store.LoadFramework("test")

	// Search for "request"
	results := store.Search("request", 10)
	if len(results) == 0 {
		t.Fatal("expected some results for 'request'")
	}

	// Verify HandleRequest is in results (should have highest score)
	found := false
	for _, r := range results {
		if r.Name == "HandleRequest" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected HandleRequest in results")
	}

	// Search with limit
	results = store.Search("handler", 2)
	if len(results) > 2 {
		t.Errorf("expected max 2 results, got %d", len(results))
	}
}

func TestStore_SearchByType(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	elements := []indexer.CodeElement{
		{Type: "function", Name: "Func1", Package: "pkg"},
		{Type: "function", Name: "Func2", Package: "pkg"},
		{Type: "struct", Name: "Struct1", Package: "pkg"},
		{Type: "interface", Name: "Interface1", Package: "pkg"},
	}

	store.SaveFramework("test", "", "", elements)
	store.LoadFramework("test")

	// Search for functions
	results := store.SearchByType("function", 10)
	if len(results) != 2 {
		t.Errorf("expected 2 functions, got %d", len(results))
	}

	// Search for structs
	results = store.SearchByType("struct", 10)
	if len(results) != 1 {
		t.Errorf("expected 1 struct, got %d", len(results))
	}
}

func TestStore_GetPatterns(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	elements := []indexer.CodeElement{
		{Type: "function", Name: "NewClient", Package: "client"},
		{Type: "function", Name: "NewServer", Package: "server"},
		{Type: "function", Name: "GetUser", Package: "user"},
		{Type: "function", Name: "SetConfig", Package: "config"},
		{Type: "function", Name: "IsValid", Package: "validator"},
		{Type: "function", Name: "HasPermission", Package: "auth"},
		{Type: "function", Name: "RequestHandler", Package: "handlers"},
		{Type: "struct", Name: "UserService", Package: "user"},
		{Type: "struct", Name: "UserRepository", Package: "user"},
		{Type: "struct", Name: "ValidationError", Package: "errors"},
	}

	store.SaveFramework("test", "", "", elements)
	store.LoadFramework("test")

	patterns := store.GetPatterns()

	if len(patterns["constructor"]) != 2 {
		t.Errorf("expected 2 constructors (NewClient, NewServer), got %d", len(patterns["constructor"]))
	}
	if len(patterns["getter"]) != 1 {
		t.Errorf("expected 1 getter (GetUser), got %d", len(patterns["getter"]))
	}
	if len(patterns["setter"]) != 1 {
		t.Errorf("expected 1 setter (SetConfig), got %d", len(patterns["setter"]))
	}
	if len(patterns["predicate"]) != 2 {
		t.Errorf("expected 2 predicates (IsValid, HasPermission), got %d", len(patterns["predicate"]))
	}
	if len(patterns["handler"]) != 1 {
		t.Errorf("expected 1 handler, got %d", len(patterns["handler"]))
	}
	if len(patterns["service"]) != 1 {
		t.Errorf("expected 1 service, got %d", len(patterns["service"]))
	}
	if len(patterns["repository"]) != 1 {
		t.Errorf("expected 1 repository, got %d", len(patterns["repository"]))
	}
	if len(patterns["error"]) != 1 {
		t.Errorf("expected 1 error type, got %d", len(patterns["error"]))
	}
}

func TestSplitCamelCase(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"HandleRequest", []string{"Handle", "Request"}},
		{"getUserByID", []string{"get", "User", "By", "I", "D"}},
		{"URL", []string{"U", "R", "L"}},
		{"simple", []string{"simple"}},
		{"HTTPServer", []string{"H", "T", "T", "P", "Server"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := splitCamelCase(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
				return
			}
			for i, word := range result {
				if word != tt.expected[i] {
					t.Errorf("expected word[%d] = %q, got %q", i, tt.expected[i], word)
				}
			}
		})
	}
}

func TestStore_LoadFramework_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "knowledge-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	store := NewStore(tmpDir)

	_, err = store.LoadFramework("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent framework")
	}
}
