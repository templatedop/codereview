package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles communication with GitLab API
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// Config holds configuration for the GitLab client
type Config struct {
	BaseURL string        // e.g., "https://gitlab.yourcompany.com"
	Token   string        // GitLab personal access token
	Timeout time.Duration // Request timeout
}

// NewClient creates a new GitLab client
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}
	return &Client{
		baseURL: cfg.BaseURL,
		token:   cfg.Token,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// MergeRequest represents a GitLab merge request
type MergeRequest struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	State        string `json:"state"`
	WebURL       string `json:"web_url"`
}

// MRChange represents a file change in a merge request
type MRChange struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

// MRChanges represents the changes in a merge request
type MRChanges struct {
	MergeRequest
	Changes []MRChange `json:"changes"`
}

// Discussion represents a GitLab discussion
type Discussion struct {
	ID    string `json:"id"`
	Notes []Note `json:"notes"`
}

// Note represents a comment/note on a merge request
type Note struct {
	ID        int       `json:"id"`
	Body      string    `json:"body"`
	Author    Author    `json:"author"`
	CreatedAt time.Time `json:"created_at"`
}

// Author represents a GitLab user
type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

// GetMergeRequest fetches a merge request by project and MR ID
func (c *Client) GetMergeRequest(ctx context.Context, projectID int, mrIID int) (*MergeRequest, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d", c.baseURL, projectID, mrIID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab error (status %d): %s", resp.StatusCode, string(body))
	}

	var mr MergeRequest
	if err := json.NewDecoder(resp.Body).Decode(&mr); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &mr, nil
}

// GetMergeRequestChanges fetches the changes/diff for a merge request
func (c *Client) GetMergeRequestChanges(ctx context.Context, projectID int, mrIID int) (*MRChanges, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/changes", c.baseURL, projectID, mrIID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab error (status %d): %s", resp.StatusCode, string(body))
	}

	var changes MRChanges
	if err := json.NewDecoder(resp.Body).Decode(&changes); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &changes, nil
}

// CreateMRNote creates a comment on a merge request
func (c *Client) CreateMRNote(ctx context.Context, projectID int, mrIID int, body string) (*Note, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/notes", c.baseURL, projectID, mrIID)

	payload := map[string]string{"body": body}
	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var note Note
	if err := json.NewDecoder(resp.Body).Decode(&note); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &note, nil
}

// CreateDiscussion creates a new discussion on a merge request with position
type DiscussionPosition struct {
	BaseSHA      string `json:"base_sha"`
	StartSHA     string `json:"start_sha"`
	HeadSHA      string `json:"head_sha"`
	PositionType string `json:"position_type"`
	NewPath      string `json:"new_path,omitempty"`
	NewLine      int    `json:"new_line,omitempty"`
	OldPath      string `json:"old_path,omitempty"`
	OldLine      int    `json:"old_line,omitempty"`
}

// CreateDiscussion creates a discussion on a specific line of a merge request
func (c *Client) CreateDiscussion(ctx context.Context, projectID int, mrIID int, body string, position *DiscussionPosition) (*Discussion, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/merge_requests/%d/discussions", c.baseURL, projectID, mrIID)

	payload := map[string]interface{}{
		"body": body,
	}
	if position != nil {
		payload["position"] = position
	}

	payloadBytes, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("gitlab error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var discussion Discussion
	if err := json.NewDecoder(resp.Body).Decode(&discussion); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &discussion, nil
}

// GetFileContent fetches the content of a file from the repository
func (c *Client) GetFileContent(ctx context.Context, projectID int, filePath string, ref string) (string, error) {
	url := fmt.Sprintf("%s/api/v4/projects/%d/repository/files/%s/raw?ref=%s",
		c.baseURL, projectID, filePath, ref)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("gitlab error (status %d): %s", resp.StatusCode, string(body))
	}

	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return string(content), nil
}
