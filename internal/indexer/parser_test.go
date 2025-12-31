package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewParser(t *testing.T) {
	parser := NewParser()
	if parser == nil {
		t.Fatal("expected non-nil parser")
	}
	if parser.fset == nil {
		t.Fatal("expected non-nil file set")
	}
}

func TestParser_ParseFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a sample Go file
	sampleCode := `package sample

import (
	"fmt"
	"net/http"
)

// Config holds configuration
type Config struct {
	Host string
	Port int
}

// Handler interface for request handling
type Handler interface {
	Handle(w http.ResponseWriter, r *http.Request)
}

// maxRetries is the maximum number of retries
const maxRetries = 3

// defaultTimeout in seconds
var defaultTimeout = 30

// NewConfig creates a new Config
func NewConfig(host string, port int) *Config {
	return &Config{Host: host, Port: port}
}

// String returns string representation
func (c *Config) String() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
`

	filePath := filepath.Join(tmpDir, "sample.go")
	if err := os.WriteFile(filePath, []byte(sampleCode), 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	parser := NewParser()
	elements, err := parser.ParseFile(filePath, tmpDir)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	// Check we got all elements
	expectedElements := map[string]string{
		"Config":         "struct",
		"Handler":        "interface",
		"maxRetries":     "const",
		"defaultTimeout": "var",
		"NewConfig":      "function",
		"String":         "method",
	}

	foundElements := make(map[string]string)
	for _, elem := range elements {
		foundElements[elem.Name] = elem.Type
	}

	for name, expectedType := range expectedElements {
		if foundType, ok := foundElements[name]; !ok {
			t.Errorf("missing element %q", name)
		} else if foundType != expectedType {
			t.Errorf("element %q: expected type %q, got %q", name, expectedType, foundType)
		}
	}
}

func TestParser_ParseFile_FunctionDetails(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sampleCode := `package sample

// Add adds two integers and returns the result.
func Add(a int, b int) int {
	return a + b
}
`

	filePath := filepath.Join(tmpDir, "math.go")
	if err := os.WriteFile(filePath, []byte(sampleCode), 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	parser := NewParser()
	elements, err := parser.ParseFile(filePath, tmpDir)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(elements) != 1 {
		t.Fatalf("expected 1 element, got %d", len(elements))
	}

	fn := elements[0]
	if fn.Name != "Add" {
		t.Errorf("expected name 'Add', got %q", fn.Name)
	}
	if fn.Type != "function" {
		t.Errorf("expected type 'function', got %q", fn.Type)
	}
	if fn.Package != "sample" {
		t.Errorf("expected package 'sample', got %q", fn.Package)
	}
	if fn.Line != 4 {
		t.Errorf("expected line 4, got %d", fn.Line)
	}
	if fn.Doc == "" {
		t.Error("expected non-empty documentation")
	}
	if fn.Body == "" {
		t.Error("expected non-empty body")
	}
	if fn.Signature == "" {
		t.Error("expected non-empty signature")
	}
}

func TestParser_ParseFile_Method(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sampleCode := `package sample

type Counter struct {
	value int
}

func (c *Counter) Increment() {
	c.value++
}

func (c Counter) Value() int {
	return c.value
}
`

	filePath := filepath.Join(tmpDir, "counter.go")
	if err := os.WriteFile(filePath, []byte(sampleCode), 0644); err != nil {
		t.Fatalf("failed to write sample file: %v", err)
	}

	parser := NewParser()
	elements, err := parser.ParseFile(filePath, tmpDir)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	methods := make(map[string]CodeElement)
	for _, elem := range elements {
		if elem.Type == "method" {
			methods[elem.Name] = elem
		}
	}

	if len(methods) != 2 {
		t.Fatalf("expected 2 methods, got %d", len(methods))
	}

	increment, ok := methods["Increment"]
	if !ok {
		t.Fatal("missing Increment method")
	}
	if increment.Receiver != "*Counter" {
		t.Errorf("expected receiver '*Counter', got %q", increment.Receiver)
	}

	value, ok := methods["Value"]
	if !ok {
		t.Fatal("missing Value method")
	}
	if value.Receiver != "Counter" {
		t.Errorf("expected receiver 'Counter', got %q", value.Receiver)
	}
}

func TestParser_ParseDirectory(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create subdirectory structure
	subDir := filepath.Join(tmpDir, "pkg")
	os.MkdirAll(subDir, 0755)

	// Create files in root
	file1 := `package main

func Main() {}
`
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(file1), 0644)

	// Create files in subdirectory
	file2 := `package pkg

func Helper() {}
`
	os.WriteFile(filepath.Join(subDir, "helper.go"), []byte(file2), 0644)

	// Create test file (should be skipped)
	testFile := `package pkg

func TestHelper() {}
`
	os.WriteFile(filepath.Join(subDir, "helper_test.go"), []byte(testFile), 0644)

	parser := NewParser()
	elements, err := parser.ParseDirectory(tmpDir)
	if err != nil {
		t.Fatalf("ParseDirectory failed: %v", err)
	}

	// Should have Main and Helper, but not TestHelper
	names := make(map[string]bool)
	for _, elem := range elements {
		names[elem.Name] = true
	}

	if !names["Main"] {
		t.Error("missing Main function")
	}
	if !names["Helper"] {
		t.Error("missing Helper function")
	}
	if names["TestHelper"] {
		t.Error("TestHelper should have been skipped (test file)")
	}
}

func TestParser_ParseDirectory_SkipsVendor(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create vendor directory
	vendorDir := filepath.Join(tmpDir, "vendor", "somelib")
	os.MkdirAll(vendorDir, 0755)

	vendorFile := `package somelib

func VendorFunc() {}
`
	os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte(vendorFile), 0644)

	// Create regular file
	mainFile := `package main

func Main() {}
`
	os.WriteFile(filepath.Join(tmpDir, "main.go"), []byte(mainFile), 0644)

	parser := NewParser()
	elements, err := parser.ParseDirectory(tmpDir)
	if err != nil {
		t.Fatalf("ParseDirectory failed: %v", err)
	}

	for _, elem := range elements {
		if elem.Name == "VendorFunc" {
			t.Error("VendorFunc should have been skipped (vendor directory)")
		}
	}
}

func TestParser_ParseFile_InvalidSyntax(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	invalidCode := `package sample

func Broken( {
`

	filePath := filepath.Join(tmpDir, "invalid.go")
	os.WriteFile(filePath, []byte(invalidCode), 0644)

	parser := NewParser()
	_, err = parser.ParseFile(filePath, tmpDir)
	if err == nil {
		t.Error("expected error for invalid syntax")
	}
}

func TestParser_ParseFile_Imports(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "parser-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	sampleCode := `package sample

import (
	"fmt"
	"net/http"
	"encoding/json"
)

func Process() {}
`

	filePath := filepath.Join(tmpDir, "process.go")
	os.WriteFile(filePath, []byte(sampleCode), 0644)

	parser := NewParser()
	elements, err := parser.ParseFile(filePath, tmpDir)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if len(elements) == 0 {
		t.Fatal("expected at least one element")
	}

	fn := elements[0]
	if len(fn.Imports) != 3 {
		t.Errorf("expected 3 imports, got %d", len(fn.Imports))
	}

	expectedImports := map[string]bool{
		"fmt":           true,
		"net/http":      true,
		"encoding/json": true,
	}

	for _, imp := range fn.Imports {
		if !expectedImports[imp] {
			t.Errorf("unexpected import: %q", imp)
		}
	}
}

func TestCodeElement_Fields(t *testing.T) {
	elem := CodeElement{
		Type:       "function",
		Name:       "TestFunc",
		Package:    "testpkg",
		FilePath:   "test.go",
		Line:       42,
		Signature:  "func TestFunc() error",
		Doc:        "TestFunc is a test function",
		Body:       "func TestFunc() error { return nil }",
		Receiver:   "",
		Imports:    []string{"fmt"},
		References: []string{"error"},
	}

	if elem.Type != "function" {
		t.Errorf("expected type 'function', got %q", elem.Type)
	}
	if elem.Name != "TestFunc" {
		t.Errorf("expected name 'TestFunc', got %q", elem.Name)
	}
	if elem.Line != 42 {
		t.Errorf("expected line 42, got %d", elem.Line)
	}
}
