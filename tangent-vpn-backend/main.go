package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

//go:embed admin.html
var adminHTML string

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("cannot determine executable path: %v", err)
	}
	dbPath := filepath.Join(filepath.Dir(exe), "tangent_vpn.db")
	if err := InitDB(dbPath); err != nil {
		log.Fatalf("cannot open database: %v", err)
	}
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/", corsMiddleware(handler))

	addr := ":7878"
	log.Printf("TangentVPN backend listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

// corsMiddleware wraps every request with CORS headers and handles OPTIONS preflight.
func corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next(w, r)
	}
}

func handler(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Serve admin panel for browser requests to /, /admin, /index.html
	if path == "/" || path == "/admin" || path == "/index.html" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(adminHTML))
		return
	}

	switch {
	// ---- public endpoints ----
	case path == "/api/register" && r.Method == http.MethodPost:
		handleRegister(w, r)
	case path == "/api/login" && r.Method == http.MethodPost:
		handleLogin(w, r)
	case path == "/api/profile" && r.Method == http.MethodGet:
		handleProfile(w, r)
	case path == "/api/admin/verify" && r.Method == http.MethodPost:
		handleAdminVerify(w, r)

	// ---- admin endpoints ----
	case path == "/api/users" && r.Method == http.MethodGet:
		handleListUsers(w, r)
	case path == "/api/users" && r.Method == http.MethodPost:
		handleAdminCreateUser(w, r)
	case strings.HasPrefix(path, "/api/users/") && r.Method == http.MethodPut:
		handleAdminUpdateUser(w, r)
	case strings.HasPrefix(path, "/api/users/") && r.Method == http.MethodDelete:
		handleAdminDeleteUser(w, r)

	default:
		jsonError(w, "not found", http.StatusNotFound)
	}
}

// --------------- helpers ---------------

func jsonWrite(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, msg string, status int) {
	jsonWrite(w, status, map[string]string{"error": msg})
}

func bearerEmail(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return "", fmt.Errorf("missing or invalid Authorization header")
	}
	token := strings.TrimPrefix(auth, "Bearer ")
	return ValidateJWT(token)
}

// --------------- handlers ---------------

func handleRegister(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	now := time.Now()
	user := &User{
		Email:     req.Email,
		Password:  hash,
		CreatedAt: now,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		IsActive:  true,
	}
	if err := CreateUser(user); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonWrite(w, http.StatusCreated, map[string]string{"message": "registered"})
}

func handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := GetUser(req.Email)
	if err != nil {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if !CheckPassword(req.Password, user.Password) {
		jsonError(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	token, err := GenerateJWT(user.Email)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]string{
		"token":      token,
		"expires_at": user.ExpiresAt.Format(time.RFC3339),
		"email":      user.Email,
	})
}

func handleProfile(w http.ResponseWriter, r *http.Request) {
	email, err := bearerEmail(r)
	if err != nil {
		jsonError(w, err.Error(), http.StatusUnauthorized)
		return
	}
	user, err := GetUser(email)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]interface{}{
		"email":      user.Email,
		"expires_at": user.ExpiresAt.Format(time.RFC3339),
		"is_active":  user.IsActive,
		"created_at": user.CreatedAt.Format(time.RFC3339),
	})
}

func handleAdminVerify(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	valid := ValidateAdmin(req.Password)
	resp := map[string]interface{}{"valid": valid}
	if valid {
		token, err := GenerateJWT("__admin__")
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		resp["token"] = token
	}
	jsonWrite(w, http.StatusOK, resp)
}

func handleListUsers(w http.ResponseWriter, r *http.Request) {
	email, err := bearerEmail(r)
	if err != nil || email != "__admin__" {
		jsonError(w, "admin auth required", http.StatusUnauthorized)
		return
	}
	users, err := ListUsers()
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	// Strip password hashes from output.
	type safeUser struct {
		Email     string `json:"email"`
		CreatedAt string `json:"created_at"`
		ExpiresAt string `json:"expires_at"`
		IsActive  bool   `json:"is_active"`
	}
	out := make([]safeUser, 0, len(users))
	for _, u := range users {
		out = append(out, safeUser{
			Email:     u.Email,
			CreatedAt: u.CreatedAt.Format(time.RFC3339),
			ExpiresAt: u.ExpiresAt.Format(time.RFC3339),
			IsActive:  u.IsActive,
		})
	}
	jsonWrite(w, http.StatusOK, out)
}

func handleAdminCreateUser(w http.ResponseWriter, r *http.Request) {
	email, err := bearerEmail(r)
	if err != nil || email != "__admin__" {
		jsonError(w, "admin auth required", http.StatusUnauthorized)
		return
	}
	var req struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" {
		jsonError(w, "email and password are required", http.StatusBadRequest)
		return
	}

	hash, err := HashPassword(req.Password)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	var expires time.Time
	if req.ExpiresAt != "" {
		expires, err = time.Parse(time.RFC3339, req.ExpiresAt)
		if err != nil {
			// Try date-only format.
			expires, err = time.Parse("2006-01-02", req.ExpiresAt)
			if err != nil {
				jsonError(w, "invalid expires_at format, use RFC3339 or YYYY-MM-DD", http.StatusBadRequest)
				return
			}
		}
	} else {
		expires = time.Now().Add(30 * 24 * time.Hour)
	}

	user := &User{
		Email:     req.Email,
		Password:  hash,
		CreatedAt: time.Now(),
		ExpiresAt: expires,
		IsActive:  true,
	}
	if err := CreateUser(user); err != nil {
		jsonError(w, err.Error(), http.StatusConflict)
		return
	}
	jsonWrite(w, http.StatusCreated, map[string]string{"message": "user created"})
}

func handleAdminUpdateUser(w http.ResponseWriter, r *http.Request) {
	email, err := bearerEmail(r)
	if err != nil || email != "__admin__" {
		jsonError(w, "admin auth required", http.StatusUnauthorized)
		return
	}
	target := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if target == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}

	user, err := GetUser(target)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	var req struct {
		ExpiresAt *string `json:"expires_at"`
		IsActive  *bool   `json:"is_active"`
		Password  *string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			t, err = time.Parse("2006-01-02", *req.ExpiresAt)
			if err != nil {
				jsonError(w, "invalid expires_at format", http.StatusBadRequest)
				return
			}
		}
		user.ExpiresAt = t
	}
	if req.IsActive != nil {
		user.IsActive = *req.IsActive
	}
	if req.Password != nil && *req.Password != "" {
		hash, err := HashPassword(*req.Password)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		user.Password = hash
	}

	if err := UpdateUser(target, user); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]string{"message": "user updated"})
}

func handleAdminDeleteUser(w http.ResponseWriter, r *http.Request) {
	email, err := bearerEmail(r)
	if err != nil || email != "__admin__" {
		jsonError(w, "admin auth required", http.StatusUnauthorized)
		return
	}
	target := strings.TrimPrefix(r.URL.Path, "/api/users/")
	if target == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}
	if err := DeleteUser(target); err != nil {
		jsonError(w, err.Error(), http.StatusNotFound)
		return
	}
	jsonWrite(w, http.StatusOK, map[string]string{"message": "user deleted"})
}
