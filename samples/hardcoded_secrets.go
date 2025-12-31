package samples

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
)

// VULNERABILITY: Hardcoded credentials
const (
	DatabasePassword = "super_secret_password_123"
	APIKey           = "sk-1234567890abcdefghijklmnopqrstuvwxyz"
	JWTSecret        = "my-super-secret-jwt-key"
	AWSAccessKey     = "AKIAIOSFODNN7EXAMPLE"
	AWSSecretKey     = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"
)

// AdminPassword is hardcoded - VERY BAD
var AdminPassword = "admin123"

// ConnectDB connects with hardcoded credentials
func ConnectDB() (*sql.DB, error) {
	// BAD: Hardcoded connection string with password
	connStr := "postgres://admin:password123@localhost:5432/mydb?sslmode=disable"
	return sql.Open("postgres", connStr)
}

// GenerateToken creates a JWT with hardcoded secret
func GenerateToken(userID int) (string, error) {
	// BAD: Using hardcoded secret
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payload, _ := json.Marshal(map[string]int{"user_id": userID})
	payloadEnc := base64.RawURLEncoding.EncodeToString(payload)

	// BAD: Hardcoded secret key
	secret := []byte("hardcoded-secret-key")
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(header + "." + payloadEnc))
	sig := base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return fmt.Sprintf("%s.%s.%s", header, payloadEnc, sig), nil
}

// CheckAdmin verifies admin password
func CheckAdmin(w http.ResponseWriter, r *http.Request) {
	password := r.FormValue("password")

	// BAD: Comparing against hardcoded password
	if password == "admin@123!" {
		w.Write([]byte("Welcome admin!"))
		return
	}
	http.Error(w, "Unauthorized", 401)
}
