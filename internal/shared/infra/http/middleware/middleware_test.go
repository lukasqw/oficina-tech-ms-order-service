package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.opentelemetry.io/otel"
	authDomain "oficina-tech/internal/modules/access_control/domain/auth"
	"oficina-tech/internal/shared/infra/observability"
)

func TestMain(m *testing.M) {
	_ = observability.InitMetrics(otel.GetMeterProvider().Meter("test"))
	os.Exit(m.Run())
}

// mockJWTService implements auth.JWTService for tests.
type mockJWTService struct {
	claims *authDomain.TokenClaims
	err    error
}

func (m *mockJWTService) GenerateToken(claims authDomain.TokenClaims) (string, error) {
	return "mocked-token", nil
}

func (m *mockJWTService) ValidateToken(_ string) (*authDomain.TokenClaims, error) {
	return m.claims, m.err
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// --- Authenticate ---

func TestAuthenticate_MissingHeader(t *testing.T) {
	m := NewAuthMiddleware(&mockJWTService{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	m.Authenticate(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestAuthenticate_InvalidFormat_NoBearer(t *testing.T) {
	m := NewAuthMiddleware(&mockJWTService{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	rec := httptest.NewRecorder()
	m.Authenticate(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestAuthenticate_OnlyBearerNoToken(t *testing.T) {
	m := NewAuthMiddleware(&mockJWTService{})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	rec := httptest.NewRecorder()
	m.Authenticate(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestAuthenticate_InvalidToken(t *testing.T) {
	svc := &mockJWTService{err: errors.New("invalid token")}
	m := NewAuthMiddleware(svc)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer badtoken")
	rec := httptest.NewRecorder()
	m.Authenticate(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

func TestAuthenticate_ValidToken_PropagatesContext(t *testing.T) {
	claims := &authDomain.TokenClaims{UserID: "u1", Email: "u@example.com", Role: "ADMIN"}
	m := NewAuthMiddleware(&mockJWTService{claims: claims})

	var capturedID interface{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = r.Context().Value(UserIDKey)
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer validtoken")
	rec := httptest.NewRecorder()
	m.Authenticate(handler).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if capturedID != "u1" {
		t.Errorf("want userID 'u1' in context, got %v", capturedID)
	}
}

// --- RequireRole ---

func TestRequireRole_NoRoleInContext(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	m.RequireRole("ADMIN")(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRequireRole_InsufficientRole(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserRoleKey, RoleCustomer)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRequireRole_SufficientRole(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserRoleKey, RoleAdmin)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
}

func TestRequireRole_HierarchyInheritance(t *testing.T) {
	m := NewRBACMiddleware()
	// ADMIN satisfies MANAGER requirement via hierarchy
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserRoleKey, RoleAdmin)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireRole(RoleManager)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (ADMIN satisfies MANAGER), got %d", rec.Code)
	}
}

func TestRequireRole_CustomerCannotAccessUserEndpoint(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), UserRoleKey, RoleCustomer)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireRole(RoleUser)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403 (CUSTOMER cannot access USER endpoints), got %d", rec.Code)
	}
}

// --- RequireOwnerOrRole ---

func TestRequireOwnerOrRole_NoUserInContext(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/resource/123", nil)
	rec := httptest.NewRecorder()
	m.RequireOwnerOrRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRequireOwnerOrRole_AdminBypassesOwnership(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/resource/other-user-id", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, "my-user-id")
	ctx = context.WithValue(ctx, UserRoleKey, RoleAdmin)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireOwnerOrRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (ADMIN bypasses ownership), got %d", rec.Code)
	}
}

func TestRequireOwnerOrRole_InsufficientRoleAndNotOwner(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/resource/other-user", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, "my-user-id")
	ctx = context.WithValue(ctx, UserRoleKey, RoleCustomer)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireOwnerOrRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRequireOwnerOrRole_NoRoleInContext(t *testing.T) {
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/resource/123", nil)
	ctx := context.WithValue(req.Context(), UserIDKey, "some-user-id")
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireOwnerOrRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("want 403, got %d", rec.Code)
	}
}

func TestRequireOwnerOrRole_OwnerMatchesUUIDInPath(t *testing.T) {
	ownerID := "11111111-1111-4111-8111-111111111111"
	m := NewRBACMiddleware()
	req := httptest.NewRequest(http.MethodGet, "/resource/"+ownerID, nil)
	ctx := context.WithValue(req.Context(), UserIDKey, ownerID)
	ctx = context.WithValue(ctx, UserRoleKey, RoleCustomer)
	req = req.WithContext(ctx)
	rec := httptest.NewRecorder()
	m.RequireOwnerOrRole(RoleAdmin)(http.HandlerFunc(okHandler)).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("want 200 (owner match), got %d", rec.Code)
	}
}

func TestHasRequiredRole_UnknownAllowedRole(t *testing.T) {
	if hasRequiredRole(RoleAdmin, []string{"UNKNOWN_ROLE"}) {
		t.Error("expected false when allowedRole is not in roleHierarchy")
	}
}

func TestExtractResourceID_NoUUID(t *testing.T) {
	id := extractResourceID("/users/not-a-uuid")
	if id != "" {
		t.Errorf("expected empty string for non-UUID path segment, got %q", id)
	}
}

// --- WrapMux / NewObservabilityMiddleware ---

func TestWrapMux_MatchedRoute_Returns200(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ping", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := NewObservabilityMiddleware(WrapMux(mux))

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestWrapMux_UnmatchedRoute_NoMetricPanic(t *testing.T) {
	mux := http.NewServeMux()
	handler := NewObservabilityMiddleware(WrapMux(mux))

	req := httptest.NewRequest(http.MethodGet, "/no-such-route", nil)
	rr := httptest.NewRecorder()
	// Should not panic even with no matching route
	handler.ServeHTTP(rr, req)
}

func TestNewObservabilityMiddleware_500Route(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /error", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	handler := NewObservabilityMiddleware(WrapMux(mux))
	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestNewObservabilityMiddleware_400Route(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /bad", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	})

	handler := NewObservabilityMiddleware(WrapMux(mux))
	req := httptest.NewRequest(http.MethodGet, "/bad", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}
