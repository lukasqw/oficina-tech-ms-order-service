package auth

type TokenClaims struct {
	UserID string
	Email  string
	Role   string
}

type JWTService interface {
	GenerateToken(claims TokenClaims) (string, error)
	ValidateToken(token string) (*TokenClaims, error)
}
