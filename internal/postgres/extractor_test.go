package postgres

import (
	"testing"
)

func TestSQLExtractor_ExtractGo(t *testing.T) {
	extractor := NewSQLExtractor()

	tests := []struct {
		name          string
		code          string
		wantFragments int
		wantQuery     string
		wantDynamic   bool
	}{
		{
			name: "db.Query with backtick string",
			code: `
func getUsers(db *sql.DB) {
	rows, err := db.Query(` + "`SELECT * FROM users WHERE id = $1`" + `, userID)
}`,
			wantFragments: 1,
			wantQuery:     "SELECT * FROM users WHERE id = $1",
		},
		{
			name: "db.Exec with backtick string",
			code: `
func insertUser(db *sql.DB) {
	_, err := db.Exec(` + "`INSERT INTO users (name, email) VALUES ($1, $2)`" + `, name, email)
}`,
			wantFragments: 1,
			wantQuery:     "INSERT INTO users (name, email) VALUES ($1, $2)",
		},
		{
			name: "GORM Raw query",
			code: `
func findUser(db *gorm.DB) {
	db.Raw(` + "`SELECT * FROM users WHERE active = true`" + `).Scan(&users)
}`,
			wantFragments: 1,
			wantQuery:     "SELECT * FROM users WHERE active = true",
		},
		{
			name: "dynamic SQL with fmt.Sprintf",
			code: `
func search(db *sql.DB, table string) {
	query := fmt.Sprintf("SELECT * FROM %s WHERE id = 1", table)
	db.Query(query)
}`,
			wantFragments: 1,
			wantDynamic:   true,
		},
		{
			name: "const SQL definition",
			code: `
const getUserQuery = ` + "`SELECT id, name, email FROM users WHERE id = $1`" + `
`,
			wantFragments: 1,
			wantQuery:     "SELECT id, name, email FROM users WHERE id = $1",
		},
		{
			name:          "no SQL",
			code:          `func main() { fmt.Println("Hello") }`,
			wantFragments: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragments := extractor.Extract(tt.code, "go", "test.go")

			if len(fragments) != tt.wantFragments {
				t.Errorf("Extract() got %d fragments, want %d", len(fragments), tt.wantFragments)
				for i, f := range fragments {
					t.Logf("Fragment %d: %s", i, f.Query)
				}
				return
			}

			if tt.wantQuery != "" && len(fragments) > 0 {
				// Check if query contains expected content
				found := false
				for _, f := range fragments {
					if f.Query == tt.wantQuery || containsSQL(f.Query, tt.wantQuery) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Extract() query mismatch, got %q, want %q", fragments[0].Query, tt.wantQuery)
				}
			}

			if tt.wantDynamic && len(fragments) > 0 {
				found := false
				for _, f := range fragments {
					if f.IsDynamic {
						found = true
						break
					}
				}
				if !found {
					t.Error("Extract() expected dynamic SQL flag")
				}
			}
		})
	}
}

func TestSQLExtractor_ExtractPython(t *testing.T) {
	extractor := NewSQLExtractor()

	tests := []struct {
		name          string
		code          string
		wantFragments int
	}{
		{
			name: "cursor.execute with triple quotes",
			code: `
def get_users(cursor):
    cursor.execute("""SELECT * FROM users WHERE active = true""")
`,
			wantFragments: 1,
		},
		{
			name: "SQLAlchemy text()",
			code: `
def query(session):
    result = session.execute(text("""SELECT id FROM orders"""))
`,
			wantFragments: 1,
		},
		{
			name: "f-string SQL (dynamic)",
			code: `
def search(cursor, table):
    cursor.execute(f"SELECT * FROM {table}")
`,
			wantFragments: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fragments := extractor.Extract(tt.code, "python", "test.py")

			if len(fragments) != tt.wantFragments {
				t.Errorf("Extract() got %d fragments, want %d", len(fragments), tt.wantFragments)
			}
		})
	}
}

func TestSQLExtractor_ExtractSQL(t *testing.T) {
	extractor := NewSQLExtractor()

	code := `
CREATE TABLE users (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL
);

INSERT INTO users (name) VALUES ('test');

SELECT * FROM users;

UPDATE users SET name = 'updated' WHERE id = 1;

DELETE FROM users WHERE id = 2;
`

	fragments := extractor.Extract(code, "sql", "test.sql")

	if len(fragments) < 3 {
		t.Errorf("Extract() got %d fragments, want at least 3", len(fragments))
	}

	// Check for different query types
	queryTypes := make(map[SQLType]bool)
	for _, f := range fragments {
		queryTypes[f.QueryType] = true
	}

	expectedTypes := []SQLType{SQLTypeCreate, SQLTypeInsert, SQLTypeSelect}
	for _, qt := range expectedTypes {
		if !queryTypes[qt] {
			t.Errorf("Extract() missing query type %s", qt)
		}
	}
}

func TestSQLExtractor_ExtractFromDiff(t *testing.T) {
	extractor := NewSQLExtractor()

	diff := `
@@ -10,6 +10,10 @@ func getUser(db *sql.DB, id int) (*User, error) {
+	rows, err := db.Query(` + "`SELECT * FROM users WHERE id = $1`" + `, id)
+	if err != nil {
+		return nil, err
+	}
`

	fragments := extractor.ExtractFromDiff(diff, "go", "user.go")

	if len(fragments) == 0 {
		t.Error("ExtractFromDiff() returned no fragments")
	}
}

func TestClassifySQL(t *testing.T) {
	tests := []struct {
		query string
		want  SQLType
	}{
		{"SELECT * FROM users", SQLTypeSelect},
		{"WITH cte AS (SELECT 1) SELECT * FROM cte", SQLTypeSelect},
		{"INSERT INTO users VALUES (1)", SQLTypeInsert},
		{"UPDATE users SET name = 'x'", SQLTypeUpdate},
		{"DELETE FROM users", SQLTypeDelete},
		{"CREATE TABLE users (id INT)", SQLTypeCreate},
		{"ALTER TABLE users ADD COLUMN x INT", SQLTypeAlter},
		{"DROP TABLE users", SQLTypeDrop},
		{"GRANT SELECT ON users TO role", SQLTypeGrant},
		{"REVOKE SELECT ON users FROM role", SQLTypeRevoke},
		{"TRUNCATE users", SQLTypeTruncate},
		{"VACUUM ANALYZE users", SQLTypeOther},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := classifySQL(tt.query)
			if got != tt.want {
				t.Errorf("classifySQL(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestExtractTables(t *testing.T) {
	tests := []struct {
		query      string
		wantTables []string
	}{
		{
			query:      "SELECT * FROM users",
			wantTables: []string{"users"},
		},
		{
			query:      "SELECT * FROM users u JOIN orders o ON u.id = o.user_id",
			wantTables: []string{"users", "orders"},
		},
		{
			query:      "INSERT INTO products (name) VALUES ('test')",
			wantTables: []string{"products"},
		},
		{
			query:      "UPDATE customers SET name = 'x' WHERE id = 1",
			wantTables: []string{"customers"},
		},
		{
			query:      "SELECT * FROM a, b, c WHERE a.id = b.id",
			wantTables: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := extractTables(tt.query)

			if len(got) != len(tt.wantTables) {
				t.Errorf("extractTables() got %v, want %v", got, tt.wantTables)
				return
			}

			for _, want := range tt.wantTables {
				found := false
				for _, g := range got {
					if g == want {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("extractTables() missing table %s, got %v", want, got)
				}
			}
		})
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"main.go", "go"},
		{"script.py", "python"},
		{"App.java", "java"},
		{"index.js", "javascript"},
		{"component.tsx", "typescript"},
		{"app.rb", "ruby"},
		{"index.php", "php"},
		{"schema.sql", "sql"},
		{"unknown.xyz", "xyz"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := detectLanguage(tt.path)
			if got != tt.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestIsMigrationFilePath(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"db/migrations/001_create_users.sql", true},
		{"migrations/add_column.sql", true},
		{"db/schema.sql", true},
		{"flyway/V1__init.sql", true},
		{"src/models/user.go", false},
		{"test/query_test.go", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsMigrationFile(tt.path)
			if got != tt.want {
				t.Errorf("IsMigrationFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestContainsDynamicIndicators(t *testing.T) {
	tests := []struct {
		sql  string
		want bool
	}{
		{"SELECT * FROM users WHERE id = $1", false},
		{"SELECT * FROM " + "users", false},
		{"SELECT * FROM ' + tableName + '", true},
		{"CONCAT('SELECT * FROM ', table)", true},
		{"SELECT * FROM ${table}", true},
	}

	for _, tt := range tests {
		t.Run(tt.sql, func(t *testing.T) {
			got := containsDynamicIndicators(tt.sql)
			if got != tt.want {
				t.Errorf("containsDynamicIndicators(%q) = %v, want %v", tt.sql, got, tt.want)
			}
		})
	}
}

// Helper function
func containsSQL(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || containsSubstr(haystack, needle))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
