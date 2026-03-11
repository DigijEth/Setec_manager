package server

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const claimsKey contextKey = "claims"
const permissionsKey contextKey = "permissions"

// authRequired validates JWT from cookie or Authorization header.
// After validation it loads the user's permission names from the database
// and stores them in the request context for downstream handlers.
func (s *Server) authRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenStr := ""

		// Try cookie first
		if cookie, err := r.Cookie("setec_token"); err == nil {
			tokenStr = cookie.Value
		}

		// Fall back to Authorization header
		if tokenStr == "" {
			auth := r.Header.Get("Authorization")
			if strings.HasPrefix(auth, "Bearer ") {
				tokenStr = strings.TrimPrefix(auth, "Bearer ")
			}
		}

		if tokenStr == "" {
			// If HTML request, redirect to login
			if acceptsHTML(r) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
			return s.JWTKey, nil
		})
		if err != nil || !token.Valid {
			if acceptsHTML(r) {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), claimsKey, claims)

		// Load permissions from DB and cache in context
		perms, err := s.DB.GetUserPermissions(claims.UserID)
		if err == nil {
			permSet := make(map[string]bool, len(perms))
			for _, p := range perms {
				permSet[p.Name] = true
			}
			ctx = context.WithValue(ctx, permissionsKey, permSet)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// adminRequired checks that the authenticated user has admin role.
func (s *Server) adminRequired(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := getClaimsFromContext(r.Context())
		if claims == nil || claims.Role != "admin" {
			// Also accept users in the "admin" group
			if claims != nil && hasGroup(claims.Groups, "admin") {
				next.ServeHTTP(w, r)
				return
			}
			http.Error(w, "Admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(r.Context()))
	})
}

// permissionRequired returns middleware that checks if the authenticated user
// has the specified permission. Returns 403 Forbidden if not.
func (s *Server) permissionRequired(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !hasPermissionFromContext(r.Context(), perm) {
				// Admins (by legacy role field) always pass
				claims := getClaimsFromContext(r.Context())
				if claims != nil && (claims.Role == "admin" || hasGroup(claims.Groups, "admin")) {
					next.ServeHTTP(w, r)
					return
				}
				http.Error(w, "Forbidden: missing permission "+perm, http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func getClaimsFromContext(ctx context.Context) *Claims {
	claims, _ := ctx.Value(claimsKey).(*Claims)
	return claims
}

// getPermissionsFromContext returns the permission set stored in the context
// by authRequired. Returns nil if not present.
func getPermissionsFromContext(ctx context.Context) map[string]bool {
	perms, _ := ctx.Value(permissionsKey).(map[string]bool)
	return perms
}

// hasPermissionFromContext checks whether the context carries the named permission.
func hasPermissionFromContext(ctx context.Context, perm string) bool {
	perms := getPermissionsFromContext(ctx)
	if perms == nil {
		return false
	}
	return perms[perm]
}

// hasGroup checks if a group name appears in the slice.
func hasGroup(groups []string, name string) bool {
	for _, g := range groups {
		if g == name {
			return true
		}
	}
	return false
}

func acceptsHTML(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// ── Rate Limiter ────────────────────────────────────────────────────

type rateLimiter struct {
	mu       sync.Mutex
	attempts map[string][]time.Time
	limit    int
	window   time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		attempts: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

func (rl *rateLimiter) Allow(key string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove expired entries
	var valid []time.Time
	for _, t := range rl.attempts[key] {
		if t.After(cutoff) {
			valid = append(valid, t)
		}
	}

	if len(valid) >= rl.limit {
		rl.attempts[key] = valid
		return false
	}

	rl.attempts[key] = append(valid, now)
	return true
}

func (s *Server) loginRateLimit(next http.Handler) http.Handler {
	limiter := newRateLimiter(5, time.Minute)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if !limiter.Allow(ip) {
			http.Error(w, "Too many login attempts. Try again in a minute.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
