package gitlab

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
		BaseURL: "https://gitlab.example.com",
		Token:   "test-token",
		Timeout: 30 * time.Second,
	}

	client := NewClient(cfg)

	if client.baseURL != cfg.BaseURL {
		t.Errorf("expected baseURL %q, got %q", cfg.BaseURL, client.baseURL)
	}
	if client.token != cfg.Token {
		t.Errorf("expected token %q, got %q", cfg.Token, client.token)
	}
}

func TestNewClient_DefaultTimeout(t *testing.T) {
	cfg := Config{
		BaseURL: "https://gitlab.example.com",
		Token:   "test-token",
	}

	client := NewClient(cfg)

	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected default timeout 30s, got %v", client.httpClient.Timeout)
	}
}

func TestClient_GetMergeRequest(t *testing.T) {
	expectedMR := MergeRequest{
		ID:           1,
		IID:          42,
		Title:        "Test MR",
		SourceBranch: "feature",
		TargetBranch: "main",
		State:        "opened",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/123/merge_requests/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "test-token" {
			t.Error("missing or invalid token header")
		}

		json.NewEncoder(w).Encode(expectedMR)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	mr, err := client.GetMergeRequest(context.Background(), 123, 42)
	if err != nil {
		t.Fatalf("GetMergeRequest failed: %v", err)
	}

	if mr.IID != expectedMR.IID {
		t.Errorf("expected IID %d, got %d", expectedMR.IID, mr.IID)
	}
	if mr.Title != expectedMR.Title {
		t.Errorf("expected title %q, got %q", expectedMR.Title, mr.Title)
	}
}

func TestClient_GetMergeRequest_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not found"))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	_, err := client.GetMergeRequest(context.Background(), 123, 999)
	if err == nil {
		t.Error("expected error for not found")
	}
}

func TestClient_GetMergeRequestChanges(t *testing.T) {
	expected := MRChanges{
		MergeRequest: MergeRequest{IID: 42},
		Changes: []MRChange{
			{NewPath: "file.go", Diff: "+added line", NewFile: false},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/123/merge_requests/42/changes" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	changes, err := client.GetMergeRequestChanges(context.Background(), 123, 42)
	if err != nil {
		t.Fatalf("GetMergeRequestChanges failed: %v", err)
	}

	if len(changes.Changes) != 1 {
		t.Fatalf("expected 1 change, got %d", len(changes.Changes))
	}
	if changes.Changes[0].NewPath != "file.go" {
		t.Errorf("expected path 'file.go', got %q", changes.Changes[0].NewPath)
	}
}

func TestClient_CreateMRNote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/123/merge_requests/42/notes" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var payload map[string]string
		json.NewDecoder(r.Body).Decode(&payload)
		if payload["body"] != "Test comment" {
			t.Errorf("unexpected body: %q", payload["body"])
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Note{ID: 1, Body: payload["body"]})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	note, err := client.CreateMRNote(context.Background(), 123, 42, "Test comment")
	if err != nil {
		t.Fatalf("CreateMRNote failed: %v", err)
	}

	if note.Body != "Test comment" {
		t.Errorf("expected body 'Test comment', got %q", note.Body)
	}
}

func TestClient_CreateDiscussion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/123/merge_requests/42/discussions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Discussion{ID: "abc123"})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	position := &DiscussionPosition{
		BaseSHA:      "abc",
		HeadSHA:      "def",
		StartSHA:     "abc",
		PositionType: "text",
		NewPath:      "file.go",
		NewLine:      10,
	}

	discussion, err := client.CreateDiscussion(context.Background(), 123, 42, "Comment", position)
	if err != nil {
		t.Fatalf("CreateDiscussion failed: %v", err)
	}

	if discussion.ID != "abc123" {
		t.Errorf("expected ID 'abc123', got %q", discussion.ID)
	}
}

func TestClient_GetMRVersions(t *testing.T) {
	expected := []MRVersion{
		{
			ID:             1,
			HeadCommitSHA:  "abc123",
			BaseCommitSHA:  "def456",
			StartCommitSHA: "ghi789",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/projects/123/merge_requests/42/versions" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(expected)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	versions, err := client.GetMRVersions(context.Background(), 123, 42)
	if err != nil {
		t.Fatalf("GetMRVersions failed: %v", err)
	}

	if len(versions) != 1 {
		t.Fatalf("expected 1 version, got %d", len(versions))
	}
	if versions[0].HeadCommitSHA != "abc123" {
		t.Errorf("expected HeadCommitSHA 'abc123', got %q", versions[0].HeadCommitSHA)
	}
}

func TestClient_GetFileContent(t *testing.T) {
	expectedContent := "package main\n\nfunc main() {}\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("ref") != "main" {
			t.Errorf("expected ref=main, got %q", r.URL.Query().Get("ref"))
		}
		w.Write([]byte(expectedContent))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	content, err := client.GetFileContent(context.Background(), 123, "main.go", "main")
	if err != nil {
		t.Fatalf("GetFileContent failed: %v", err)
	}

	if content != expectedContent {
		t.Errorf("expected content %q, got %q", expectedContent, content)
	}
}

func TestClient_PostReviewSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Note{ID: 1})
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "test-token",
	})

	err := client.PostReviewSummary(context.Background(), 123, 42, "Summary")
	if err != nil {
		t.Fatalf("PostReviewSummary failed: %v", err)
	}
}

func TestMRChange_Fields(t *testing.T) {
	change := MRChange{
		OldPath:     "old.go",
		NewPath:     "new.go",
		Diff:        "+line",
		NewFile:     true,
		RenamedFile: false,
		DeletedFile: false,
	}

	if change.NewPath != "new.go" {
		t.Errorf("expected NewPath 'new.go', got %q", change.NewPath)
	}
	if !change.NewFile {
		t.Error("expected NewFile to be true")
	}
}
