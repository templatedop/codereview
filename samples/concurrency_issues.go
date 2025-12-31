package samples

import (
	"net/http"
	"os"
	"sync"
)

// VULNERABILITY: Race condition - shared state without synchronization
var (
	counter    int
	cache      = make(map[string]string)
	userTokens = make(map[int]string)
)

// IncrementCounter has a race condition
func IncrementCounter() {
	// BAD: No mutex protection
	counter++
}

// GetCounter reads shared state unsafely
func GetCounter() int {
	// BAD: Reading shared state without lock
	return counter
}

// SetCache writes to shared map without synchronization
func SetCache(key, value string) {
	// BAD: Concurrent map write can cause panic
	cache[key] = value
}

// GetCache reads from shared map without synchronization
func GetCache(key string) string {
	// BAD: Concurrent map read while write happens
	return cache[key]
}

// RequestHandler has race condition on shared state
func RequestHandler(w http.ResponseWriter, r *http.Request) {
	// BAD: Multiple goroutines accessing shared map
	userID := 123
	userTokens[userID] = r.Header.Get("Authorization")
}

// Counter with mutex but used incorrectly
type SafeCounter struct {
	mu    sync.Mutex
	count int
}

// BAD: Lock not held while reading
func (c *SafeCounter) UnsafeRead() int {
	return c.count // Should use c.mu.Lock()
}

// ResourceLeak doesn't close file
func ResourceLeak(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// BAD: file.Close() never called - resource leak!

	buf := make([]byte, 1024)
	n, _ := file.Read(buf)
	return buf[:n], nil
}

// AnotherLeak opens HTTP response but doesn't close body
func AnotherLeak(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	// BAD: resp.Body.Close() never called

	buf := make([]byte, 1024)
	resp.Body.Read(buf)
	return string(buf), nil
}
