package rag

import (
	"context"
	"fmt"
	"strings"
)

// Retriever provides context retrieval for RAG-based analysis.
type Retriever struct {
	embedder    Embedder
	vectorStore VectorStore
	chunker     *Chunker
}

// NewRetriever creates a new retriever instance.
func NewRetriever(embedder Embedder, vectorStore VectorStore, chunker *Chunker) *Retriever {
	if chunker == nil {
		cfg := DefaultChunkConfig()
		chunker = NewChunker(cfg)
	}
	return &Retriever{
		embedder:    embedder,
		vectorStore: vectorStore,
		chunker:     chunker,
	}
}

// RetrievalRequest defines parameters for context retrieval.
type RetrievalRequest struct {
	// Query is the text to find similar documents for.
	Query string
	// Collections specifies which collections to search.
	Collections []string
	// TopK is the number of results to return.
	TopK int
	// MinScore is the minimum similarity score threshold.
	MinScore float32
	// IncludeMetadata includes document metadata in results.
	IncludeMetadata bool
}

// RetrievalResponse contains retrieved context.
type RetrievalResponse struct {
	// Documents contains the retrieved documents.
	Documents []RetrievedDocument
	// Query is the original query.
	Query string
	// FormattedContext is the context formatted for LLM prompts.
	FormattedContext string
}

// RetrievedDocument represents a document with relevance info.
type RetrievedDocument struct {
	Content    string
	Source     string
	Category   string
	Score      float32
	Metadata   map[string]string
	Collection string
}

// Retrieve finds relevant documents for the given query.
func (r *Retriever) Retrieve(ctx context.Context, req RetrievalRequest) (*RetrievalResponse, error) {
	if req.TopK <= 0 {
		req.TopK = 5
	}

	// Generate query embedding
	queryEmbed, err := r.embedder.Embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}

	// Search vector store
	var results []SearchResult
	if len(req.Collections) == 0 {
		// Get all collections
		collections, err := r.vectorStore.ListCollections(ctx)
		if err != nil {
			return nil, fmt.Errorf("list collections: %w", err)
		}
		req.Collections = collections
	}

	results, err = r.vectorStore.SearchMultiple(ctx, req.Collections, queryEmbed, req.TopK*2)
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}

	// Filter by score and deduplicate
	seen := make(map[string]bool)
	var docs []RetrievedDocument
	for _, result := range results {
		if result.Score < req.MinScore {
			continue
		}
		// Deduplicate by content hash
		key := hashContent(result.Document.Content)
		if seen[key] {
			continue
		}
		seen[key] = true

		doc := RetrievedDocument{
			Content:    result.Document.Content,
			Source:     result.Document.Source,
			Category:   result.Document.Category,
			Score:      result.Score,
			Collection: result.Collection,
		}
		if req.IncludeMetadata {
			doc.Metadata = result.Document.Metadata
		}
		docs = append(docs, doc)

		if len(docs) >= req.TopK {
			break
		}
	}

	response := &RetrievalResponse{
		Documents: docs,
		Query:     req.Query,
	}

	// Format context for LLM
	response.FormattedContext = r.FormatContext(docs)

	return response, nil
}

// RetrieveForCategory retrieves context for a specific category.
func (r *Retriever) RetrieveForCategory(ctx context.Context, query, category string, topK int) (*RetrievalResponse, error) {
	collections := []string{category}
	return r.Retrieve(ctx, RetrievalRequest{
		Query:       query,
		Collections: collections,
		TopK:        topK,
		MinScore:    0.5,
	})
}

// RetrieveForPostgres retrieves PostgreSQL-specific context.
func (r *Retriever) RetrieveForPostgres(ctx context.Context, query string, categories []string) (*RetrievalResponse, error) {
	if len(categories) == 0 {
		categories = []string{"performance", "security", "standards", "migrations"}
	}

	// Prefix collections with postgres namespace
	collections := make([]string, len(categories))
	for i, cat := range categories {
		collections[i] = "postgres_" + cat
	}

	return r.Retrieve(ctx, RetrievalRequest{
		Query:           query,
		Collections:     collections,
		TopK:            5,
		MinScore:        0.6,
		IncludeMetadata: true,
	})
}

// FormatContext formats retrieved documents for LLM context.
func (r *Retriever) FormatContext(docs []RetrievedDocument) string {
	if len(docs) == 0 {
		return "No relevant documentation found."
	}

	var sb strings.Builder
	sb.WriteString("RELEVANT KNOWLEDGE BASE CONTEXT:\n")
	sb.WriteString("================================\n\n")

	for i, doc := range docs {
		sb.WriteString(fmt.Sprintf("--- Document %d (Score: %.2f, Source: %s) ---\n", i+1, doc.Score, doc.Source))
		if doc.Category != "" {
			sb.WriteString(fmt.Sprintf("Category: %s\n", doc.Category))
		}
		sb.WriteString("\n")
		sb.WriteString(doc.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// FormatContextCompact provides a more compact format for limited context windows.
func (r *Retriever) FormatContextCompact(docs []RetrievedDocument, maxTokens int) string {
	if len(docs) == 0 {
		return ""
	}

	var sb strings.Builder
	currentTokens := 0

	for _, doc := range docs {
		docTokens := CountTokens(doc.Content)
		if currentTokens+docTokens > maxTokens {
			// Truncate this document to fit
			remaining := maxTokens - currentTokens - 50 // Leave room for markers
			if remaining <= 0 {
				break
			}
			truncated := doc.Content[:remaining*4]
			sb.WriteString(fmt.Sprintf("[%s] %s...\n\n", doc.Source, truncated))
			break
		}
		sb.WriteString(fmt.Sprintf("[%s] %s\n\n", doc.Source, doc.Content))
		currentTokens += docTokens
	}

	return sb.String()
}

// IndexDocument adds a document to the knowledge base.
func (r *Retriever) IndexDocument(ctx context.Context, collection string, doc Document) error {
	// Generate embedding for the document content
	embedding, err := r.embedder.Embed(ctx, doc.Content)
	if err != nil {
		return fmt.Errorf("embed document: %w", err)
	}
	doc.Embedding = embedding

	return r.vectorStore.Insert(ctx, collection, doc)
}

// IndexDocumentChunked splits a large document into chunks and indexes each.
func (r *Retriever) IndexDocumentChunked(ctx context.Context, collection string, doc Document) error {
	chunks := r.chunker.ChunkText(doc.Content)
	if len(chunks) == 0 {
		return nil
	}

	for i, chunk := range chunks {
		chunkDoc := Document{
			ID:         fmt.Sprintf("%s_chunk_%d", doc.ID, i),
			Content:    chunk.Content,
			Source:     doc.Source,
			Category:   doc.Category,
			Metadata:   doc.Metadata,
			ChunkIndex: i,
		}

		embedding, err := r.embedder.Embed(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", i, err)
		}
		chunkDoc.Embedding = embedding

		if err := r.vectorStore.Insert(ctx, collection, chunkDoc); err != nil {
			return fmt.Errorf("index chunk %d: %w", i, err)
		}
	}

	return nil
}

// hashContent creates a simple hash for deduplication.
func hashContent(content string) string {
	// Simple hash based on first 100 and last 100 chars plus length
	if len(content) < 200 {
		return content
	}
	return fmt.Sprintf("%s...%s:%d", content[:100], content[len(content)-100:], len(content))
}
