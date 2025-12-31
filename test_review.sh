#!/bin/bash
# test_review.sh - Interactive testing script for code review system

set -e

# Configuration
LLM_URL="${LLM_URL:-http://localhost:8000}"
LLM_MODEL="${LLM_MODEL:-deepseek-coder}"
REVIEWER="./code-reviewer"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Code Review System Test Suite ===${NC}"
echo "LLM Server: $LLM_URL"
echo "Model: $LLM_MODEL"
echo ""

# Build if needed
if [ ! -f "$REVIEWER" ]; then
    echo "Building code-reviewer..."
    go build -o code-reviewer ./cmd/reviewer/
fi

# Test function
run_test() {
    local name="$1"
    local file="$2"
    local diff="$3"

    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}TEST: $name${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""

    echo "$diff" | $REVIEWER -llm-url "$LLM_URL" -model "$LLM_MODEL" -file "$file"

    echo ""
}

# Test 1: SQL Injection
run_test "SQL Injection Detection" "database.go" 'diff --git a/database.go b/database.go
+func FindUser(db *sql.DB, username string) (*User, error) {
+    query := fmt.Sprintf("SELECT * FROM users WHERE name = '\''%s'\''", username)
+    row := db.QueryRow(query)
+    var user User
+    err := row.Scan(&user.ID, &user.Name, &user.Email)
+    return &user, err
+}'

# Test 2: Hardcoded Secrets
run_test "Hardcoded Secrets" "auth.go" 'diff --git a/auth.go b/auth.go
+package auth
+
+const (
+    JWTSecret     = "super-secret-key-12345"
+    AdminPassword = "admin@123"
+    APIToken      = "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
+)'

# Test 3: Race Condition
run_test "Race Condition" "cache.go" 'diff --git a/cache.go b/cache.go
+package cache
+
+var store = make(map[string]interface{})
+
+func Set(key string, value interface{}) {
+    store[key] = value
+}
+
+func Get(key string) interface{} {
+    return store[key]
+}'

# Test 4: Resource Leak
run_test "Resource Leak" "file.go" 'diff --git a/file.go b/file.go
+func ReadConfig(path string) ([]byte, error) {
+    file, err := os.Open(path)
+    if err != nil {
+        return nil, err
+    }
+    // Missing file.Close()
+    return io.ReadAll(file)
+}'

# Test 5: XSS Vulnerability
run_test "XSS Vulnerability" "handler.go" 'diff --git a/handler.go b/handler.go
+func SearchHandler(w http.ResponseWriter, r *http.Request) {
+    query := r.URL.Query().Get("q")
+    // Directly embedding user input in HTML
+    fmt.Fprintf(w, "<html><body><h1>Results for: %s</h1></body></html>", query)
+}'

# Test 6: Good Code (should pass)
run_test "Good Code (Should Approve)" "service.go" 'diff --git a/service.go b/service.go
+func (s *Service) GetUser(ctx context.Context, id int64) (*User, error) {
+    const query = "SELECT id, name, email FROM users WHERE id = $1"
+
+    row := s.db.QueryRowContext(ctx, query, id)
+
+    var user User
+    if err := row.Scan(&user.ID, &user.Name, &user.Email); err != nil {
+        if errors.Is(err, sql.ErrNoRows) {
+            return nil, ErrUserNotFound
+        }
+        return nil, fmt.Errorf("scan user: %w", err)
+    }
+
+    return &user, nil
+}'

echo -e "${GREEN}=== All Tests Complete ===${NC}"
