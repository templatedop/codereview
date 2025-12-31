package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEmbeddingCache_GetSet(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	key := "test-key"
	embedding := []float32{0.1, 0.2, 0.3}

	// Get non-existent key
	result := cache.Get(key)
	if result != nil {
		t.Errorf("Get() on non-existent key = %v, want nil", result)
	}

	// Set and get
	cache.Set(key, embedding)
	result = cache.Get(key)
	if result == nil {
		t.Fatal("Get() after Set() = nil, want embedding")
	}

	if len(result) != len(embedding) {
		t.Errorf("Get() returned %d elements, want %d", len(result), len(embedding))
	}

	for i, v := range result {
		if v != embedding[i] {
			t.Errorf("Get()[%d] = %f, want %f", i, v, embedding[i])
		}
	}
}

func TestEmbeddingCache_TTL(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond,
	})

	key := "ttl-test"
	embedding := []float32{0.1, 0.2}

	cache.Set(key, embedding)

	// Should be available immediately
	if cache.Get(key) == nil {
		t.Error("Get() immediately after Set() = nil")
	}

	// Wait for TTL to expire
	time.Sleep(60 * time.Millisecond)

	// Should be expired now
	if cache.Get(key) != nil {
		t.Error("Get() after TTL expiry should be nil")
	}
}

func TestEmbeddingCache_Eviction(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 3,
		TTL:     time.Hour,
	})

	// Add 3 entries
	cache.Set("key1", []float32{0.1})
	time.Sleep(10 * time.Millisecond)
	cache.Set("key2", []float32{0.2})
	time.Sleep(10 * time.Millisecond)
	cache.Set("key3", []float32{0.3})

	if cache.Size() != 3 {
		t.Errorf("Size() = %d, want 3", cache.Size())
	}

	// Add 4th entry - should evict oldest (key1)
	cache.Set("key4", []float32{0.4})

	if cache.Size() != 3 {
		t.Errorf("Size() after eviction = %d, want 3", cache.Size())
	}

	// key1 should be evicted
	if cache.Get("key1") != nil {
		t.Error("key1 should have been evicted")
	}

	// key4 should exist
	if cache.Get("key4") == nil {
		t.Error("key4 should exist")
	}
}

func TestEmbeddingCache_Prune(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond,
	})

	cache.Set("key1", []float32{0.1})
	cache.Set("key2", []float32{0.2})

	// Wait for entries to expire
	time.Sleep(60 * time.Millisecond)

	pruned := cache.Prune()
	if pruned != 2 {
		t.Errorf("Prune() = %d, want 2", pruned)
	}

	if cache.Size() != 0 {
		t.Errorf("Size() after Prune() = %d, want 0", cache.Size())
	}
}

func TestEmbeddingCache_Clear(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cache.Set("key1", []float32{0.1})
	cache.Set("key2", []float32{0.2})

	cache.Clear()

	if cache.Size() != 0 {
		t.Errorf("Size() after Clear() = %d, want 0", cache.Size())
	}
}

func TestEmbeddingCache_Stats(t *testing.T) {
	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cache.Set("key1", []float32{0.1})
	time.Sleep(10 * time.Millisecond)
	cache.Set("key2", []float32{0.2})

	stats := cache.Stats()

	if stats.Size != 2 {
		t.Errorf("Stats.Size = %d, want 2", stats.Size)
	}
	if stats.MaxSize != 100 {
		t.Errorf("Stats.MaxSize = %d, want 100", stats.MaxSize)
	}
	if stats.TTL != time.Hour {
		t.Errorf("Stats.TTL = %v, want 1h", stats.TTL)
	}
	if stats.OldestAge < stats.NewestAge {
		t.Error("OldestAge should be >= NewestAge")
	}
}

func TestEmbeddingCache_Persistence(t *testing.T) {
	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "cache.json")

	// Create cache with persistence
	cache1 := NewEmbeddingCache(CacheConfig{
		MaxSize:   100,
		TTL:       time.Hour,
		StorePath: storePath,
	})

	cache1.Set("key1", []float32{0.1, 0.2, 0.3})
	cache1.Set("key2", []float32{0.4, 0.5, 0.6})

	// Wait for async save
	time.Sleep(100 * time.Millisecond)

	// Verify file exists
	if _, err := os.Stat(storePath); os.IsNotExist(err) {
		t.Fatal("Cache file was not created")
	}

	// Create new cache that loads from disk
	cache2 := NewEmbeddingCache(CacheConfig{
		MaxSize:   100,
		TTL:       time.Hour,
		StorePath: storePath,
	})

	if cache2.Size() != 2 {
		t.Errorf("Loaded cache Size() = %d, want 2", cache2.Size())
	}

	embedding := cache2.Get("key1")
	if embedding == nil {
		t.Fatal("Loaded cache missing key1")
	}

	expected := []float32{0.1, 0.2, 0.3}
	for i, v := range embedding {
		if v != expected[i] {
			t.Errorf("Loaded embedding[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestCachedEmbedder_Embed(t *testing.T) {
	callCount := 0
	mockEmbedder := &mockEmbedderForCache{
		embedFn: func(ctx context.Context, text string) ([]float32, error) {
			callCount++
			return []float32{0.1, 0.2, 0.3}, nil
		},
		dimension: 3,
	}

	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cachedEmbedder := NewCachedEmbedder(mockEmbedder, cache)

	// First call should hit the embedder
	result1, err := cachedEmbedder.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("First call: embedder called %d times, want 1", callCount)
	}

	// Second call should use cache
	result2, err := cachedEmbedder.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("Second call: embedder called %d times, want 1 (cached)", callCount)
	}

	// Results should be equal
	for i := range result1 {
		if result1[i] != result2[i] {
			t.Errorf("Cached result differs at index %d", i)
		}
	}

	// Different text should call embedder again
	_, err = cachedEmbedder.Embed(context.Background(), "different text")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if callCount != 2 {
		t.Errorf("Third call with new text: embedder called %d times, want 2", callCount)
	}
}

func TestCachedEmbedder_EmbedBatch(t *testing.T) {
	callCount := 0
	mockEmbedder := &mockEmbedderForCache{
		embedBatchFn: func(ctx context.Context, texts []string) ([][]float32, error) {
			callCount++
			result := make([][]float32, len(texts))
			for i := range texts {
				result[i] = []float32{float32(i) * 0.1, float32(i) * 0.2}
			}
			return result, nil
		},
		dimension: 2,
	}

	cache := NewEmbeddingCache(CacheConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	})

	cachedEmbedder := NewCachedEmbedder(mockEmbedder, cache)

	// First batch call
	texts := []string{"text1", "text2", "text3"}
	results, err := cachedEmbedder.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("EmbedBatch() returned %d results, want 3", len(results))
	}
	if callCount != 1 {
		t.Errorf("First batch: embedder called %d times, want 1", callCount)
	}

	// Same batch should use cache entirely
	_, err = cachedEmbedder.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if callCount != 1 {
		t.Errorf("Cached batch: embedder called %d times, want 1", callCount)
	}

	// Partial overlap should only call for uncached texts
	partialTexts := []string{"text1", "text4"}
	_, err = cachedEmbedder.EmbedBatch(context.Background(), partialTexts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if callCount != 2 {
		t.Errorf("Partial batch: embedder called %d times, want 2", callCount)
	}
}

func TestCachedEmbedder_Dimension(t *testing.T) {
	mockEmbedder := &mockEmbedderForCache{
		dimension: 768,
	}

	cache := NewEmbeddingCache(DefaultCacheConfig())
	cachedEmbedder := NewCachedEmbedder(mockEmbedder, cache)

	if cachedEmbedder.Dimension() != 768 {
		t.Errorf("Dimension() = %d, want 768", cachedEmbedder.Dimension())
	}
}

func TestDefaultCacheConfig(t *testing.T) {
	cfg := DefaultCacheConfig()

	if cfg.MaxSize != 10000 {
		t.Errorf("MaxSize = %d, want 10000", cfg.MaxSize)
	}
	if cfg.TTL != 24*time.Hour {
		t.Errorf("TTL = %v, want 24h", cfg.TTL)
	}
	if cfg.StorePath != "" {
		t.Errorf("StorePath = %q, want empty", cfg.StorePath)
	}
}

func TestHashText(t *testing.T) {
	// Same text should produce same hash
	hash1 := hashText("test text")
	hash2 := hashText("test text")
	if hash1 != hash2 {
		t.Error("hashText() should be deterministic")
	}

	// Different text should produce different hash
	hash3 := hashText("different text")
	if hash1 == hash3 {
		t.Error("hashText() should produce different hashes for different texts")
	}

	// Hash should be hex encoded SHA256 (64 chars)
	if len(hash1) != 64 {
		t.Errorf("hashText() length = %d, want 64", len(hash1))
	}
}

// mockEmbedderForCache is a mock embedder for cache tests
type mockEmbedderForCache struct {
	embedFn      func(ctx context.Context, text string) ([]float32, error)
	embedBatchFn func(ctx context.Context, texts []string) ([][]float32, error)
	dimension    int
}

func (m *mockEmbedderForCache) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.embedFn != nil {
		return m.embedFn(ctx, text)
	}
	return []float32{0.1}, nil
}

func (m *mockEmbedderForCache) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if m.embedBatchFn != nil {
		return m.embedBatchFn(ctx, texts)
	}
	result := make([][]float32, len(texts))
	for i := range texts {
		result[i] = []float32{0.1}
	}
	return result, nil
}

func (m *mockEmbedderForCache) Dimension() int {
	return m.dimension
}
