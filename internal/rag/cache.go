package rag

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CachedEmbedder wraps an Embedder with caching capabilities.
type CachedEmbedder struct {
	embedder  Embedder
	cache     *EmbeddingCache
	dimension int
}

// EmbeddingCache provides caching for embeddings.
type EmbeddingCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	maxSize   int
	ttl       time.Duration
	storePath string
}

type cacheEntry struct {
	Embedding []float32 `json:"embedding"`
	CreatedAt time.Time `json:"created_at"`
}

// CacheConfig holds configuration for the embedding cache.
type CacheConfig struct {
	// MaxSize is the maximum number of entries in the cache.
	MaxSize int
	// TTL is the time-to-live for cache entries.
	TTL time.Duration
	// StorePath is the path to persist the cache (optional).
	StorePath string
}

// DefaultCacheConfig returns sensible defaults.
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxSize: 10000,
		TTL:     24 * time.Hour,
	}
}

// NewEmbeddingCache creates a new embedding cache.
func NewEmbeddingCache(cfg CacheConfig) *EmbeddingCache {
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 10000
	}
	if cfg.TTL == 0 {
		cfg.TTL = 24 * time.Hour
	}

	cache := &EmbeddingCache{
		entries:   make(map[string]*cacheEntry),
		maxSize:   cfg.MaxSize,
		ttl:       cfg.TTL,
		storePath: cfg.StorePath,
	}

	if cfg.StorePath != "" {
		_ = cache.loadFromDisk()
	}

	return cache
}

// NewCachedEmbedder creates a new cached embedder.
func NewCachedEmbedder(embedder Embedder, cache *EmbeddingCache) *CachedEmbedder {
	return &CachedEmbedder{
		embedder:  embedder,
		cache:     cache,
		dimension: embedder.Dimension(),
	}
}

// Embed generates or retrieves a cached embedding.
func (e *CachedEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	key := hashText(text)

	// Check cache
	if embedding := e.cache.Get(key); embedding != nil {
		return embedding, nil
	}

	// Generate embedding
	embedding, err := e.embedder.Embed(ctx, text)
	if err != nil {
		return nil, err
	}

	// Store in cache
	e.cache.Set(key, embedding)

	return embedding, nil
}

// EmbedBatch generates or retrieves cached embeddings for multiple texts.
func (e *CachedEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	var uncachedTexts []string
	var uncachedIndices []int

	// Check cache for each text
	for i, text := range texts {
		key := hashText(text)
		if embedding := e.cache.Get(key); embedding != nil {
			results[i] = embedding
		} else {
			uncachedTexts = append(uncachedTexts, text)
			uncachedIndices = append(uncachedIndices, i)
		}
	}

	// If all cached, return
	if len(uncachedTexts) == 0 {
		return results, nil
	}

	// Generate embeddings for uncached texts
	embeddings, err := e.embedder.EmbedBatch(ctx, uncachedTexts)
	if err != nil {
		return nil, err
	}

	// Store in cache and results
	for i, embedding := range embeddings {
		idx := uncachedIndices[i]
		results[idx] = embedding
		key := hashText(uncachedTexts[i])
		e.cache.Set(key, embedding)
	}

	return results, nil
}

// Dimension returns the embedding dimension.
func (e *CachedEmbedder) Dimension() int {
	return e.dimension
}

// Get retrieves an embedding from the cache.
func (c *EmbeddingCache) Get(key string) []float32 {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key]
	if !ok {
		return nil
	}

	// Check TTL
	if time.Since(entry.CreatedAt) > c.ttl {
		return nil
	}

	return entry.Embedding
}

// Set stores an embedding in the cache.
func (c *EmbeddingCache) Set(key string, embedding []float32) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		Embedding: embedding,
		CreatedAt: time.Now(),
	}

	// Persist if store path is set
	if c.storePath != "" {
		go c.saveToDisk()
	}
}

// Size returns the number of entries in the cache.
func (c *EmbeddingCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Clear removes all entries from the cache.
func (c *EmbeddingCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*cacheEntry)
}

// Prune removes expired entries from the cache.
func (c *EmbeddingCache) Prune() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	pruned := 0
	now := time.Now()
	for key, entry := range c.entries {
		if now.Sub(entry.CreatedAt) > c.ttl {
			delete(c.entries, key)
			pruned++
		}
	}

	return pruned
}

func (c *EmbeddingCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

func (c *EmbeddingCache) saveToDisk() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if err := os.MkdirAll(filepath.Dir(c.storePath), 0755); err != nil {
		return
	}

	data, err := json.Marshal(c.entries)
	if err != nil {
		return
	}

	_ = os.WriteFile(c.storePath, data, 0644)
}

func (c *EmbeddingCache) loadFromDisk() error {
	data, err := os.ReadFile(c.storePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var entries map[string]*cacheEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return err
	}

	c.entries = entries
	return nil
}

// hashText creates a deterministic hash of text for cache keys.
func hashText(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}

// CacheStats provides statistics about the cache.
type CacheStats struct {
	Size       int           `json:"size"`
	MaxSize    int           `json:"max_size"`
	TTL        time.Duration `json:"ttl"`
	OldestAge  time.Duration `json:"oldest_age"`
	NewestAge  time.Duration `json:"newest_age"`
}

// Stats returns statistics about the cache.
func (c *EmbeddingCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := CacheStats{
		Size:    len(c.entries),
		MaxSize: c.maxSize,
		TTL:     c.ttl,
	}

	now := time.Now()
	for _, entry := range c.entries {
		age := now.Sub(entry.CreatedAt)
		if stats.OldestAge == 0 || age > stats.OldestAge {
			stats.OldestAge = age
		}
		if stats.NewestAge == 0 || age < stats.NewestAge {
			stats.NewestAge = age
		}
	}

	return stats
}
