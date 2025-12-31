package github

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RepoClient handles GitHub repository operations
type RepoClient struct {
	cacheDir string
}

// NewRepoClient creates a new repository client
func NewRepoClient(cacheDir string) *RepoClient {
	if cacheDir == "" {
		cacheDir = filepath.Join(os.TempDir(), "code-reviewer-repos")
	}
	return &RepoClient{cacheDir: cacheDir}
}

// CloneOrUpdate clones a repo or updates it if already exists
// repoURL can be:
//   - https://github.com/owner/repo
//   - github.com/owner/repo
//   - owner/repo
func (c *RepoClient) CloneOrUpdate(ctx context.Context, repoURL string) (string, error) {
	// Normalize URL
	repoURL = normalizeGitHubURL(repoURL)

	// Extract repo name for local path
	repoName := extractRepoName(repoURL)
	localPath := filepath.Join(c.cacheDir, repoName)

	// Check if already cloned
	if _, err := os.Stat(filepath.Join(localPath, ".git")); err == nil {
		// Already exists, pull latest
		fmt.Printf("Updating existing repo: %s\n", repoName)
		cmd := exec.CommandContext(ctx, "git", "-C", localPath, "pull", "--ff-only")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			// Pull failed, try to continue with existing code
			fmt.Printf("Warning: git pull failed: %v (using existing code)\n", err)
		}
		return localPath, nil
	}

	// Clone the repo
	fmt.Printf("Cloning repo: %s\n", repoURL)
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, localPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git clone: %w", err)
	}

	return localPath, nil
}

// GetLocalPath returns the local path for a repo without cloning
func (c *RepoClient) GetLocalPath(repoURL string) string {
	repoURL = normalizeGitHubURL(repoURL)
	repoName := extractRepoName(repoURL)
	return filepath.Join(c.cacheDir, repoName)
}

// normalizeGitHubURL converts various GitHub URL formats to https URL
func normalizeGitHubURL(url string) string {
	url = strings.TrimSpace(url)
	url = strings.TrimSuffix(url, ".git")

	// Handle different formats
	if strings.HasPrefix(url, "git@github.com:") {
		// git@github.com:owner/repo -> https://github.com/owner/repo
		url = strings.TrimPrefix(url, "git@github.com:")
		url = "https://github.com/" + url
	} else if strings.HasPrefix(url, "github.com/") {
		url = "https://" + url
	} else if !strings.HasPrefix(url, "http") && strings.Contains(url, "/") {
		// owner/repo format
		url = "https://github.com/" + url
	}

	return url
}

// extractRepoName gets "owner-repo" from URL
func extractRepoName(url string) string {
	url = strings.TrimSuffix(url, ".git")
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		owner := parts[len(parts)-2]
		repo := parts[len(parts)-1]
		return owner + "-" + repo
	}
	return filepath.Base(url)
}
