package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Document represents a document stored in the vector store.
type Document struct {
	ID         string            `json:"id"`
	Content    string            `json:"content"`
	Embedding  []float32         `json:"embedding,omitempty"`
	Source     string            `json:"source"`
	Category   string            `json:"category"`
	Metadata   map[string]string `json:"metadata"`
	ChunkIndex int               `json:"chunk_index"`
}

// SearchResult represents a search result with similarity score.
type SearchResult struct {
	Document   Document `json:"document"`
	Score      float32  `json:"score"`
	Collection string   `json:"collection"`
}

// VectorStore provides vector similarity search capabilities.
type VectorStore interface {
	// Insert adds a document to the specified collection.
	Insert(ctx context.Context, collection string, doc Document) error
	// InsertBatch adds multiple documents to the specified collection.
	InsertBatch(ctx context.Context, collection string, docs []Document) error
	// Search finds similar documents in the specified collection.
	Search(ctx context.Context, collection string, query []float32, topK int) ([]SearchResult, error)
	// SearchMultiple searches across multiple collections.
	SearchMultiple(ctx context.Context, collections []string, query []float32, topK int) ([]SearchResult, error)
	// Delete removes a document by ID.
	Delete(ctx context.Context, collection, id string) error
	// ListCollections returns all collection names.
	ListCollections(ctx context.Context) ([]string, error)
	// CollectionSize returns the number of documents in a collection.
	CollectionSize(ctx context.Context, collection string) (int, error)
}

// MemoryVectorStore implements VectorStore with in-memory storage.
// Suitable for development and small-scale deployments.
type MemoryVectorStore struct {
	mu          sync.RWMutex
	collections map[string][]Document
	storagePath string
}

// NewMemoryVectorStore creates a new in-memory vector store.
func NewMemoryVectorStore(storagePath string) *MemoryVectorStore {
	store := &MemoryVectorStore{
		collections: make(map[string][]Document),
		storagePath: storagePath,
	}
	if storagePath != "" {
		_ = store.loadFromDisk()
	}
	return store
}

// Insert adds a document to the specified collection.
func (s *MemoryVectorStore) Insert(ctx context.Context, collection string, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.collections[collection] == nil {
		s.collections[collection] = make([]Document, 0)
	}

	// Check for duplicate
	for i, existing := range s.collections[collection] {
		if existing.ID == doc.ID {
			s.collections[collection][i] = doc
			return s.saveToDisk()
		}
	}

	s.collections[collection] = append(s.collections[collection], doc)
	return s.saveToDisk()
}

// InsertBatch adds multiple documents to the specified collection.
func (s *MemoryVectorStore) InsertBatch(ctx context.Context, collection string, docs []Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.collections[collection] == nil {
		s.collections[collection] = make([]Document, 0)
	}

	// Create a map of existing IDs for quick lookup
	existingIDs := make(map[string]int)
	for i, doc := range s.collections[collection] {
		existingIDs[doc.ID] = i
	}

	for _, doc := range docs {
		if idx, exists := existingIDs[doc.ID]; exists {
			s.collections[collection][idx] = doc
		} else {
			s.collections[collection] = append(s.collections[collection], doc)
			existingIDs[doc.ID] = len(s.collections[collection]) - 1
		}
	}

	return s.saveToDisk()
}

// Search finds similar documents using cosine similarity.
func (s *MemoryVectorStore) Search(ctx context.Context, collection string, query []float32, topK int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	docs, ok := s.collections[collection]
	if !ok || len(docs) == 0 {
		return nil, nil
	}

	results := make([]SearchResult, 0, len(docs))
	for _, doc := range docs {
		if len(doc.Embedding) == 0 {
			continue
		}
		score := cosineSimilarity(query, doc.Embedding)
		results = append(results, SearchResult{
			Document:   doc,
			Score:      score,
			Collection: collection,
		})
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// SearchMultiple searches across multiple collections.
func (s *MemoryVectorStore) SearchMultiple(ctx context.Context, collections []string, query []float32, topK int) ([]SearchResult, error) {
	var allResults []SearchResult

	for _, collection := range collections {
		results, err := s.Search(ctx, collection, query, 0)
		if err != nil {
			return nil, fmt.Errorf("search collection %s: %w", collection, err)
		}
		allResults = append(allResults, results...)
	}

	// Sort all results by score
	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if topK > 0 && len(allResults) > topK {
		allResults = allResults[:topK]
	}

	return allResults, nil
}

// Delete removes a document by ID.
func (s *MemoryVectorStore) Delete(ctx context.Context, collection, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	docs, ok := s.collections[collection]
	if !ok {
		return nil
	}

	for i, doc := range docs {
		if doc.ID == id {
			s.collections[collection] = append(docs[:i], docs[i+1:]...)
			return s.saveToDisk()
		}
	}

	return nil
}

// ListCollections returns all collection names.
func (s *MemoryVectorStore) ListCollections(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	names := make([]string, 0, len(s.collections))
	for name := range s.collections {
		names = append(names, name)
	}
	return names, nil
}

// CollectionSize returns the number of documents in a collection.
func (s *MemoryVectorStore) CollectionSize(ctx context.Context, collection string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.collections[collection]), nil
}

func (s *MemoryVectorStore) saveToDisk() error {
	if s.storagePath == "" {
		return nil
	}

	if err := os.MkdirAll(s.storagePath, 0755); err != nil {
		return fmt.Errorf("create storage dir: %w", err)
	}

	for name, docs := range s.collections {
		path := filepath.Join(s.storagePath, name+".json")
		data, err := json.MarshalIndent(docs, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal collection %s: %w", name, err)
		}
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("write collection %s: %w", name, err)
		}
	}

	return nil
}

func (s *MemoryVectorStore) loadFromDisk() error {
	if s.storagePath == "" {
		return nil
	}

	entries, err := os.ReadDir(s.storagePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read storage dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		name := entry.Name()[:len(entry.Name())-5]
		path := filepath.Join(s.storagePath, entry.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read collection %s: %w", name, err)
		}

		var docs []Document
		if err := json.Unmarshal(data, &docs); err != nil {
			return fmt.Errorf("unmarshal collection %s: %w", name, err)
		}

		s.collections[name] = docs
	}

	return nil
}

// cosineSimilarity calculates the cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) {
		return 0
	}

	var dotProduct, normA, normB float64
	for i := range a {
		dotProduct += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return float32(dotProduct / (math.Sqrt(normA) * math.Sqrt(normB)))
}
