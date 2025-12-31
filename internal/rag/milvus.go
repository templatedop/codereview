package rag

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
)

// MilvusVectorStore implements VectorStore using Milvus.
type MilvusVectorStore struct {
	client    client.Client
	dimension int
}

// MilvusConfig holds configuration for Milvus connection.
type MilvusConfig struct {
	Address   string
	Username  string
	Password  string
	Dimension int
}

// NewMilvusVectorStore creates a new Milvus-backed vector store.
func NewMilvusVectorStore(ctx context.Context, cfg MilvusConfig) (*MilvusVectorStore, error) {
	c, err := client.NewClient(ctx, client.Config{
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("connect to milvus: %w", err)
	}

	return &MilvusVectorStore{
		client:    c,
		dimension: cfg.Dimension,
	}, nil
}

// ensureCollection creates the collection if it doesn't exist.
func (s *MilvusVectorStore) ensureCollection(ctx context.Context, collection string) error {
	exists, err := s.client.HasCollection(ctx, collection)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if exists {
		return nil
	}

	schema := &entity.Schema{
		CollectionName: collection,
		Description:    "PostgreSQL knowledge base: " + collection,
		Fields: []*entity.Field{
			{
				Name:       "id",
				DataType:   entity.FieldTypeVarChar,
				PrimaryKey: true,
				AutoID:     false,
				TypeParams: map[string]string{"max_length": "256"},
			},
			{
				Name:     "embedding",
				DataType: entity.FieldTypeFloatVector,
				TypeParams: map[string]string{
					"dim": fmt.Sprintf("%d", s.dimension),
				},
			},
			{
				Name:       "content",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "65535"},
			},
			{
				Name:       "source",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "256"},
			},
			{
				Name:       "category",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "64"},
			},
			{
				Name:       "metadata",
				DataType:   entity.FieldTypeVarChar,
				TypeParams: map[string]string{"max_length": "4096"},
			},
			{
				Name:     "chunk_index",
				DataType: entity.FieldTypeInt32,
			},
		},
	}

	if err := s.client.CreateCollection(ctx, schema, entity.DefaultShardNumber); err != nil {
		return fmt.Errorf("create collection: %w", err)
	}

	// Create IVF_FLAT index for vector field
	idx, err := entity.NewIndexIvfFlat(entity.COSINE, 128)
	if err != nil {
		return fmt.Errorf("create index params: %w", err)
	}

	if err := s.client.CreateIndex(ctx, collection, "embedding", idx, false); err != nil {
		return fmt.Errorf("create index: %w", err)
	}

	// Load collection into memory
	if err := s.client.LoadCollection(ctx, collection, false); err != nil {
		return fmt.Errorf("load collection: %w", err)
	}

	return nil
}

// Insert adds a document to the specified collection.
func (s *MilvusVectorStore) Insert(ctx context.Context, collection string, doc Document) error {
	return s.InsertBatch(ctx, collection, []Document{doc})
}

// InsertBatch adds multiple documents to the specified collection.
func (s *MilvusVectorStore) InsertBatch(ctx context.Context, collection string, docs []Document) error {
	if err := s.ensureCollection(ctx, collection); err != nil {
		return err
	}

	ids := make([]string, len(docs))
	embeddings := make([][]float32, len(docs))
	contents := make([]string, len(docs))
	sources := make([]string, len(docs))
	categories := make([]string, len(docs))
	metadatas := make([]string, len(docs))
	chunkIndices := make([]int32, len(docs))

	for i, doc := range docs {
		ids[i] = doc.ID
		embeddings[i] = doc.Embedding
		contents[i] = truncateString(doc.Content, 65000)
		sources[i] = doc.Source
		categories[i] = doc.Category
		metadatas[i] = mapToJSON(doc.Metadata)
		chunkIndices[i] = int32(doc.ChunkIndex)
	}

	idColumn := entity.NewColumnVarChar("id", ids)
	embeddingColumn := entity.NewColumnFloatVector("embedding", s.dimension, embeddings)
	contentColumn := entity.NewColumnVarChar("content", contents)
	sourceColumn := entity.NewColumnVarChar("source", sources)
	categoryColumn := entity.NewColumnVarChar("category", categories)
	metadataColumn := entity.NewColumnVarChar("metadata", metadatas)
	chunkIndexColumn := entity.NewColumnInt32("chunk_index", chunkIndices)

	_, err := s.client.Insert(ctx, collection, "",
		idColumn, embeddingColumn, contentColumn, sourceColumn,
		categoryColumn, metadataColumn, chunkIndexColumn)
	if err != nil {
		return fmt.Errorf("insert documents: %w", err)
	}

	// Flush to ensure data is persisted
	if err := s.client.Flush(ctx, collection, false); err != nil {
		return fmt.Errorf("flush collection: %w", err)
	}

	return nil
}

// Search finds similar documents in the specified collection.
func (s *MilvusVectorStore) Search(ctx context.Context, collection string, query []float32, topK int) ([]SearchResult, error) {
	exists, err := s.client.HasCollection(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil, nil
	}

	// Ensure collection is loaded
	if err := s.client.LoadCollection(ctx, collection, false); err != nil {
		return nil, fmt.Errorf("load collection: %w", err)
	}

	// Wait a bit for collection to be ready
	time.Sleep(100 * time.Millisecond)

	sp, err := entity.NewIndexIvfFlatSearchParam(16)
	if err != nil {
		return nil, fmt.Errorf("create search params: %w", err)
	}

	vectors := []entity.Vector{entity.FloatVector(query)}
	results, err := s.client.Search(ctx, collection, nil, "", []string{"id", "content", "source", "category", "metadata", "chunk_index"},
		vectors, "embedding", entity.COSINE, topK, sp)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	var searchResults []SearchResult
	for _, result := range results {
		for i := 0; i < result.ResultCount; i++ {
			doc := Document{
				ID:         getStringField(result, "id", i),
				Content:    getStringField(result, "content", i),
				Source:     getStringField(result, "source", i),
				Category:   getStringField(result, "category", i),
				Metadata:   jsonToMap(getStringField(result, "metadata", i)),
				ChunkIndex: getIntField(result, "chunk_index", i),
			}
			searchResults = append(searchResults, SearchResult{
				Document:   doc,
				Score:      result.Scores[i],
				Collection: collection,
			})
		}
	}

	return searchResults, nil
}

// SearchMultiple searches across multiple collections.
func (s *MilvusVectorStore) SearchMultiple(ctx context.Context, collections []string, query []float32, topK int) ([]SearchResult, error) {
	var allResults []SearchResult

	for _, collection := range collections {
		results, err := s.Search(ctx, collection, query, topK)
		if err != nil {
			continue // Skip failed collections
		}
		allResults = append(allResults, results...)
	}

	// Sort by score descending
	for i := 0; i < len(allResults)-1; i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[j].Score > allResults[i].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	if topK > 0 && len(allResults) > topK {
		allResults = allResults[:topK]
	}

	return allResults, nil
}

// Delete removes a document by ID.
func (s *MilvusVectorStore) Delete(ctx context.Context, collection, id string) error {
	exists, err := s.client.HasCollection(ctx, collection)
	if err != nil {
		return fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return nil
	}

	expr := fmt.Sprintf("id == '%s'", id)
	if err := s.client.Delete(ctx, collection, "", expr); err != nil {
		return fmt.Errorf("delete document: %w", err)
	}

	return nil
}

// ListCollections returns all collection names.
func (s *MilvusVectorStore) ListCollections(ctx context.Context) ([]string, error) {
	collections, err := s.client.ListCollections(ctx)
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}

	names := make([]string, len(collections))
	for i, c := range collections {
		names[i] = c.Name
	}
	return names, nil
}

// CollectionSize returns the number of documents in a collection.
func (s *MilvusVectorStore) CollectionSize(ctx context.Context, collection string) (int, error) {
	exists, err := s.client.HasCollection(ctx, collection)
	if err != nil {
		return 0, fmt.Errorf("check collection: %w", err)
	}
	if !exists {
		return 0, nil
	}

	stats, err := s.client.GetCollectionStatistics(ctx, collection)
	if err != nil {
		return 0, fmt.Errorf("get statistics: %w", err)
	}

	if rowCount, ok := stats["row_count"]; ok {
		var count int
		fmt.Sscanf(rowCount, "%d", &count)
		return count, nil
	}

	return 0, nil
}

// Close closes the Milvus connection.
func (s *MilvusVectorStore) Close() error {
	return s.client.Close()
}

// Helper functions

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func mapToJSON(m map[string]string) string {
	if m == nil {
		return "{}"
	}
	data, _ := json.Marshal(m)
	return string(data)
}

func jsonToMap(s string) map[string]string {
	var m map[string]string
	json.Unmarshal([]byte(s), &m)
	if m == nil {
		m = make(map[string]string)
	}
	return m
}

func getStringField(result client.SearchResult, field string, index int) string {
	col := result.Fields.GetColumn(field)
	if col == nil {
		return ""
	}
	if vc, ok := col.(*entity.ColumnVarChar); ok {
		if index < vc.Len() {
			val, _ := vc.ValueByIdx(index)
			return val
		}
	}
	return ""
}

func getIntField(result client.SearchResult, field string, index int) int {
	col := result.Fields.GetColumn(field)
	if col == nil {
		return 0
	}
	if ic, ok := col.(*entity.ColumnInt32); ok {
		if index < ic.Len() {
			val, _ := ic.ValueByIdx(index)
			return int(val)
		}
	}
	return 0
}

// Ensure MilvusVectorStore satisfies VectorStore interface
var _ VectorStore = (*MilvusVectorStore)(nil)
