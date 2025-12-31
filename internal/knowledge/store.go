package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yourorg/code-reviewer/internal/indexer"
)

// Store manages the indexed code knowledge base
type Store struct {
	dataDir  string
	elements []indexer.CodeElement
	index    map[string][]int // keyword -> element indices
}

// Framework represents indexed framework information
type Framework struct {
	Name        string                  `json:"name"`
	RepoURL     string                  `json:"repo_url"`
	LocalPath   string                  `json:"local_path"`
	ElementCount int                    `json:"element_count"`
	Packages    []string                `json:"packages"`
	Elements    []indexer.CodeElement  `json:"elements"`
}

// NewStore creates a new knowledge store
func NewStore(dataDir string) *Store {
	if dataDir == "" {
		homeDir, _ := os.UserHomeDir()
		dataDir = filepath.Join(homeDir, ".code-reviewer", "knowledge")
	}
	return &Store{
		dataDir: dataDir,
		index:   make(map[string][]int),
	}
}

// SaveFramework saves indexed framework data
func (s *Store) SaveFramework(name string, repoURL, localPath string, elements []indexer.CodeElement) error {
	if err := os.MkdirAll(s.dataDir, 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	// Extract unique packages
	pkgSet := make(map[string]bool)
	for _, elem := range elements {
		pkgSet[elem.Package] = true
	}
	packages := make([]string, 0, len(pkgSet))
	for pkg := range pkgSet {
		packages = append(packages, pkg)
	}
	sort.Strings(packages)

	framework := Framework{
		Name:         name,
		RepoURL:      repoURL,
		LocalPath:    localPath,
		ElementCount: len(elements),
		Packages:     packages,
		Elements:     elements,
	}

	filePath := filepath.Join(s.dataDir, name+".json")
	data, err := json.MarshalIndent(framework, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal framework: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	fmt.Printf("Saved framework '%s' with %d elements to %s\n", name, len(elements), filePath)
	return nil
}

// LoadFramework loads a framework by name
func (s *Store) LoadFramework(name string) (*Framework, error) {
	filePath := filepath.Join(s.dataDir, name+".json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	var framework Framework
	if err := json.Unmarshal(data, &framework); err != nil {
		return nil, fmt.Errorf("unmarshal framework: %w", err)
	}

	// Build index
	s.elements = framework.Elements
	s.buildIndex()

	return &framework, nil
}

// ListFrameworks lists all indexed frameworks
func (s *Store) ListFrameworks() ([]string, error) {
	if _, err := os.Stat(s.dataDir); os.IsNotExist(err) {
		return nil, nil
	}

	entries, err := os.ReadDir(s.dataDir)
	if err != nil {
		return nil, err
	}

	var frameworks []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			name := strings.TrimSuffix(entry.Name(), ".json")
			frameworks = append(frameworks, name)
		}
	}

	return frameworks, nil
}

// buildIndex creates a keyword index for fast lookup
func (s *Store) buildIndex() {
	s.index = make(map[string][]int)

	for i, elem := range s.elements {
		// Index by name
		s.addToIndex(strings.ToLower(elem.Name), i)

		// Index by words in name (camelCase split)
		for _, word := range splitCamelCase(elem.Name) {
			s.addToIndex(strings.ToLower(word), i)
		}

		// Index by package
		s.addToIndex(strings.ToLower(elem.Package), i)

		// Index by type
		s.addToIndex(elem.Type, i)

		// Index by words in doc
		if elem.Doc != "" {
			for _, word := range strings.Fields(elem.Doc) {
				word = strings.ToLower(strings.Trim(word, ".,;:!?()[]{}"))
				if len(word) > 3 {
					s.addToIndex(word, i)
				}
			}
		}
	}
}

func (s *Store) addToIndex(key string, idx int) {
	s.index[key] = append(s.index[key], idx)
}

// Search finds relevant code elements based on query
func (s *Store) Search(query string, limit int) []indexer.CodeElement {
	if limit <= 0 {
		limit = 10
	}

	// Score each element
	scores := make(map[int]int)
	words := strings.Fields(strings.ToLower(query))

	for _, word := range words {
		word = strings.Trim(word, ".,;:!?()[]{}\"'")

		// Exact match
		if indices, ok := s.index[word]; ok {
			for _, idx := range indices {
				scores[idx] += 10
			}
		}

		// Partial match
		for key, indices := range s.index {
			if strings.Contains(key, word) || strings.Contains(word, key) {
				for _, idx := range indices {
					scores[idx] += 3
				}
			}
		}
	}

	// Sort by score
	type scored struct {
		idx   int
		score int
	}
	var scoredElements []scored
	for idx, score := range scores {
		scoredElements = append(scoredElements, scored{idx, score})
	}
	sort.Slice(scoredElements, func(i, j int) bool {
		return scoredElements[i].score > scoredElements[j].score
	})

	// Return top results
	var results []indexer.CodeElement
	seen := make(map[string]bool)
	for _, se := range scoredElements {
		if len(results) >= limit {
			break
		}
		elem := s.elements[se.idx]
		key := elem.Package + "." + elem.Name
		if !seen[key] {
			seen[key] = true
			results = append(results, elem)
		}
	}

	return results
}

// SearchByType finds elements of a specific type
func (s *Store) SearchByType(elemType string, limit int) []indexer.CodeElement {
	if limit <= 0 {
		limit = 20
	}

	var results []indexer.CodeElement
	for _, elem := range s.elements {
		if elem.Type == elemType {
			results = append(results, elem)
			if len(results) >= limit {
				break
			}
		}
	}
	return results
}

// GetPatterns extracts common patterns from the framework
func (s *Store) GetPatterns() map[string][]indexer.CodeElement {
	patterns := make(map[string][]indexer.CodeElement)

	for _, elem := range s.elements {
		// Group by type
		patterns[elem.Type] = append(patterns[elem.Type], elem)

		// Identify patterns by naming conventions
		name := elem.Name
		switch {
		case strings.HasPrefix(name, "New"):
			patterns["constructor"] = append(patterns["constructor"], elem)
		case strings.HasPrefix(name, "Get"):
			patterns["getter"] = append(patterns["getter"], elem)
		case strings.HasPrefix(name, "Set"):
			patterns["setter"] = append(patterns["setter"], elem)
		case strings.HasPrefix(name, "Is") || strings.HasPrefix(name, "Has"):
			patterns["predicate"] = append(patterns["predicate"], elem)
		case strings.HasSuffix(name, "Handler"):
			patterns["handler"] = append(patterns["handler"], elem)
		case strings.HasSuffix(name, "Service"):
			patterns["service"] = append(patterns["service"], elem)
		case strings.HasSuffix(name, "Repository"):
			patterns["repository"] = append(patterns["repository"], elem)
		case strings.HasSuffix(name, "Error"):
			patterns["error"] = append(patterns["error"], elem)
		}
	}

	return patterns
}

// splitCamelCase splits a camelCase string into words
func splitCamelCase(s string) []string {
	var words []string
	var current strings.Builder

	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
		}
		current.WriteRune(r)
	}

	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}
