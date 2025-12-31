package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewOllamaClient(t *testing.T) {
	cfg := Config{
		BaseURL: "http://localhost:11434",
		Model:   "deepseek-coder:1.3b",
		Timeout: 30 * time.Second,
	}

	client := NewOllamaClient(cfg)

	if client.baseURL != cfg.BaseURL {
		t.Errorf("expected baseURL %q, got %q", cfg.BaseURL, client.baseURL)
	}
	if client.model != cfg.Model {
		t.Errorf("expected model %q, got %q", cfg.Model, client.model)
	}
}

func TestNewOllamaClient_DefaultTimeout(t *testing.T) {
	cfg := Config{
		BaseURL: "http://localhost:11434",
		Model:   "test-model",
	}

	client := NewOllamaClient(cfg)

	if client.httpClient.Timeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %v", client.httpClient.Timeout)
	}
}

func TestOllamaClient_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req OllamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}
		if req.Stream != false {
			t.Errorf("expected stream=false")
		}
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}

		resp := OllamaChatResponse{
			Model: req.Model,
			Message: OllamaMessage{
				Role:    "assistant",
				Content: "Test response",
			},
			Done: true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	result, err := client.Complete(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Test response" {
		t.Errorf("expected 'Test response', got %q", result)
	}
}

func TestOllamaClient_Complete_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Model not found"))
	}))
	defer server.Close()

	client := NewOllamaClient(Config{
		BaseURL: server.URL,
		Model:   "nonexistent-model",
	})

	_, err := client.Complete(context.Background(), "System", "User")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestOllamaClient_Generate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected path /api/generate, got %s", r.URL.Path)
		}

		var req OllamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Stream != false {
			t.Errorf("expected stream=false")
		}

		resp := OllamaGenerateResponse{
			Model:    req.Model,
			Response: "Generated response",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	result, err := client.Generate(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Generated response" {
		t.Errorf("expected 'Generated response', got %q", result)
	}
}

func TestOllamaClient_Generate_NoSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req OllamaGenerateRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify prompt is just the user prompt when no system prompt
		if req.Prompt != "Just user prompt" {
			t.Errorf("expected prompt 'Just user prompt', got %q", req.Prompt)
		}

		resp := OllamaGenerateResponse{
			Response: "OK",
			Done:     true,
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	_, err := client.Generate(context.Background(), "", "Just user prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOllamaClient_ListModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Errorf("expected path /api/tags, got %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("expected GET, got %s", r.Method)
		}

		resp := struct {
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
		}{
			Models: []struct {
				Name string `json:"name"`
			}{
				{Name: "deepseek-coder:1.3b"},
				{Name: "llama2:7b"},
				{Name: "codellama:7b"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewOllamaClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	models, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}

	expectedModels := []string{"deepseek-coder:1.3b", "llama2:7b", "codellama:7b"}
	for i, expected := range expectedModels {
		if models[i] != expected {
			t.Errorf("expected model[%d] = %q, got %q", i, expected, models[i])
		}
	}
}

func TestOllamaOptions_JSON(t *testing.T) {
	opts := OllamaOptions{
		Temperature: 0.7,
		NumPredict:  100,
	}

	data, err := json.Marshal(opts)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded OllamaOptions
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Temperature != opts.Temperature {
		t.Errorf("expected temperature %f, got %f", opts.Temperature, decoded.Temperature)
	}
	if decoded.NumPredict != opts.NumPredict {
		t.Errorf("expected num_predict %d, got %d", opts.NumPredict, decoded.NumPredict)
	}
}
