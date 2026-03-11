package server

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	UserID   int64    `json:"user_id"`
	Username string   `json:"username"`
	Role     string   `json:"role"`
	Groups   []string `json:"groups,omitempty"`
	jwt.RegisteredClaims
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token    string   `json:"token"`
	Username string   `json:"username"`
	Role     string   `json:"role"`
	Groups   []string `json:"groups,omitempty"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("[auth] Failed to decode login request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	log.Printf("[auth] Login attempt for user: %q", req.Username)

	user, err := s.DB.AuthenticateUser(req.Username, req.Password)
	if err != nil {
		log.Printf("[auth] AuthenticateUser error: %v", err)
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	if user == nil {
		log.Printf("[auth] Authentication failed for user: %q", req.Username)
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}
	log.Printf("[auth] Login successful for user: %q (id=%d)", user.Username, user.ID)

	// Load user's group names for JWT
	groups, err := s.DB.GetUserGroupNames(user.ID)
	if err != nil {
		log.Printf("[auth] Warning: failed to load groups for user %s: %v", user.Username, err)
		groups = nil
	}

	// Generate JWT
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		Groups:   groups,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenStr, err := token.SignedString(s.JWTKey)
	if err != nil {
		http.Error(w, "Token generation failed", http.StatusInternalServerError)
		return
	}

	// Set cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "setec_token",
		Value:    tokenStr,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.Config.Server.TLS,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   86400,
	})

	writeJSON(w, http.StatusOK, loginResponse{
		Token:    tokenStr,
		Username: user.Username,
		Role:     user.Role,
		Groups:   groups,
	})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "setec_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})
	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	claims := getClaimsFromContext(r.Context())
	if claims == nil {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":  claims.UserID,
		"username": claims.Username,
		"role":     claims.Role,
		"groups":   claims.Groups,
	})
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	s.renderTemplate(w, "login.html", nil)
}

// Temporary debug endpoint — remove after login is working
func (s *Server) handleDebugAuth(w http.ResponseWriter, r *http.Request) {
	count, err := s.DB.ManagerUserCount()
	if err != nil {
		writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
		return
	}
	users, err := s.DB.ListManagerUsers()
	if err != nil {
		writeJSON(w, 500, map[string]interface{}{"error": err.Error()})
		return
	}
	var userList []map[string]interface{}
	for _, u := range users {
		userList = append(userList, map[string]interface{}{
			"id":            u.ID,
			"username":      u.Username,
			"role":          u.Role,
			"hash_len":      len(u.PasswordHash),
			"hash_prefix":   u.PasswordHash[:10],
			"force_change":  u.ForceChange,
		})
	}
	writeJSON(w, 200, map[string]interface{}{
		"user_count": count,
		"users":      userList,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
