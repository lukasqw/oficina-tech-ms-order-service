package utils

// Envelope encapsula todas as respostas da API
// Representa o formato padrão de resposta para todos os endpoints
type Envelope struct {
	// Response data payload (null when errors are present)
	Data interface{} `json:"data,omitempty" swaggertype:"object"`
	// Array of error details (empty array on success)
	Errors []ErrorDetail `json:"errors,omitempty"`
}

// ErrorDetail representa um erro individual na resposta da API
// Fornece informações estruturadas sobre erros de validação, negócio ou sistema
type ErrorDetail struct {
	// Error code for programmatic error handling
	Code string `json:"code" example:"VALIDATION_FAILED" enums:"INVALID_REQUEST,VALIDATION_FAILED,INVALID_UUID,INVALID_CREDENTIALS,NOT_FOUND,DUPLICATE_RESOURCE,UNAUTHORIZED,FORBIDDEN,INTERNAL_ERROR,DATABASE_ERROR"`
	// Human-readable error message in Portuguese
	Message string `json:"message" example:"O campo email é obrigatório"`
	// Field name for validation errors (optional, only present for field-specific errors)
	Field *string `json:"field,omitempty" example:"email"`
}
