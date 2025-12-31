// Package atomicfile provides atomic file operations to prevent data corruption.
package atomicfile

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// Writer provides atomic file writing.
// It writes to a temporary file first, then renames to the target path.
type Writer struct {
	path     string
	tmpPath  string
	file     *os.File
	perm     os.FileMode
	mu       sync.Mutex
	closed   bool
}

// NewWriter creates a new atomic file writer.
func NewWriter(path string, perm os.FileMode) (*Writer, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create directory: %w", err)
	}

	// Create temporary file in same directory for atomic rename
	tmpPath := path + ".tmp"
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}

	return &Writer{
		path:    path,
		tmpPath: tmpPath,
		file:    file,
		perm:    perm,
	}, nil
}

// Write implements io.Writer.
func (w *Writer) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return 0, fmt.Errorf("writer is closed")
	}

	return w.file.Write(p)
}

// Close closes the writer and atomically renames the temp file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	// Sync to ensure data is written to disk
	if err := w.file.Sync(); err != nil {
		w.file.Close()
		os.Remove(w.tmpPath)
		return fmt.Errorf("sync: %w", err)
	}

	// Close the file
	if err := w.file.Close(); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("close: %w", err)
	}

	// Atomic rename
	if err := os.Rename(w.tmpPath, w.path); err != nil {
		os.Remove(w.tmpPath)
		return fmt.Errorf("rename: %w", err)
	}

	return nil
}

// Abort aborts the write and removes the temp file.
func (w *Writer) Abort() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return nil
	}
	w.closed = true

	w.file.Close()
	return os.Remove(w.tmpPath)
}

// WriteFile atomically writes data to a file.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	w, err := NewWriter(path, perm)
	if err != nil {
		return err
	}

	if _, err := w.Write(data); err != nil {
		w.Abort()
		return err
	}

	return w.Close()
}

// WriteJSON atomically writes JSON data to a file.
func WriteJSON(path string, v interface{}, perm os.FileMode) error {
	w, err := NewWriter(path, perm)
	if err != nil {
		return err
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(v); err != nil {
		w.Abort()
		return fmt.Errorf("encode json: %w", err)
	}

	return w.Close()
}

// ReadJSON reads JSON from a file.
func ReadJSON(path string, v interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(v)
}

// CopyFile atomically copies a file.
func CopyFile(src, dst string, perm os.FileMode) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer srcFile.Close()

	w, err := NewWriter(dst, perm)
	if err != nil {
		return err
	}

	if _, err := io.Copy(w, srcFile); err != nil {
		w.Abort()
		return fmt.Errorf("copy: %w", err)
	}

	return w.Close()
}

// SafeWriter provides a safe writer that can be committed or rolled back.
type SafeWriter struct {
	path     string
	backup   string
	hasBackup bool
	writer   *Writer
}

// NewSafeWriter creates a writer with automatic backup.
func NewSafeWriter(path string, perm os.FileMode) (*SafeWriter, error) {
	sw := &SafeWriter{
		path: path,
	}

	// Create backup if file exists
	if _, err := os.Stat(path); err == nil {
		sw.backup = path + ".bak"
		if err := CopyFile(path, sw.backup, perm); err != nil {
			return nil, fmt.Errorf("create backup: %w", err)
		}
		sw.hasBackup = true
	}

	// Create writer
	writer, err := NewWriter(path, perm)
	if err != nil {
		if sw.hasBackup {
			os.Remove(sw.backup)
		}
		return nil, err
	}
	sw.writer = writer

	return sw, nil
}

// Write implements io.Writer.
func (sw *SafeWriter) Write(p []byte) (n int, err error) {
	return sw.writer.Write(p)
}

// Commit commits the write and removes backup.
func (sw *SafeWriter) Commit() error {
	if err := sw.writer.Close(); err != nil {
		return err
	}

	// Remove backup on success
	if sw.hasBackup {
		os.Remove(sw.backup)
	}

	return nil
}

// Rollback aborts the write and restores backup.
func (sw *SafeWriter) Rollback() error {
	sw.writer.Abort()

	// Restore backup if exists
	if sw.hasBackup {
		if err := os.Rename(sw.backup, sw.path); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
	}

	return nil
}

// FileLocker provides file-based locking.
type FileLocker struct {
	path string
	file *os.File
}

// Lock acquires a lock on the file.
func Lock(path string) (*FileLocker, error) {
	lockPath := path + ".lock"

	// Try to create lock file exclusively
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		if os.IsExist(err) {
			return nil, fmt.Errorf("file is locked: %s", path)
		}
		return nil, err
	}

	// Write PID to lock file
	fmt.Fprintf(file, "%d", os.Getpid())

	return &FileLocker{
		path: lockPath,
		file: file,
	}, nil
}

// Unlock releases the lock.
func (l *FileLocker) Unlock() error {
	if l.file != nil {
		l.file.Close()
	}
	return os.Remove(l.path)
}

// IsLocked checks if a file is locked.
func IsLocked(path string) bool {
	lockPath := path + ".lock"
	_, err := os.Stat(lockPath)
	return err == nil
}
