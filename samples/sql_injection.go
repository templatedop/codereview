package samples

import (
	"database/sql"
	"fmt"
	"net/http"
)

// UserRepository handles user database operations
type UserRepository struct {
	db *sql.DB
}

// GetUserByID retrieves a user by their ID - VULNERABLE TO SQL INJECTION
func (r *UserRepository) GetUserByID(userID string) (*User, error) {
	// BAD: String concatenation in SQL query
	query := "SELECT id, name, email, password FROM users WHERE id = " + userID
	row := r.db.QueryRow(query)

	var user User
	err := row.Scan(&user.ID, &user.Name, &user.Email, &user.Password)
	return &user, err
}

// SearchUsers searches users by name - VULNERABLE TO SQL INJECTION
func (r *UserRepository) SearchUsers(name string) ([]User, error) {
	// BAD: Using fmt.Sprintf for SQL query
	query := fmt.Sprintf("SELECT * FROM users WHERE name LIKE '%%%s%%'", name)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}

	var users []User
	for rows.Next() {
		var u User
		rows.Scan(&u.ID, &u.Name, &u.Email)
		users = append(users, u)
	}
	return users, nil
}

// DeleteUser deletes a user - VULNERABLE TO SQL INJECTION
func (r *UserRepository) DeleteUser(w http.ResponseWriter, req *http.Request) {
	userID := req.URL.Query().Get("id")
	// BAD: Direct user input in query
	_, err := r.db.Exec("DELETE FROM users WHERE id = " + userID)
	if err != nil {
		http.Error(w, "Failed", 500)
		return
	}
	w.Write([]byte("Deleted"))
}

type User struct {
	ID       int
	Name     string
	Email    string
	Password string
}
