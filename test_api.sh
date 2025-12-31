#!/bin/bash
# test_api.sh - Test the HTTP API with curl

SERVER="${SERVER:-http://localhost:8080}"

echo "Testing Code Review API at $SERVER"
echo ""

# Test 1: Health check
echo "=== Health Check ==="
curl -s "$SERVER/health" | jq .
echo ""

# Test 2: SQL Injection
echo "=== Test: SQL Injection ==="
curl -s -X POST "$SERVER/api/review" \
  -H "Content-Type: application/json" \
  -d '{
    "file_path": "user.go",
    "diff": "diff --git a/user.go b/user.go\n+func GetUser(db *sql.DB, id string) (*User, error) {\n+    query := \"SELECT * FROM users WHERE id = \" + id\n+    return db.QueryRow(query).Scan(&user)\n+}"
  }' | jq .
echo ""

# Test 3: Hardcoded Secret
echo "=== Test: Hardcoded Secret ==="
curl -s -X POST "$SERVER/api/review" \
  -H "Content-Type: application/json" \
  -d '{
    "file_path": "config.go",
    "diff": "diff --git a/config.go b/config.go\n+const APIKey = \"sk-1234567890abcdef1234567890abcdef\""
  }' | jq .
echo ""

# Test 4: Batch Review
echo "=== Test: Batch Review ==="
curl -s -X POST "$SERVER/api/review/batch" \
  -H "Content-Type: application/json" \
  -d '{
    "files": [
      {
        "file_path": "auth.go",
        "diff": "diff --git a/auth.go b/auth.go\n+func Login(w http.ResponseWriter, r *http.Request) {\n+    password := r.FormValue(\"password\")\n+    if password == \"admin123\" {\n+        // hardcoded password\n+    }\n+}"
      },
      {
        "file_path": "db.go",
        "diff": "diff --git a/db.go b/db.go\n+func Query(db *sql.DB, table, id string) {\n+    db.Query(\"SELECT * FROM \" + table + \" WHERE id = \" + id)\n+}"
      }
    ]
  }' | jq .
echo ""

echo "=== Done ==="
