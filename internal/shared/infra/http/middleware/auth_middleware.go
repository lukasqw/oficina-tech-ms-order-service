package middleware

import (
	"context"
	"net/http"
	"oficina-tech/internal/modules/access_control/domain/auth"
	"oficina-tech/internal/shared/utils"
	"strings"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	UserIDKey        contextKey = "user_id"
	UserEmailKey     contextKey = "user_email"
	UserRoleKey      contextKey = "user_role"
	AuthorizationKey contextKey = "authorization"
)

// AuthMiddleware validates JWT tokens and extracts user information
type AuthMiddleware struct {
	jwtService auth.JWTService
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(jwtService auth.JWTService) *AuthMiddleware {
	return &AuthMiddleware{
		jwtService: jwtService,
	}
}

// Authenticate validates JWT token from Authorization header and adds user info to request context
// Expected header format: Authorization: Bearer <token>
func (m *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Missing Authorization header")
			return
		}

		// Check Bearer prefix
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Invalid Authorization header format")
			return
		}

		token := parts[1]
		if token == "" {
			utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Invalid Authorization header format")
			return
		}

		// Validate JWT token
		claims, err := m.jwtService.ValidateToken(token)
		if err != nil {
			utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, "Invalid or expired token")
			return
		}

		// Add user information to request context
		ctx := r.Context()
		ctx = context.WithValue(ctx, UserIDKey, claims.UserID)
		ctx = context.WithValue(ctx, UserRoleKey, claims.Role)
		ctx = context.WithValue(ctx, UserEmailKey, claims.Email)
		ctx = context.WithValue(ctx, AuthorizationKey, authHeader)

		// Continue with the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
