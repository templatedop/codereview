package postgres

import (
	"path/filepath"
	"regexp"
	"strings"
)

// SQLFragment represents an extracted SQL query with context.
type SQLFragment struct {
	Query       string   // The SQL query
	Line        int      // Line number in source file
	EndLine     int      // End line number
	File        string   // Source file path
	Language    string   // Source language (go, python, etc.)
	Context     string   // Surrounding code context
	QueryType   SQLType  // Type of SQL query
	Tables      []string // Tables referenced
	IsRaw       bool     // Whether this is raw/dynamic SQL
	IsDynamic   bool     // Whether query contains dynamic parts
	Placeholder string   // Placeholder style ($1, ?, :name)
}

// SQLType represents the type of SQL statement.
type SQLType string

const (
	SQLTypeSelect   SQLType = "SELECT"
	SQLTypeInsert   SQLType = "INSERT"
	SQLTypeUpdate   SQLType = "UPDATE"
	SQLTypeDelete   SQLType = "DELETE"
	SQLTypeCreate   SQLType = "CREATE"
	SQLTypeAlter    SQLType = "ALTER"
	SQLTypeDrop     SQLType = "DROP"
	SQLTypeGrant    SQLType = "GRANT"
	SQLTypeRevoke   SQLType = "REVOKE"
	SQLTypeTruncate SQLType = "TRUNCATE"
	SQLTypeOther    SQLType = "OTHER"
)

// SQLExtractor extracts SQL queries from source code.
type SQLExtractor struct {
	patterns map[string][]extractionPattern
}

type extractionPattern struct {
	regex       *regexp.Regexp
	groupIndex  int
	isDynamic   bool
	placeholder string
}

// NewSQLExtractor creates a new SQL extractor.
func NewSQLExtractor() *SQLExtractor {
	e := &SQLExtractor{
		patterns: make(map[string][]extractionPattern),
	}
	e.initPatterns()
	return e
}

func (e *SQLExtractor) initPatterns() {
	// Go patterns
	e.patterns["go"] = []extractionPattern{
		// db.Query/Exec with string literal
		{
			regex:       regexp.MustCompile("(?s)(?:db|tx|conn|rows)\\s*\\.\\s*(?:Query|QueryRow|Exec|QueryContext|ExecContext|QueryRowContext)\\s*\\([^,]*,?\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "$",
		},
		// db.Query/Exec with double-quoted string (less common)
		{
			regex:       regexp.MustCompile(`(?:db|tx|conn|rows)\s*\.\s*(?:Query|QueryRow|Exec|QueryContext|ExecContext|QueryRowContext)\s*\([^,]*,?\s*"([^"]+)"`),
			groupIndex:  1,
			placeholder: "$",
		},
		// sqlx NamedExec/NamedQuery
		{
			regex:       regexp.MustCompile("(?s)(?:db|tx)\\s*\\.\\s*(?:NamedExec|NamedQuery|NamedExecContext|NamedQueryContext)\\s*\\([^,]*,?\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: ":",
		},
		// GORM Raw queries
		{
			regex:       regexp.MustCompile("(?s)\\.\\s*Raw\\s*\\(\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "?",
		},
		// GORM Exec
		{
			regex:       regexp.MustCompile("(?s)\\.\\s*Exec\\s*\\(\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "?",
		},
		// pgx queries
		{
			regex:       regexp.MustCompile("(?s)(?:pool|conn)\\s*\\.\\s*(?:Query|QueryRow|Exec)\\s*\\([^,]*,?\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "$",
		},
		// String concatenation (dynamic SQL indicator)
		{
			regex:      regexp.MustCompile(`(?:db|tx|conn)\s*\.\s*(?:Query|Exec)[^(]*\([^)]*\+[^)]*\)`),
			groupIndex: 0,
			isDynamic:  true,
		},
		// fmt.Sprintf with SQL
		{
			regex:      regexp.MustCompile(`fmt\.Sprintf\s*\(\s*"([^"]*(?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP)[^"]*)"`),
			groupIndex: 1,
			isDynamic:  true,
		},
		// Constant definitions with SQL
		{
			regex:       regexp.MustCompile("(?s)(?:const|var)\\s+\\w+\\s*=\\s*`((?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|WITH)[^`]+)`"),
			groupIndex:  1,
			placeholder: "$",
		},
	}

	// Python patterns
	e.patterns["python"] = []extractionPattern{
		// cursor.execute with triple quotes
		{
			regex:       regexp.MustCompile(`(?s)(?:cursor|conn|db|session)\s*\.\s*(?:execute|executemany|fetchall|fetchone)\s*\(\s*"""([^"]+)"""`),
			groupIndex:  1,
			placeholder: "%s",
		},
		// cursor.execute with single quotes
		{
			regex:       regexp.MustCompile(`(?:cursor|conn|db|session)\s*\.\s*(?:execute|executemany)\s*\(\s*'([^']+)'`),
			groupIndex:  1,
			placeholder: "%s",
		},
		// SQLAlchemy text()
		{
			regex:       regexp.MustCompile(`(?s)text\s*\(\s*"""([^"]+)"""`),
			groupIndex:  1,
			placeholder: ":",
		},
		// f-string with SQL (dynamic, potentially unsafe)
		{
			regex:      regexp.MustCompile(`f"[^"]*(?:SELECT|INSERT|UPDATE|DELETE)[^"]*\{`),
			groupIndex: 0,
			isDynamic:  true,
		},
		// .format() with SQL (dynamic)
		{
			regex:      regexp.MustCompile(`"[^"]*(?:SELECT|INSERT|UPDATE|DELETE)[^"]*"\.format\(`),
			groupIndex: 0,
			isDynamic:  true,
		},
	}

	// Java patterns
	e.patterns["java"] = []extractionPattern{
		// PreparedStatement with SQL string
		{
			regex:       regexp.MustCompile(`(?:prepareStatement|createStatement|executeQuery|executeUpdate)\s*\(\s*"([^"]+)"`),
			groupIndex:  1,
			placeholder: "?",
		},
		// JPA @Query annotation
		{
			regex:       regexp.MustCompile(`@Query\s*\(\s*(?:value\s*=\s*)?"([^"]+)"`),
			groupIndex:  1,
			placeholder: "?",
		},
		// String concatenation in SQL
		{
			regex:      regexp.MustCompile(`(?:executeQuery|executeUpdate)\s*\([^)]*\+[^)]*\)`),
			groupIndex: 0,
			isDynamic:  true,
		},
	}

	// JavaScript/TypeScript patterns
	e.patterns["javascript"] = []extractionPattern{
		// Template literal SQL
		{
			regex:       regexp.MustCompile("(?s)(?:query|execute|raw|sql)\\s*\\(\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "$",
		},
		// String SQL
		{
			regex:       regexp.MustCompile(`(?:query|execute|raw)\s*\(\s*"([^"]+)"`),
			groupIndex:  1,
			placeholder: "$",
		},
		// Knex/Prisma raw
		{
			regex:       regexp.MustCompile("(?s)\\.\\s*(?:raw|\\$queryRaw)\\s*`([^`]+)`"),
			groupIndex:  1,
			placeholder: "$",
		},
	}
	e.patterns["typescript"] = e.patterns["javascript"]

	// Ruby patterns
	e.patterns["ruby"] = []extractionPattern{
		// ActiveRecord find_by_sql
		{
			regex:       regexp.MustCompile(`(?:find_by_sql|execute|select_all)\s*\(\s*["']([^"']+)["']`),
			groupIndex:  1,
			placeholder: "$",
		},
		// Here doc SQL
		{
			regex:       regexp.MustCompile(`(?s)<<[-~]?SQL\n([^E]+)SQL`),
			groupIndex:  1,
			placeholder: "$",
		},
		// String interpolation (dynamic)
		{
			regex:      regexp.MustCompile(`(?:find_by_sql|execute)\s*\([^)]*#\{[^}]+\}[^)]*\)`),
			groupIndex: 0,
			isDynamic:  true,
		},
	}

	// PHP patterns
	e.patterns["php"] = []extractionPattern{
		// PDO prepare
		{
			regex:       regexp.MustCompile(`(?:prepare|query|exec)\s*\(\s*["']([^"']+)["']`),
			groupIndex:  1,
			placeholder: ":",
		},
		// Variable interpolation (dynamic)
		{
			regex:      regexp.MustCompile(`(?:query|exec)\s*\(\s*"[^"]*\$[^"]*"`),
			groupIndex: 0,
			isDynamic:  true,
		},
	}

	// SQL file patterns (direct SQL)
	e.patterns["sql"] = []extractionPattern{
		// Match entire SQL statements
		{
			regex:      regexp.MustCompile(`(?si)((?:SELECT|INSERT|UPDATE|DELETE|CREATE|ALTER|DROP|GRANT|REVOKE|TRUNCATE|WITH)\s+[^;]+;?)`),
			groupIndex: 1,
		},
	}
}

// Extract extracts SQL fragments from source code.
func (e *SQLExtractor) Extract(code, language, filePath string) []SQLFragment {
	if language == "" {
		language = detectLanguage(filePath)
	}
	language = strings.ToLower(language)

	patterns, ok := e.patterns[language]
	if !ok {
		// Try generic patterns
		patterns = e.patterns["go"] // Go patterns work for many languages
	}

	var fragments []SQLFragment
	lines := strings.Split(code, "\n")

	for _, pattern := range patterns {
		matches := pattern.regex.FindAllStringSubmatchIndex(code, -1)
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}

			var sql string
			if pattern.groupIndex > 0 && len(match) > pattern.groupIndex*2+1 {
				start := match[pattern.groupIndex*2]
				end := match[pattern.groupIndex*2+1]
				if start >= 0 && end >= 0 {
					sql = code[start:end]
				}
			} else {
				sql = code[match[0]:match[1]]
			}

			sql = cleanSQL(sql)
			if !isValidSQL(sql) {
				continue
			}

			// Calculate line numbers
			startLine := countLines(code[:match[0]]) + 1
			endLine := countLines(code[:match[1]]) + 1

			// Extract context
			contextStart := max(0, startLine-3)
			contextEnd := min(len(lines), endLine+3)
			context := strings.Join(lines[contextStart:contextEnd], "\n")

			fragment := SQLFragment{
				Query:       sql,
				Line:        startLine,
				EndLine:     endLine,
				File:        filePath,
				Language:    language,
				Context:     context,
				QueryType:   classifySQL(sql),
				Tables:      extractTables(sql),
				IsDynamic:   pattern.isDynamic || containsDynamicIndicators(sql),
				Placeholder: pattern.placeholder,
			}

			fragments = append(fragments, fragment)
		}
	}

	return deduplicateFragments(fragments)
}

// ExtractFromDiff extracts SQL fragments from a diff.
func (e *SQLExtractor) ExtractFromDiff(diff, language, filePath string) []SQLFragment {
	// Extract added lines from diff
	var addedLines []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			addedLines = append(addedLines, strings.TrimPrefix(line, "+"))
		}
	}

	addedCode := strings.Join(addedLines, "\n")
	return e.Extract(addedCode, language, filePath)
}

// Helpers

func detectLanguage(filePath string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(filePath), "."))
	switch ext {
	case "go":
		return "go"
	case "py":
		return "python"
	case "java":
		return "java"
	case "js":
		return "javascript"
	case "ts", "tsx":
		return "typescript"
	case "rb":
		return "ruby"
	case "php":
		return "php"
	case "sql", "pgsql":
		return "sql"
	default:
		return ext
	}
}

func cleanSQL(sql string) string {
	// Remove excessive whitespace
	sql = strings.TrimSpace(sql)
	sql = regexp.MustCompile(`\s+`).ReplaceAllString(sql, " ")
	// Remove line continuations
	sql = strings.ReplaceAll(sql, "\\\n", " ")
	return sql
}

func isValidSQL(sql string) bool {
	if len(sql) < 10 {
		return false
	}
	upper := strings.ToUpper(sql)
	keywords := []string{"SELECT", "INSERT", "UPDATE", "DELETE", "CREATE", "ALTER", "DROP", "GRANT", "REVOKE", "TRUNCATE", "WITH"}
	for _, kw := range keywords {
		if strings.Contains(upper, kw) {
			return true
		}
	}
	return false
}

func classifySQL(sql string) SQLType {
	upper := strings.ToUpper(strings.TrimSpace(sql))
	switch {
	case strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH"):
		return SQLTypeSelect
	case strings.HasPrefix(upper, "INSERT"):
		return SQLTypeInsert
	case strings.HasPrefix(upper, "UPDATE"):
		return SQLTypeUpdate
	case strings.HasPrefix(upper, "DELETE"):
		return SQLTypeDelete
	case strings.HasPrefix(upper, "CREATE"):
		return SQLTypeCreate
	case strings.HasPrefix(upper, "ALTER"):
		return SQLTypeAlter
	case strings.HasPrefix(upper, "DROP"):
		return SQLTypeDrop
	case strings.HasPrefix(upper, "GRANT"):
		return SQLTypeGrant
	case strings.HasPrefix(upper, "REVOKE"):
		return SQLTypeRevoke
	case strings.HasPrefix(upper, "TRUNCATE"):
		return SQLTypeTruncate
	default:
		return SQLTypeOther
	}
}

func extractTables(sql string) []string {
	var tables []string
	seen := make(map[string]bool)

	// FROM clause
	fromRe := regexp.MustCompile(`(?i)\bFROM\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\s*,\s*[a-zA-Z_][a-zA-Z0-9_]*)*)`)
	if matches := fromRe.FindStringSubmatch(sql); len(matches) > 1 {
		for _, t := range strings.Split(matches[1], ",") {
			t = strings.TrimSpace(strings.Split(t, " ")[0])
			if t != "" && !seen[t] {
				tables = append(tables, t)
				seen[t] = true
			}
		}
	}

	// JOIN clauses
	joinRe := regexp.MustCompile(`(?i)\bJOIN\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	for _, match := range joinRe.FindAllStringSubmatch(sql, -1) {
		if len(match) > 1 && !seen[match[1]] {
			tables = append(tables, match[1])
			seen[match[1]] = true
		}
	}

	// INTO clause
	intoRe := regexp.MustCompile(`(?i)\bINTO\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	if matches := intoRe.FindStringSubmatch(sql); len(matches) > 1 && !seen[matches[1]] {
		tables = append(tables, matches[1])
		seen[matches[1]] = true
	}

	// UPDATE clause
	updateRe := regexp.MustCompile(`(?i)\bUPDATE\s+([a-zA-Z_][a-zA-Z0-9_]*)`)
	if matches := updateRe.FindStringSubmatch(sql); len(matches) > 1 && !seen[matches[1]] {
		tables = append(tables, matches[1])
		seen[matches[1]] = true
	}

	return tables
}

func containsDynamicIndicators(sql string) bool {
	dynamicIndicators := []string{
		"+ ", " +", "||", "CONCAT(", "format(",
		"${", "#{", "%s", "%d", "'+", "'+",
	}
	for _, ind := range dynamicIndicators {
		if strings.Contains(sql, ind) {
			return true
		}
	}
	return false
}

func countLines(s string) int {
	return strings.Count(s, "\n")
}

func deduplicateFragments(fragments []SQLFragment) []SQLFragment {
	seen := make(map[string]bool)
	var result []SQLFragment
	for _, f := range fragments {
		key := f.Query + ":" + string(rune(f.Line))
		if !seen[key] {
			seen[key] = true
			result = append(result, f)
		}
	}
	return result
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
