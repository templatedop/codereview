package rag

import (
	"context"
	"testing"
)

// MockEmbedder is a mock implementation of Embedder for testing.
type MockEmbedder struct {
	dimension int
	embedFn   func(text string) []float32
}

func NewMockEmbedder(dim int) *MockEmbedder {
	return &MockEmbedder{
		dimension: dim,
		embedFn: func(text string) []float32 {
			// Simple deterministic embedding based on text length
			result := make([]float32, dim)
			for i := 0; i < dim && i < len(text); i++ {
				result[i] = float32(text[i]) / 255.0
			}
			return result
		},
	}
}

func (m *MockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	return m.embedFn(text), nil
}

func (m *MockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	results := make([][]float32, len(texts))
	for i, text := range texts {
		results[i] = m.embedFn(text)
	}
	return results, nil
}

func (m *MockEmbedder) Dimension() int {
	return m.dimension
}

func TestRetriever_Retrieve(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder(3)
	store := NewMemoryVectorStore("")
	retriever := NewRetriever(embedder, store, nil)

	// Add test documents
	docs := []Document{
		{ID: "1", Content: "PostgreSQL indexing best practices", Embedding: []float32{1, 0, 0}, Source: "howtos", Category: "performance"},
		{ID: "2", Content: "SQL injection prevention guide", Embedding: []float32{0, 1, 0}, Source: "security", Category: "security"},
		{ID: "3", Content: "Database optimization tips", Embedding: []float32{0.9, 0.1, 0}, Source: "howtos", Category: "performance"},
	}

	for _, doc := range docs {
		store.Insert(ctx, "test", doc)
	}

	// Test retrieval
	resp, err := retriever.Retrieve(ctx, RetrievalRequest{
		Query:       "indexing",
		Collections: []string{"test"},
		TopK:        2,
		MinScore:    0,
	})

	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	if len(resp.Documents) == 0 {
		t.Error("Retrieve() returned no documents")
	}

	if resp.FormattedContext == "" {
		t.Error("Retrieve() returned empty formatted context")
	}
}

func TestRetriever_RetrieveForCategory(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder(3)
	store := NewMemoryVectorStore("")
	retriever := NewRetriever(embedder, store, nil)

	// Add documents to specific collection
	store.Insert(ctx, "postgres_performance", Document{
		ID: "1", Content: "Index tuning", Embedding: []float32{1, 0, 0},
	})

	resp, err := retriever.RetrieveForCategory(ctx, "index optimization", "postgres_performance", 5)
	if err != nil {
		t.Fatalf("RetrieveForCategory() error = %v", err)
	}

	if resp == nil {
		t.Error("RetrieveForCategory() returned nil response")
	}
}

func TestRetriever_FormatContext(t *testing.T) {
	retriever := NewRetriever(nil, nil, nil)

	docs := []RetrievedDocument{
		{Content: "Document 1 content", Source: "source1", Score: 0.95},
		{Content: "Document 2 content", Source: "source2", Score: 0.85, Category: "performance"},
	}

	formatted := retriever.FormatContext(docs)

	if formatted == "" {
		t.Error("FormatContext() returned empty string")
	}

	if formatted == "No relevant documentation found." {
		t.Error("FormatContext() incorrectly returned no docs message")
	}

	// Check that content is included
	if !contains(formatted, "Document 1 content") {
		t.Error("FormatContext() missing document 1 content")
	}
	if !contains(formatted, "Document 2 content") {
		t.Error("FormatContext() missing document 2 content")
	}
}

func TestRetriever_FormatContextEmpty(t *testing.T) {
	retriever := NewRetriever(nil, nil, nil)

	formatted := retriever.FormatContext(nil)

	if formatted != "No relevant documentation found." {
		t.Errorf("FormatContext(nil) = %q, want 'No relevant documentation found.'", formatted)
	}
}

func TestRetriever_FormatContextCompact(t *testing.T) {
	retriever := NewRetriever(nil, nil, nil)

	docs := []RetrievedDocument{
		{Content: "Short content", Source: "source1", Score: 0.95},
	}

	formatted := retriever.FormatContextCompact(docs, 1000)

	if formatted == "" {
		t.Error("FormatContextCompact() returned empty string")
	}

	if !contains(formatted, "Short content") {
		t.Error("FormatContextCompact() missing content")
	}
}

func TestRetriever_IndexDocument(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder(3)
	store := NewMemoryVectorStore("")
	retriever := NewRetriever(embedder, store, nil)

	doc := Document{
		ID:      "new_doc",
		Content: "New document content",
		Source:  "test",
	}

	if err := retriever.IndexDocument(ctx, "index_test", doc); err != nil {
		t.Fatalf("IndexDocument() error = %v", err)
	}

	// Verify document was indexed
	size, _ := store.CollectionSize(ctx, "index_test")
	if size != 1 {
		t.Errorf("Collection size = %d, want 1", size)
	}
}

func TestRetriever_IndexDocumentChunked(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder(3)
	store := NewMemoryVectorStore("")

	chunker := NewChunker(ChunkConfig{
		ChunkSize:    10,
		ChunkOverlap: 2,
		MinChunkSize: 5,
	})
	retriever := NewRetriever(embedder, store, chunker)

	doc := Document{
		ID:      "chunked_doc",
		Content: "This is a longer document that should be split into multiple chunks for better retrieval.",
		Source:  "test",
	}

	if err := retriever.IndexDocumentChunked(ctx, "chunk_test", doc); err != nil {
		t.Fatalf("IndexDocumentChunked() error = %v", err)
	}

	// Should have multiple chunks
	size, _ := store.CollectionSize(ctx, "chunk_test")
	if size < 1 {
		t.Errorf("Collection size = %d, want >= 1", size)
	}
}

func TestRetriever_Deduplication(t *testing.T) {
	ctx := context.Background()
	embedder := NewMockEmbedder(3)
	store := NewMemoryVectorStore("")
	retriever := NewRetriever(embedder, store, nil)

	// Add duplicate documents with same content
	sameEmbedding := []float32{1, 0, 0}
	store.Insert(ctx, "dedup", Document{ID: "1", Content: "Same content here", Embedding: sameEmbedding})
	store.Insert(ctx, "dedup", Document{ID: "2", Content: "Same content here", Embedding: sameEmbedding})
	store.Insert(ctx, "dedup", Document{ID: "3", Content: "Different content", Embedding: []float32{0, 1, 0}})

	resp, err := retriever.Retrieve(ctx, RetrievalRequest{
		Query:       "content",
		Collections: []string{"dedup"},
		TopK:        10,
		MinScore:    0,
	})

	if err != nil {
		t.Fatalf("Retrieve() error = %v", err)
	}

	// Should deduplicate based on content
	if len(resp.Documents) > 2 {
		t.Errorf("Retrieve() returned %d documents, expected deduplication", len(resp.Documents))
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
