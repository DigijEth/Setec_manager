package server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// ── Security Headers Middleware ──────────────────────────────────────

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self' 'unsafe-inline'; "+
				"style-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"font-src 'self'; "+
				"connect-src 'self'; "+
				"frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

// ── Request Body Limit ──────────────────────────────────────────────

func maxBodySize(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}

// ── CSRF Protection ─────────────────────────────────────────────────

const csrfTokenLength = 32
const csrfCookieName = "setec_csrf"
const csrfHeaderName = "X-CSRF-Token"
const csrfFormField = "csrf_token"

func generateCSRFToken() string {
	b := make([]byte, csrfTokenLength)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func csrfProtection(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe methods don't need CSRF validation
		if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
			// Ensure a CSRF cookie exists for forms to use
			if _, err := r.Cookie(csrfCookieName); err != nil {
				token := generateCSRFToken()
				http.SetCookie(w, &http.Cookie{
					Name:     csrfCookieName,
					Value:    token,
					Path:     "/",
					HttpOnly: false, // JS needs to read this
					Secure:   true,
					SameSite: http.SameSiteStrictMode,
					MaxAge:   86400,
				})
			}
			next.ServeHTTP(w, r)
			return
		}

		// For mutating requests, validate CSRF token
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil {
			http.Error(w, "CSRF token missing", http.StatusForbidden)
			return
		}

		// Check header first, then form field
		token := r.Header.Get(csrfHeaderName)
		if token == "" {
			token = r.FormValue(csrfFormField)
		}

		// API requests with JSON Content-Type + Bearer auth skip CSRF
		// (they're not vulnerable to CSRF since browsers don't send custom headers)
		contentType := r.Header.Get("Content-Type")
		authHeader := r.Header.Get("Authorization")
		if strings.Contains(contentType, "application/json") && strings.HasPrefix(authHeader, "Bearer ") {
			next.ServeHTTP(w, r)
			return
		}

		if token != cookie.Value {
			http.Error(w, "CSRF token invalid", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// ── Password Policy ─────────────────────────────────────────────────

type passwordPolicy struct {
	MinLength    int
	RequireUpper bool
	RequireLower bool
	RequireDigit bool
}

var defaultPasswordPolicy = passwordPolicy{
	MinLength:    8,
	RequireUpper: true,
	RequireLower: true,
	RequireDigit: true,
}

func validatePassword(password string) error {
	p := defaultPasswordPolicy

	if len(password) < p.MinLength {
		return fmt.Errorf("password must be at least %d characters", p.MinLength)
	}

	hasUpper, hasLower, hasDigit := false, false, false
	for _, c := range password {
		if unicode.IsUpper(c) {
			hasUpper = true
		}
		if unicode.IsLower(c) {
			hasLower = true
		}
		if unicode.IsDigit(c) {
			hasDigit = true
		}
	}

	if p.RequireUpper && !hasUpper {
		return fmt.Errorf("password must contain at least one uppercase letter")
	}
	if p.RequireLower && !hasLower {
		return fmt.Errorf("password must contain at least one lowercase letter")
	}
	if p.RequireDigit && !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	return nil
}

// ── Persistent JWT Key ──────────────────────────────────────────────

func LoadOrCreateJWTKey(dataDir string) ([]byte, error) {
	keyPath := filepath.Join(dataDir, ".jwt_key")

	// Try to load existing key
	data, err := os.ReadFile(keyPath)
	if err == nil && len(data) == 32 {
		return data, nil
	}

	// Generate new key
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}

	// Save with restrictive permissions
	os.MkdirAll(dataDir, 0700)
	if err := os.WriteFile(keyPath, key, 0600); err != nil {
		return nil, err
	}

	return key, nil
}

// ── Audit Logger ────────────────────────────────────────────────────

func (s *Server) logAudit(r *http.Request, action, detail string) {
	claims := getClaimsFromContext(r.Context())
	username := "anonymous"
	if claims != nil {
		username = claims.Username
	}
	ip := r.RemoteAddr

	// Insert into audit log table
	s.DB.Conn().Exec(`INSERT INTO audit_log (username, ip, action, detail) VALUES (?, ?, ?, ?)`,
		username, ip, action, detail)
}
