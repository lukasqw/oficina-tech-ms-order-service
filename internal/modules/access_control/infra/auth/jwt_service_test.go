package auth

import (
	"testing"
	"time"

	authDomain "oficina-tech/internal/modules/access_control/domain/auth"

	"github.com/golang-jwt/jwt/v5"
)

func TestNewJWTService_DefaultSecret(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "")
	svc := NewJWTService()
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestNewJWTService_EnvSecret(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "my-test-secret-32chars-padding!!")
	svc := NewJWTService()
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestGenerateAndValidateToken_RoundTrip(t *testing.T) {
	const secret = "test-secret-key-32chars-padding!!"
	t.Setenv("JWT_SECRET_KEY", secret)
	svc := NewJWTService()

	claims := authDomain.TokenClaims{
		UserID: "user-123",
		Email:  "test@example.com",
		Role:   "ADMIN",
	}

	token, err := svc.GenerateToken(claims)
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	result, err := svc.ValidateToken(token)
	if err != nil {
		t.Fatalf("ValidateToken() error: %v", err)
	}
	if result.UserID != claims.UserID {
		t.Errorf("UserID: want %s, got %s", claims.UserID, result.UserID)
	}
	if result.Email != claims.Email {
		t.Errorf("Email: want %s, got %s", claims.Email, result.Email)
	}
	if result.Role != claims.Role {
		t.Errorf("Role: want %s, got %s", claims.Role, result.Role)
	}
}

func TestValidateToken_InvalidString(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-32chars-padding!!")
	svc := NewJWTService()
	if _, err := svc.ValidateToken("not.a.valid.token"); err == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestValidateToken_EmptyString(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-32chars-padding!!")
	svc := NewJWTService()
	if _, err := svc.ValidateToken(""); err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestValidateToken_WrongSecret(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "correct-secret-key-32chars-pad!!")
	svc := NewJWTService()

	wrongSecret := []byte("wrong-secret-key-32chars-padding!")
	c := &jwtClaims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenStr, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(wrongSecret)

	if _, err := svc.ValidateToken(tokenStr); err == nil {
		t.Fatal("expected error for token signed with wrong secret")
	}
}

func TestValidateToken_ExpiredToken(t *testing.T) {
	const secret = "test-secret-key-32chars-padding!!"
	t.Setenv("JWT_SECRET_KEY", secret)
	svc := NewJWTService()

	c := &jwtClaims{
		UserID: "u1",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "u1",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
		},
	}
	tokenStr, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))

	if _, err := svc.ValidateToken(tokenStr); err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestValidateToken_EmptySubject(t *testing.T) {
	const secret = "test-secret-key-32chars-padding!!"
	t.Setenv("JWT_SECRET_KEY", secret)
	svc := NewJWTService()

	c := &jwtClaims{
		UserID: "",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	tokenStr, _ := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))

	if _, err := svc.ValidateToken(tokenStr); err == nil {
		t.Fatal("expected error: empty userID should be invalid")
	}
}

func TestGenerateToken_MultipleRoles(t *testing.T) {
	t.Setenv("JWT_SECRET_KEY", "test-secret-key-32chars-padding!!")
	svc := NewJWTService()

	for _, role := range []string{"CUSTOMER", "USER", "MANAGER", "ADMIN"} {
		claims := authDomain.TokenClaims{UserID: "u1", Email: "u@x.com", Role: role}
		token, err := svc.GenerateToken(claims)
		if err != nil || token == "" {
			t.Errorf("GenerateToken(%s) failed: %v", role, err)
		}
	}
}
