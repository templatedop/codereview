package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryVectorStore_InsertAndSearch(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	// Create test documents with embeddings
	docs := []Document{
		{
			ID:        "doc1",
			Content:   "PostgreSQL performance optimization",
			Embedding: []float32{1.0, 0.0, 0.0},
			Source:    "test",
			Category:  "performance",
		},
		{
			ID:        "doc2",
			Content:   "SQL injection prevention",
			Embedding: []float32{0.0, 1.0, 0.0},
			Source:    "test",
			Category:  "security",
		},
		{
			ID:        "doc3",
			Content:   "Database indexing strategies",
			Embedding: []float32{0.9, 0.1, 0.0},
			Source:    "test",
			Category:  "performance",
		},
	}

	// Insert documents
	for _, doc := range docs {
		if err := store.Insert(ctx, "test_collection", doc); err != nil {
			t.Fatalf("Insert() error = %v", err)
		}
	}

	// Search for similar documents
	query := []float32{1.0, 0.0, 0.0} // Should match doc1 best
	results, err := store.Search(ctx, "test_collection", query, 2)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Search() returned %d results, want 2", len(results))
	}

	if len(results) > 0 && results[0].Document.ID != "doc1" {
		t.Errorf("Search() top result = %s, want doc1", results[0].Document.ID)
	}
}

func TestMemoryVectorStore_InsertBatch(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	docs := []Document{
		{ID: "doc1", Content: "Content 1", Embedding: []float32{1, 0, 0}},
		{ID: "doc2", Content: "Content 2", Embedding: []float32{0, 1, 0}},
		{ID: "doc3", Content: "Content 3", Embedding: []float32{0, 0, 1}},
	}

	if err := store.InsertBatch(ctx, "batch_test", docs); err != nil {
		t.Fatalf("InsertBatch() error = %v", err)
	}

	size, err := store.CollectionSize(ctx, "batch_test")
	if err != nil {
		t.Fatalf("CollectionSize() error = %v", err)
	}

	if size != 3 {
		t.Errorf("CollectionSize() = %d, want 3", size)
	}
}

func TestMemoryVectorStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	doc := Document{ID: "delete_me", Content: "Test", Embedding: []float32{1, 0, 0}}
	if err := store.Insert(ctx, "delete_test", doc); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	size, _ := store.CollectionSize(ctx, "delete_test")
	if size != 1 {
		t.Fatalf("Initial size = %d, want 1", size)
	}

	if err := store.Delete(ctx, "delete_test", "delete_me"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	size, _ = store.CollectionSize(ctx, "delete_test")
	if size != 0 {
		t.Errorf("Size after delete = %d, want 0", size)
	}
}

func TestMemoryVectorStore_ListCollections(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	// Insert into multiple collections
	store.Insert(ctx, "collection_a", Document{ID: "1", Embedding: []float32{1}})
	store.Insert(ctx, "collection_b", Document{ID: "2", Embedding: []float32{1}})
	store.Insert(ctx, "collection_c", Document{ID: "3", Embedding: []float32{1}})

	collections, err := store.ListCollections(ctx)
	if err != nil {
		t.Fatalf("ListCollections() error = %v", err)
	}

	if len(collections) != 3 {
		t.Errorf("ListCollections() returned %d collections, want 3", len(collections))
	}
}

func TestMemoryVectorStore_SearchMultiple(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	// Insert into different collections
	store.Insert(ctx, "perf", Document{ID: "p1", Content: "Performance", Embedding: []float32{1, 0}})
	store.Insert(ctx, "sec", Document{ID: "s1", Content: "Security", Embedding: []float32{0, 1}})

	query := []float32{0.5, 0.5}
	results, err := store.SearchMultiple(ctx, []string{"perf", "sec"}, query, 10)
	if err != nil {
		t.Fatalf("SearchMultiple() error = %v", err)
	}

	if len(results) != 2 {
		t.Errorf("SearchMultiple() returned %d results, want 2", len(results))
	}
}

func TestMemoryVectorStore_UpdateExisting(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	doc1 := Document{ID: "same_id", Content: "Original", Embedding: []float32{1, 0}}
	doc2 := Document{ID: "same_id", Content: "Updated", Embedding: []float32{0, 1}}

	store.Insert(ctx, "update_test", doc1)
	store.Insert(ctx, "update_test", doc2)

	size, _ := store.CollectionSize(ctx, "update_test")
	if size != 1 {
		t.Errorf("Size after update = %d, want 1", size)
	}

	// Verify content was updated
	results, _ := store.Search(ctx, "update_test", []float32{0, 1}, 1)
	if len(results) > 0 && results[0].Document.Content != "Updated" {
		t.Errorf("Content = %s, want Updated", results[0].Document.Content)
	}
}

func TestMemoryVectorStore_Persistence(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "vectorstore_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	ctx := context.Background()

	// Create store and insert data
	store1 := NewMemoryVectorStore(tmpDir)
	doc := Document{ID: "persist", Content: "Persistent data", Embedding: []float32{1, 2, 3}}
	if err := store1.Insert(ctx, "persist_test", doc); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}

	// Create new store loading from same path
	store2 := NewMemoryVectorStore(tmpDir)
	size, err := store2.CollectionSize(ctx, "persist_test")
	if err != nil {
		t.Fatalf("CollectionSize() error = %v", err)
	}

	if size != 1 {
		t.Errorf("Persisted size = %d, want 1", size)
	}

	// Verify data exists
	if _, err := os.Stat(filepath.Join(tmpDir, "persist_test.json")); os.IsNotExist(err) {
		t.Error("Persistence file was not created")
	}
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    []float32
		b    []float32
		want float32
	}{
		{
			name: "identical vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{1, 0, 0},
			want: 1.0,
		},
		{
			name: "orthogonal vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{0, 1, 0},
			want: 0.0,
		},
		{
			name: "opposite vectors",
			a:    []float32{1, 0, 0},
			b:    []float32{-1, 0, 0},
			want: -1.0,
		},
		{
			name: "different lengths",
			a:    []float32{1, 0},
			b:    []float32{1, 0, 0},
			want: 0.0, // Should handle gracefully
		},
		{
			name: "zero vector",
			a:    []float32{0, 0, 0},
			b:    []float32{1, 0, 0},
			want: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cosineSimilarity(tt.a, tt.b)
			if got < tt.want-0.001 || got > tt.want+0.001 {
				t.Errorf("cosineSimilarity() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMemoryVectorStore_EmptyCollection(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryVectorStore("")

	results, err := store.Search(ctx, "nonexistent", []float32{1, 0}, 5)
	if err != nil {
		t.Fatalf("Search() on nonexistent collection error = %v", err)
	}

	if results != nil && len(results) != 0 {
		t.Errorf("Search() on nonexistent returned %d results, want 0", len(results))
	}
}
