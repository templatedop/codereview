package rag

import (
	"regexp"
	"strings"
	"unicode"
)

// ChunkConfig defines parameters for text chunking.
type ChunkConfig struct {
	// ChunkSize is the target number of tokens per chunk.
	ChunkSize int
	// ChunkOverlap is the number of overlapping tokens between chunks.
	ChunkOverlap int
	// MinChunkSize is the minimum chunk size to keep.
	MinChunkSize int
	// SeparatorPattern defines how to split text into initial segments.
	SeparatorPattern string
}

// DefaultChunkConfig returns sensible defaults for chunking.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		ChunkSize:        500,
		ChunkOverlap:     50,
		MinChunkSize:     100,
		SeparatorPattern: `\n\n|\n#{1,3}\s|---`,
	}
}

// Chunk represents a piece of a larger document.
type Chunk struct {
	Content    string
	Index      int
	StartChar  int
	EndChar    int
	TokenCount int
}

// Chunker splits documents into smaller pieces for embedding.
type Chunker struct {
	config ChunkConfig
}

// NewChunker creates a new chunker with the given configuration.
func NewChunker(config ChunkConfig) *Chunker {
	return &Chunker{config: config}
}

// ChunkText splits text into overlapping chunks.
func (c *Chunker) ChunkText(text string) []Chunk {
	if text == "" {
		return nil
	}

	// Normalize whitespace
	text = strings.TrimSpace(text)

	// Estimate token count (rough approximation: 1 token ≈ 4 chars)
	estimatedTokens := len(text) / 4
	if estimatedTokens <= c.config.ChunkSize {
		return []Chunk{{
			Content:    text,
			Index:      0,
			StartChar:  0,
			EndChar:    len(text),
			TokenCount: estimatedTokens,
		}}
	}

	// Split by separators first
	var segments []string
	if c.config.SeparatorPattern != "" {
		re := regexp.MustCompile(c.config.SeparatorPattern)
		segments = re.Split(text, -1)
	} else {
		segments = []string{text}
	}

	// Merge segments into chunks
	var chunks []Chunk
	var currentContent strings.Builder
	var currentStart int
	chunkIndex := 0

	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}

		segmentTokens := len(segment) / 4
		currentTokens := currentContent.Len() / 4

		// If adding this segment would exceed chunk size, finalize current chunk
		if currentTokens+segmentTokens > c.config.ChunkSize && currentContent.Len() > 0 {
			content := currentContent.String()
			chunks = append(chunks, Chunk{
				Content:    content,
				Index:      chunkIndex,
				StartChar:  currentStart,
				EndChar:    currentStart + len(content),
				TokenCount: len(content) / 4,
			})
			chunkIndex++

			// Start new chunk with overlap
			overlap := extractOverlap(content, c.config.ChunkOverlap)
			currentContent.Reset()
			currentContent.WriteString(overlap)
			if overlap != "" {
				currentContent.WriteString("\n\n")
			}
			currentStart = currentStart + len(content) - len(overlap)
		}

		currentContent.WriteString(segment)
		currentContent.WriteString("\n\n")
	}

	// Add remaining content
	if currentContent.Len() > c.config.MinChunkSize/4 {
		content := strings.TrimSpace(currentContent.String())
		chunks = append(chunks, Chunk{
			Content:    content,
			Index:      chunkIndex,
			StartChar:  currentStart,
			EndChar:    currentStart + len(content),
			TokenCount: len(content) / 4,
		})
	}

	return chunks
}

// ChunkMarkdown splits markdown content intelligently by headers and sections.
func (c *Chunker) ChunkMarkdown(content string) []Chunk {
	// Find header positions
	headerRe := regexp.MustCompile(`(?m)^#{1,6}\s+.+$`)
	matches := headerRe.FindAllStringIndex(content, -1)

	if len(matches) == 0 {
		return c.ChunkText(content)
	}

	// Split by headers
	var sections []string
	prevEnd := 0
	for _, match := range matches {
		if match[0] > prevEnd {
			section := content[prevEnd:match[0]]
			if strings.TrimSpace(section) != "" {
				sections = append(sections, section)
			}
		}
		prevEnd = match[0]
	}
	// Add last section
	if prevEnd < len(content) {
		sections = append(sections, content[prevEnd:])
	}

	// Process each section
	var allChunks []Chunk
	for _, section := range sections {
		sectionChunks := c.ChunkText(section)
		for _, chunk := range sectionChunks {
			chunk.Index = len(allChunks)
			allChunks = append(allChunks, chunk)
		}
	}

	return allChunks
}

// ChunkSQL splits SQL content by statements.
func (c *Chunker) ChunkSQL(content string) []Chunk {
	// Split by semicolons followed by newlines
	stmtRe := regexp.MustCompile(`;\s*\n`)
	statements := stmtRe.Split(content, -1)

	var chunks []Chunk
	var currentContent strings.Builder
	currentStart := 0
	chunkIndex := 0

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}

		stmtTokens := len(stmt) / 4
		currentTokens := currentContent.Len() / 4

		// Each complex statement gets its own chunk
		isComplexStmt := strings.Contains(strings.ToUpper(stmt), "CREATE TABLE") ||
			strings.Contains(strings.ToUpper(stmt), "CREATE FUNCTION") ||
			strings.Contains(strings.ToUpper(stmt), "CREATE PROCEDURE") ||
			strings.Contains(strings.ToUpper(stmt), "CREATE VIEW")

		if (currentTokens+stmtTokens > c.config.ChunkSize || isComplexStmt) && currentContent.Len() > 0 {
			content := currentContent.String()
			chunks = append(chunks, Chunk{
				Content:    content,
				Index:      chunkIndex,
				StartChar:  currentStart,
				EndChar:    currentStart + len(content),
				TokenCount: len(content) / 4,
			})
			chunkIndex++
			currentContent.Reset()
			currentStart = currentStart + len(content)
		}

		currentContent.WriteString(stmt)
		currentContent.WriteString(";\n\n")
	}

	if currentContent.Len() > 0 {
		content := currentContent.String()
		chunks = append(chunks, Chunk{
			Content:    content,
			Index:      chunkIndex,
			StartChar:  currentStart,
			EndChar:    currentStart + len(content),
			TokenCount: len(content) / 4,
		})
	}

	return chunks
}

// extractOverlap gets the last N tokens worth of content for overlap.
func extractOverlap(content string, tokens int) string {
	chars := tokens * 4
	if len(content) <= chars {
		return content
	}

	// Find a word boundary near the target position
	start := len(content) - chars
	for start > 0 && start < len(content) && !unicode.IsSpace(rune(content[start])) {
		start++
	}
	if start >= len(content) {
		start = len(content) - chars
	}

	return strings.TrimSpace(content[start:])
}

// CountTokens provides a rough estimate of token count.
func CountTokens(text string) int {
	// Simple approximation: ~4 characters per token on average
	return len(text) / 4
}
