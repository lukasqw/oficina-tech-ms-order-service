package utils

const (
	// Validation errors (4xx)
	ErrCodeInvalidRequest     = "INVALID_REQUEST"
	ErrCodeValidationFailed   = "VALIDATION_FAILED"
	ErrCodeInvalidUUID        = "INVALID_UUID"
	ErrCodeInvalidCredentials = "INVALID_CREDENTIALS"

	// Resource errors (4xx)
	ErrCodeNotFound          = "NOT_FOUND"
	ErrCodeDuplicateResource = "DUPLICATE_RESOURCE"
	ErrCodeUnauthorized      = "UNAUTHORIZED"
	ErrCodeForbidden         = "FORBIDDEN"

	// Server errors (5xx)
	ErrCodeInternalError = "INTERNAL_ERROR"
	ErrCodeDatabaseError = "DATABASE_ERROR"
)
