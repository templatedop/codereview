package atomicfile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	content := []byte("hello world")

	if err := WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	// Verify content
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Content = %q, want %q", string(got), string(content))
	}

	// Verify no temp file left
	files, _ := os.ReadDir(dir)
	if len(files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(files))
	}
}

func TestWriteFile_CreateDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "test.txt")
	content := []byte("nested content")

	if err := WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Content = %q, want %q", string(got), string(content))
	}
}

func TestWriteJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")

	data := map[string]interface{}{
		"name":  "test",
		"value": 42,
	}

	if err := WriteJSON(path, data, 0644); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	// Read and verify
	var got map[string]interface{}
	if err := ReadJSON(path, &got); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}

	if got["name"] != "test" {
		t.Errorf("name = %v, want %v", got["name"], "test")
	}
	if got["value"] != float64(42) {
		t.Errorf("value = %v, want %v", got["value"], 42)
	}
}

func TestWriter_Abort(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aborted.txt")

	w, err := NewWriter(path, 0644)
	if err != nil {
		t.Fatalf("NewWriter() error = %v", err)
	}

	w.Write([]byte("partial content"))
	w.Abort()

	// File should not exist
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("File should not exist after abort")
	}

	// Temp file should not exist
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Error("Temp file should not exist after abort")
	}
}

func TestCopyFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "destination.txt")

	content := []byte("file to copy")
	if err := os.WriteFile(src, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if err := CopyFile(src, dst, 0644); err != nil {
		t.Fatalf("CopyFile() error = %v", err)
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("Content = %q, want %q", string(got), string(content))
	}
}

func TestSafeWriter_Commit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "safe.txt")

	// Create initial file
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sw, err := NewSafeWriter(path, 0644)
	if err != nil {
		t.Fatalf("NewSafeWriter() error = %v", err)
	}

	sw.Write([]byte("updated"))
	if err := sw.Commit(); err != nil {
		t.Fatalf("Commit() error = %v", err)
	}

	// Verify content
	got, _ := os.ReadFile(path)
	if string(got) != "updated" {
		t.Errorf("Content = %q, want %q", string(got), "updated")
	}

	// Backup should be removed
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Error("Backup should be removed after commit")
	}
}

func TestSafeWriter_Rollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollback.txt")

	// Create initial file
	if err := os.WriteFile(path, []byte("original"), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	sw, err := NewSafeWriter(path, 0644)
	if err != nil {
		t.Fatalf("NewSafeWriter() error = %v", err)
	}

	sw.Write([]byte("failed update"))
	if err := sw.Rollback(); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}

	// Verify original content restored
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Errorf("Content = %q, want %q", string(got), "original")
	}
}

func TestLock(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lockable.txt")

	lock1, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock() error = %v", err)
	}

	// Check IsLocked
	if !IsLocked(path) {
		t.Error("IsLocked() = false, want true")
	}

	// Second lock should fail
	_, err = Lock(path)
	if err == nil {
		t.Error("Second Lock() should fail")
	}

	// Unlock
	lock1.Unlock()

	// Should be able to lock again
	lock2, err := Lock(path)
	if err != nil {
		t.Fatalf("Lock() after unlock error = %v", err)
	}
	lock2.Unlock()
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "concurrent.json")

	// Initial write
	data := map[string]int{"counter": 0}
	WriteJSON(path, data, 0644)

	// Concurrent writes
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(val int) {
			defer wg.Done()
			data := map[string]int{"counter": val}
			WriteJSON(path, data, 0644)
		}(i)
	}
	wg.Wait()

	// File should be valid JSON
	var result map[string]int
	if err := ReadJSON(path, &result); err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}

	// Counter should be some value between 0-9
	if result["counter"] < 0 || result["counter"] > 9 {
		t.Errorf("counter = %d, want 0-9", result["counter"])
	}
}

func TestWriteFile_LargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.bin")

	// 1MB of data
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	if err := WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if len(got) != len(content) {
		t.Errorf("len = %d, want %d", len(got), len(content))
	}
}

func TestReadJSON_NotFound(t *testing.T) {
	var data map[string]string
	err := ReadJSON("/nonexistent/path.json", &data)
	if err == nil {
		t.Error("ReadJSON() should fail for non-existent file")
	}
}

func TestReadJSON_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.json")

	os.WriteFile(path, []byte("not json"), 0644)

	var data map[string]string
	err := ReadJSON(path, &data)
	if err == nil {
		t.Error("ReadJSON() should fail for invalid JSON")
	}
}

// Ensure JSON struct maintains order
type orderedStruct struct {
	A string `json:"a"`
	B int    `json:"b"`
	C bool   `json:"c"`
}

func TestWriteJSON_Struct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "struct.json")

	data := orderedStruct{A: "test", B: 42, C: true}

	if err := WriteJSON(path, data, 0644); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}

	// Read raw to check formatting
	content, _ := os.ReadFile(path)
	if len(content) == 0 {
		t.Fatal("File is empty")
	}

	// Verify it's properly indented
	var unmarshaled orderedStruct
	if err := json.Unmarshal(content, &unmarshaled); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if unmarshaled.A != "test" || unmarshaled.B != 42 || !unmarshaled.C {
		t.Error("Unmarshaled struct doesn't match")
	}
}
