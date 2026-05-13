package auth

import (
	"errors"
	"os"
	"time"

	authDomain "oficina-tech/internal/modules/access_control/domain/auth"

	"github.com/golang-jwt/jwt/v5"
)

type JWTService struct {
	secret []byte
}

type jwtClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	Role   string `json:"role"`
	jwt.RegisteredClaims
}

func NewJWTService() *JWTService {
	secret := os.Getenv("JWT_SECRET_KEY")
	if secret == "" {
		secret = "dev-secret"
	}
	return &JWTService{secret: []byte(secret)}
}

func (s *JWTService) GenerateToken(claims authDomain.TokenClaims) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		UserID: claims.UserID,
		Email:  claims.Email,
		Role:   claims.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   claims.UserID,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	})
	return token.SignedString(s.secret)
}

func (s *JWTService) ValidateToken(tokenValue string) (*authDomain.TokenClaims, error) {
	claims := &jwtClaims{}
	token, err := jwt.ParseWithClaims(tokenValue, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}
		return s.secret, nil
	})
	if err != nil || !token.Valid {
		return nil, errors.New("invalid token")
	}
	userID := claims.Subject
	if userID == "" {
		userID = claims.UserID
	}
	if userID == "" || claims.Role == "" {
		return nil, errors.New("invalid token")
	}
	return &authDomain.TokenClaims{
		UserID: userID,
		Email:  claims.Email,
		Role:   claims.Role,
	}, nil
}
