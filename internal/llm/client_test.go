package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient(t *testing.T) {
	cfg := Config{
		BaseURL: "http://localhost:8000",
		Model:   "test-model",
		Timeout: 30 * time.Second,
	}

	client := NewClient(cfg)

	if client.baseURL != cfg.BaseURL {
		t.Errorf("expected baseURL %q, got %q", cfg.BaseURL, client.baseURL)
	}
	if client.model != cfg.Model {
		t.Errorf("expected model %q, got %q", cfg.Model, client.model)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	cfg := Config{
		BaseURL: "http://localhost:8000",
		Model:   "test-model",
	}

	client := NewClient(cfg)

	if client.httpClient.Timeout != 5*time.Minute {
		t.Errorf("expected default timeout 5m, got %v", client.httpClient.Timeout)
	}
}

func TestClient_Chat(t *testing.T) {
	expectedResponse := ChatResponse{
		ID: "test-id",
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "Hello, world!",
				},
				FinishReason: "stop",
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json")
		}

		var req ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Model != "test-model" {
			t.Errorf("expected model test-model, got %s", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expectedResponse)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	messages := []Message{
		{Role: "user", Content: "Hello"},
	}

	resp, err := client.Chat(context.Background(), messages)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}

	if resp.Choices[0].Message.Content != "Hello, world!" {
		t.Errorf("expected content 'Hello, world!', got %q", resp.Choices[0].Message.Content)
	}
}

func TestClient_Chat_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	_, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "test"}})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_Complete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ChatRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Verify system and user messages
		if len(req.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(req.Messages))
		}
		if req.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system")
		}
		if req.Messages[1].Role != "user" {
			t.Errorf("expected second message to be user")
		}

		resp := ChatResponse{
			Choices: []Choice{
				{Message: Message{Content: "Completion response"}},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	result, err := client.Complete(context.Background(), "System prompt", "User prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Completion response" {
		t.Errorf("expected 'Completion response', got %q", result)
	}
}

func TestClient_Complete_NoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := ChatResponse{
			Choices: []Choice{},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Model:   "test-model",
	})

	_, err := client.Complete(context.Background(), "System", "User")
	if err == nil {
		t.Fatal("expected error for no choices, got nil")
	}
}

func TestMessage_JSON(t *testing.T) {
	msg := Message{
		Role:    "assistant",
		Content: "Hello",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.Role != msg.Role || decoded.Content != msg.Content {
		t.Errorf("mismatch: expected %+v, got %+v", msg, decoded)
	}
}
