package middleware

import (
	"net/http"
	"oficina-tech/internal/shared/utils"
	"strings"
)

// Role constants
const (
	RoleCustomer = "CUSTOMER"
	RoleUser     = "USER"
	RoleManager  = "MANAGER"
	RoleAdmin    = "ADMIN"
)

// roleHierarchy defines the hierarchy levels for roles.
// CUSTOMER is level 0 so it never inherits access to internal (USER+) endpoints
// unless explicitly listed in RequireRole.
var roleHierarchy = map[string]int{
	RoleCustomer: 0,
	RoleUser:     1,
	RoleManager:  2,
	RoleAdmin:    3,
}

// RBACMiddleware handles role-based access control
type RBACMiddleware struct{}

// NewRBACMiddleware creates a new RBAC middleware
func NewRBACMiddleware() *RBACMiddleware {
	return &RBACMiddleware{}
}

// RequireRole creates middleware that checks if user has required role
// Implements role hierarchy: ADMIN > MANAGER > USER
func (m *RBACMiddleware) RequireRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user role from context
			userRole, ok := r.Context().Value(UserRoleKey).(string)
			if !ok || userRole == "" {
				utils.RespondErrorEnvelope(w, http.StatusForbidden, utils.ErrCodeForbidden, "User role not found in context")
				return
			}

			// Check if user has sufficient role
			if !hasRequiredRole(userRole, allowedRoles) {
				utils.RespondErrorEnvelope(w, http.StatusForbidden, utils.ErrCodeForbidden, "Insufficient permissions")
				return
			}

			// User has required role, continue
			next.ServeHTTP(w, r)
		})
	}
}

// RequireOwnerOrRole checks if user is owner of resource or has required role
func (m *RBACMiddleware) RequireOwnerOrRole(allowedRoles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract user ID from context
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok {
				utils.RespondErrorEnvelope(w, http.StatusForbidden, utils.ErrCodeForbidden, "User ID not found in context")
				return
			}

			// Extract user role from context
			userRole, ok := r.Context().Value(UserRoleKey).(string)
			if !ok || userRole == "" {
				utils.RespondErrorEnvelope(w, http.StatusForbidden, utils.ErrCodeForbidden, "User role not found in context")
				return
			}

			// Extract resource owner ID from URL path
			// Expected format: /users/{id} or similar
			resourceID := extractResourceID(r.URL.Path)

			// Check if user is the owner
			if resourceID != "" && userID == resourceID {
				// User is the owner, allow access
				next.ServeHTTP(w, r)
				return
			}

			// User is not the owner, check if they have required role
			if !hasRequiredRole(userRole, allowedRoles) {
				utils.RespondErrorEnvelope(w, http.StatusForbidden, utils.ErrCodeForbidden, "Insufficient permissions")
				return
			}

			// User has required role, continue
			next.ServeHTTP(w, r)
		})
	}
}

// hasRequiredRole checks if user's role meets the requirement based on hierarchy
func hasRequiredRole(userRole string, allowedRoles []string) bool {
	userLevel, exists := roleHierarchy[userRole]
	if !exists {
		return false
	}

	// Check if user's role level meets any of the allowed roles
	for _, allowedRole := range allowedRoles {
		allowedLevel, exists := roleHierarchy[allowedRole]
		if !exists {
			continue
		}

		// User has sufficient role if their level is >= allowed level
		if userLevel >= allowedLevel {
			return true
		}
	}

	return false
}

// extractResourceID extracts the resource ID from the URL path
// Handles paths like /users/{uuid}, /api/users/{uuid}, etc.
func extractResourceID(path string) string {
	// Split path by "/"
	parts := strings.Split(strings.Trim(path, "/"), "/")

	// Look for UUID in the path (typically the last segment)
	for i := len(parts) - 1; i >= 0; i-- {
		// Validar se é um UUID válido
		if err := utils.ValidateUUID(parts[i]); err == nil {
			return parts[i]
		}
	}

	return ""
}
