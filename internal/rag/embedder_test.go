package rag

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaEmbedder_Embed(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("Expected path /api/embeddings, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Decode request
		var req ollamaEmbedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("Failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("Expected model test-model, got %s", req.Model)
		}

		// Return mock embedding
		resp := ollamaEmbedResponse{
			Embedding: []float32{0.1, 0.2, 0.3},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 3)

	embedding, err := embedder.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(embedding) != 3 {
		t.Errorf("Embed() returned %d dimensions, want 3", len(embedding))
	}

	expected := []float32{0.1, 0.2, 0.3}
	for i, v := range embedding {
		if v != expected[i] {
			t.Errorf("Embed()[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestOllamaEmbedder_EmbedBatch(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := ollamaEmbedResponse{
			Embedding: []float32{float32(callCount) * 0.1, float32(callCount) * 0.2},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 2)

	texts := []string{"text1", "text2", "text3"}
	embeddings, err := embedder.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}

	if len(embeddings) != 3 {
		t.Errorf("EmbedBatch() returned %d embeddings, want 3", len(embeddings))
	}

	if callCount != 3 {
		t.Errorf("EmbedBatch() made %d calls, want 3", callCount)
	}
}

func TestOllamaEmbedder_Dimension(t *testing.T) {
	embedder := NewOllamaEmbedder("http://localhost", "model", 768)
	if embedder.Dimension() != 768 {
		t.Errorf("Dimension() = %d, want 768", embedder.Dimension())
	}
}

func TestOllamaEmbedder_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer server.Close()

	embedder := NewOllamaEmbedder(server.URL, "test-model", 3)

	_, err := embedder.Embed(context.Background(), "test")
	if err == nil {
		t.Error("Embed() expected error, got nil")
	}
}

func TestOpenAIEmbedder_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("Expected path /v1/embeddings, got %s", r.URL.Path)
		}

		// Check authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			t.Errorf("Expected Authorization header 'Bearer test-key', got %s", auth)
		}

		resp := openAIEmbedResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.5, 0.6, 0.7}, Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "test-key", "text-embedding-ada-002", 3)

	embedding, err := embedder.Embed(context.Background(), "test text")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(embedding) != 3 {
		t.Errorf("Embed() returned %d dimensions, want 3", len(embedding))
	}

	expected := []float32{0.5, 0.6, 0.7}
	for i, v := range embedding {
		if v != expected[i] {
			t.Errorf("Embed()[%d] = %f, want %f", i, v, expected[i])
		}
	}
}

func TestOpenAIEmbedder_EmbedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openAIEmbedRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := openAIEmbedResponse{
			Data: make([]struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}, len(req.Input)),
		}
		for i := range req.Input {
			resp.Data[i] = struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				Embedding: []float32{float32(i) * 0.1, float32(i) * 0.2},
				Index:     i,
			}
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "test-key", "model", 2)

	texts := []string{"text1", "text2", "text3"}
	embeddings, err := embedder.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}

	if len(embeddings) != 3 {
		t.Errorf("EmbedBatch() returned %d embeddings, want 3", len(embeddings))
	}
}

func TestOpenAIEmbedder_Dimension(t *testing.T) {
	embedder := NewOpenAIEmbedder("http://localhost", "key", "model", 1536)
	if embedder.Dimension() != 1536 {
		t.Errorf("Dimension() = %d, want 1536", embedder.Dimension())
	}
}

func TestOpenAIEmbedder_NoAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check no authorization header is sent when key is empty
		auth := r.Header.Get("Authorization")
		if auth != "" {
			t.Errorf("Expected no Authorization header, got %s", auth)
		}

		resp := openAIEmbedResponse{
			Data: []struct {
				Embedding []float32 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float32{0.1}, Index: 0},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	embedder := NewOpenAIEmbedder(server.URL, "", "model", 1)

	_, err := embedder.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
}
