package rag

import (
	"strings"
	"testing"
)

func TestChunker_ChunkText(t *testing.T) {
	tests := []struct {
		name       string
		config     ChunkConfig
		input      string
		wantChunks int
		wantFirst  string
	}{
		{
			name:       "empty input",
			config:     DefaultChunkConfig(),
			input:      "",
			wantChunks: 0,
		},
		{
			name:       "short text single chunk",
			config:     DefaultChunkConfig(),
			input:      "This is a short text that fits in one chunk.",
			wantChunks: 1,
			wantFirst:  "This is a short text that fits in one chunk.",
		},
		{
			name: "text with multiple sections",
			config: ChunkConfig{
				ChunkSize:    500,
				ChunkOverlap: 50,
				MinChunkSize: 100,
			},
			input: "First section content here.\n\nSecond section content here.\n\nThird section content here.",
			wantChunks: 1, // All sections fit in one chunk
		},
		{
			name:   "text with paragraph separators",
			config: DefaultChunkConfig(),
			input: `First paragraph with some content.

Second paragraph with more content.

Third paragraph with even more content.`,
			wantChunks: 1, // All fits in one chunk with default config
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewChunker(tt.config)
			chunks := c.ChunkText(tt.input)

			if len(chunks) != tt.wantChunks {
				t.Errorf("ChunkText() got %d chunks, want %d", len(chunks), tt.wantChunks)
			}

			if tt.wantFirst != "" && len(chunks) > 0 {
				if chunks[0].Content != tt.wantFirst {
					t.Errorf("ChunkText() first chunk = %q, want %q", chunks[0].Content, tt.wantFirst)
				}
			}

			// Verify chunk indices are sequential
			for i, chunk := range chunks {
				if chunk.Index != i {
					t.Errorf("Chunk index = %d, want %d", chunk.Index, i)
				}
			}
		})
	}
}

func TestChunker_ChunkMarkdown(t *testing.T) {
	input := `# Header 1

This is content under header 1.

## Header 2

This is content under header 2.

### Header 3

This is content under header 3.
`

	c := NewChunker(DefaultChunkConfig())
	chunks := c.ChunkMarkdown(input)

	if len(chunks) == 0 {
		t.Error("ChunkMarkdown() returned no chunks")
	}

	// Verify content is preserved
	var combined string
	for _, chunk := range chunks {
		combined += chunk.Content + "\n"
	}

	// Check that headers are present
	if !strings.Contains(combined, "Header 1") {
		t.Error("ChunkMarkdown() lost Header 1")
	}
	if !strings.Contains(combined, "Header 2") {
		t.Error("ChunkMarkdown() lost Header 2")
	}
}

func TestChunker_ChunkSQL(t *testing.T) {
	input := `CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO users (name) VALUES ('test');

SELECT * FROM users WHERE id = 1;
`

	c := NewChunker(ChunkConfig{
		ChunkSize:    100,
		ChunkOverlap: 10,
		MinChunkSize: 20,
	})
	chunks := c.ChunkSQL(input)

	if len(chunks) == 0 {
		t.Error("ChunkSQL() returned no chunks")
	}

	// Verify SQL statements are preserved
	var combined string
	for _, chunk := range chunks {
		combined += chunk.Content
	}

	if !strings.Contains(combined, "CREATE TABLE") {
		t.Error("ChunkSQL() lost CREATE TABLE statement")
	}
	if !strings.Contains(combined, "INSERT INTO") {
		t.Error("ChunkSQL() lost INSERT statement")
	}
	if !strings.Contains(combined, "SELECT") {
		t.Error("ChunkSQL() lost SELECT statement")
	}
}

func TestCountTokens(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"", 0},
		{"test", 1},
		{"This is a test", 3},
		{strings.Repeat("a", 100), 25},
	}

	for _, tt := range tests {
		got := CountTokens(tt.text)
		if got != tt.expected {
			t.Errorf("CountTokens(%q) = %d, want %d", tt.text, got, tt.expected)
		}
	}
}

func TestDefaultChunkConfig(t *testing.T) {
	cfg := DefaultChunkConfig()

	if cfg.ChunkSize != 500 {
		t.Errorf("ChunkSize = %d, want 500", cfg.ChunkSize)
	}
	if cfg.ChunkOverlap != 50 {
		t.Errorf("ChunkOverlap = %d, want 50", cfg.ChunkOverlap)
	}
	if cfg.MinChunkSize != 100 {
		t.Errorf("MinChunkSize = %d, want 100", cfg.MinChunkSize)
	}
}
