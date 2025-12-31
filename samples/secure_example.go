package samples

import (
	"context"
	"database/sql"
	"errors"
	"html/template"
	"net/http"
	"os"
	"sync"
)

// SecureUserRepository demonstrates secure database operations
type SecureUserRepository struct {
	db *sql.DB
}

// GetUserByID uses parameterized queries - SECURE
func (r *SecureUserRepository) GetUserByID(ctx context.Context, userID int) (*User, error) {
	// GOOD: Using parameterized query with placeholder
	const query = "SELECT id, name, email FROM users WHERE id = $1"
	row := r.db.QueryRowContext(ctx, query, userID)

	var user User
	if err := row.Scan(&user.ID, &user.Name, &user.Email); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return &user, nil
}

// SearchUsers uses parameterized LIKE query - SECURE
func (r *SecureUserRepository) SearchUsers(ctx context.Context, name string) ([]User, error) {
	// GOOD: Using parameter for LIKE query
	const query = "SELECT id, name, email FROM users WHERE name LIKE $1"
	rows, err := r.db.QueryContext(ctx, query, "%"+name+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close() // GOOD: Always close rows

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// SecureCache demonstrates thread-safe map access
type SecureCache struct {
	mu    sync.RWMutex
	items map[string]string
}

// Set safely writes to the cache
func (c *SecureCache) Set(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[key] = value
}

// Get safely reads from the cache
func (c *SecureCache) Get(key string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[key]
	return v, ok
}

// ReadFileSafely demonstrates proper resource handling
func ReadFileSafely(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close() // GOOD: Always close file

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
}

// SecureSearchHandler demonstrates XSS prevention
func SecureSearchHandler(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")

	// GOOD: Using html/template which auto-escapes
	tmpl := template.Must(template.New("search").Parse(`
		<!DOCTYPE html>
		<html>
		<body>
			<h1>Search results for: {{.}}</h1>
		</body>
		</html>
	`))

	w.Header().Set("Content-Type", "text/html")
	tmpl.Execute(w, query) // Auto-escaped!
}

var ErrUserNotFound = errors.New("user not found")
