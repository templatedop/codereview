// Package postgres provides PostgreSQL-specific code analysis
// with RAG-based knowledge retrieval.
package postgres

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/yourorg/code-reviewer/internal/rag"
)

// Category represents a knowledge base category.
type Category string

const (
	CategoryPerformance Category = "performance"
	CategorySecurity    Category = "security"
	CategoryStandards   Category = "standards"
	CategoryMigrations  Category = "migrations"
)

// AllCategories returns all available categories.
func AllCategories() []Category {
	return []Category{
		CategoryPerformance,
		CategorySecurity,
		CategoryStandards,
		CategoryMigrations,
	}
}

// CollectionName returns the vector store collection name for a category.
func CollectionName(category Category) string {
	return "postgres_" + string(category)
}

// Ingester handles ingestion of PostgreSQL knowledge documents.
type Ingester struct {
	embedder    rag.Embedder
	vectorStore rag.VectorStore
	chunker     *rag.Chunker
	workDir     string
}

// NewIngester creates a new knowledge ingester.
func NewIngester(embedder rag.Embedder, vectorStore rag.VectorStore, workDir string) *Ingester {
	cfg := rag.DefaultChunkConfig()
	cfg.ChunkSize = 500
	cfg.ChunkOverlap = 50
	return &Ingester{
		embedder:    embedder,
		vectorStore: vectorStore,
		chunker:     rag.NewChunker(cfg),
		workDir:     workDir,
	}
}

// IngestSource defines a knowledge source to ingest.
type IngestSource struct {
	Name     string
	RepoURL  string
	Branch   string
	PathGlob string
}

// DefaultSources returns the default postgres-ai sources.
func DefaultSources() []IngestSource {
	return []IngestSource{
		{
			Name:     "postgres-howtos",
			RepoURL:  "https://github.com/postgres-ai/postgres-howtos.git",
			Branch:   "main",
			PathGlob: "*.md",
		},
		{
			Name:     "postgres-checklist",
			RepoURL:  "https://github.com/postgres-ai/postgres-checklist.git",
			Branch:   "main",
			PathGlob: "**/*.md",
		},
	}
}

// IngestAll ingests all default sources.
func (i *Ingester) IngestAll(ctx context.Context) error {
	sources := DefaultSources()
	for _, source := range sources {
		if err := i.IngestSource(ctx, source); err != nil {
			return fmt.Errorf("ingest %s: %w", source.Name, err)
		}
	}
	return nil
}

// IngestSource ingests a single knowledge source.
func (i *Ingester) IngestSource(ctx context.Context, source IngestSource) error {
	// Clone or update repository
	repoPath := filepath.Join(i.workDir, source.Name)
	if err := i.cloneOrPull(ctx, source.RepoURL, source.Branch, repoPath); err != nil {
		return fmt.Errorf("clone/pull: %w", err)
	}

	// Find matching files
	pattern := filepath.Join(repoPath, source.PathGlob)
	files, err := filepath.Glob(pattern)
	if err != nil {
		return fmt.Errorf("glob files: %w", err)
	}

	// Also check subdirectories for ** patterns
	if strings.Contains(source.PathGlob, "**") {
		files, err = i.walkGlob(repoPath, source.PathGlob)
		if err != nil {
			return fmt.Errorf("walk glob: %w", err)
		}
	}

	// Process each file
	for _, file := range files {
		if err := i.ingestFile(ctx, source.Name, repoPath, file); err != nil {
			// Log but continue with other files
			fmt.Printf("Warning: failed to ingest %s: %v\n", file, err)
		}
	}

	return nil
}

// IngestDirectory ingests all markdown files from a local directory.
func (i *Ingester) IngestDirectory(ctx context.Context, sourceName, dirPath string) error {
	return filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".markdown" && ext != ".sql" {
			return nil
		}
		return i.ingestFile(ctx, sourceName, dirPath, path)
	})
}

// IngestOrgStandards ingests organization-specific PostgreSQL standards.
func (i *Ingester) IngestOrgStandards(ctx context.Context, orgName, standardsPath string) error {
	return i.IngestDirectory(ctx, "org-"+orgName, standardsPath)
}

func (i *Ingester) cloneOrPull(ctx context.Context, repoURL, branch, destPath string) error {
	if _, err := os.Stat(destPath); os.IsNotExist(err) {
		// Clone
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", "--branch", branch, repoURL, destPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}

	// Pull latest
	cmd := exec.CommandContext(ctx, "git", "-C", destPath, "pull", "origin", branch)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (i *Ingester) walkGlob(root, pattern string) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Simple pattern matching - check extension
		if strings.HasSuffix(pattern, ".md") && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func (i *Ingester) ingestFile(ctx context.Context, sourceName, basePath, filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	text := string(content)
	if len(text) < 50 {
		return nil // Skip very short files
	}

	// Determine category based on content
	category := categorizeDocument(filePath, text)

	// Build document metadata
	relPath, _ := filepath.Rel(basePath, filePath)
	metadata := map[string]string{
		"file":   relPath,
		"source": sourceName,
	}

	// Extract title from markdown
	if title := extractMarkdownTitle(text); title != "" {
		metadata["title"] = title
	}

	// Chunk the document
	var chunks []rag.Chunk
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".sql" {
		chunks = i.chunker.ChunkSQL(text)
	} else {
		chunks = i.chunker.ChunkMarkdown(text)
	}

	// Create and index each chunk
	collection := CollectionName(category)
	for idx, chunk := range chunks {
		docID := fmt.Sprintf("%s_%s_%d", sourceName, sanitizeID(relPath), idx)

		embedding, err := i.embedder.Embed(ctx, chunk.Content)
		if err != nil {
			return fmt.Errorf("embed chunk %d: %w", idx, err)
		}

		doc := rag.Document{
			ID:         docID,
			Content:    chunk.Content,
			Embedding:  embedding,
			Source:     sourceName,
			Category:   string(category),
			Metadata:   metadata,
			ChunkIndex: idx,
		}

		if err := i.vectorStore.Insert(ctx, collection, doc); err != nil {
			return fmt.Errorf("insert chunk %d: %w", idx, err)
		}
	}

	return nil
}

// categorizeDocument determines the category of a document based on its content.
func categorizeDocument(path, content string) Category {
	contentLower := strings.ToLower(content)
	pathLower := strings.ToLower(path)

	// Check path first for explicit categorization
	if strings.Contains(pathLower, "security") ||
		strings.Contains(pathLower, "auth") ||
		strings.Contains(pathLower, "permission") {
		return CategorySecurity
	}
	if strings.Contains(pathLower, "migration") ||
		strings.Contains(pathLower, "alter") ||
		strings.Contains(pathLower, "upgrade") {
		return CategoryMigrations
	}
	if strings.Contains(pathLower, "naming") ||
		strings.Contains(pathLower, "convention") ||
		strings.Contains(pathLower, "standard") {
		return CategoryStandards
	}

	// Check content for categorization
	performanceKeywords := []string{
		"index", "explain", "vacuum", "analyze", "performance",
		"slow", "optimize", "query plan", "seq scan", "execution time",
		"buffer", "cache", "partitioning", "parallel",
	}
	securityKeywords := []string{
		"security", "permission", "grant", "revoke", "role",
		"rls", "row level", "authentication", "authorization",
		"sql injection", "credential", "password", "ssl", "tls",
	}
	migrationKeywords := []string{
		"migration", "alter table", "add column", "drop column",
		"rename", "constraint", "concurrent", "blocking",
		"backward compatible", "rollback",
	}
	standardsKeywords := []string{
		"naming", "convention", "best practice", "pattern",
		"anti-pattern", "schema design", "data type", "timestamp",
		"uuid", "serial", "primary key", "foreign key",
	}

	scores := map[Category]int{
		CategoryPerformance: 0,
		CategorySecurity:    0,
		CategoryMigrations:  0,
		CategoryStandards:   0,
	}

	for _, kw := range performanceKeywords {
		if strings.Contains(contentLower, kw) {
			scores[CategoryPerformance]++
		}
	}
	for _, kw := range securityKeywords {
		if strings.Contains(contentLower, kw) {
			scores[CategorySecurity]++
		}
	}
	for _, kw := range migrationKeywords {
		if strings.Contains(contentLower, kw) {
			scores[CategoryMigrations]++
		}
	}
	for _, kw := range standardsKeywords {
		if strings.Contains(contentLower, kw) {
			scores[CategoryStandards]++
		}
	}

	// Find highest scoring category
	maxScore := 0
	maxCategory := CategoryStandards
	for cat, score := range scores {
		if score > maxScore {
			maxScore = score
			maxCategory = cat
		}
	}

	return maxCategory
}

// extractMarkdownTitle extracts the first H1 heading from markdown.
func extractMarkdownTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "# ") {
			return strings.TrimPrefix(line, "# ")
		}
	}
	return ""
}

// sanitizeID creates a safe ID string from a path.
func sanitizeID(path string) string {
	path = strings.ReplaceAll(path, "/", "_")
	path = strings.ReplaceAll(path, "\\", "_")
	path = strings.ReplaceAll(path, " ", "_")
	path = strings.ReplaceAll(path, ".", "_")
	return path
}
