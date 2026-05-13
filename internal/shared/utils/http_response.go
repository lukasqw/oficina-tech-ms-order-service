package utils

import (
	"encoding/json"
	"net/http"
)

// RespondSuccess envia resposta de sucesso com envelope
func RespondSuccess(w http.ResponseWriter, status int, data interface{}) {
	envelope := Envelope{
		Data:   data,
		Errors: []ErrorDetail{},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

// RespondError envia resposta de erro com envelope
func RespondErrorEnvelope(w http.ResponseWriter, status int, code string, message string) {
	envelope := Envelope{
		Data: nil,
		Errors: []ErrorDetail{
			{
				Code:    code,
				Message: message,
				Field:   nil,
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

// RespondValidationError envia erro de validação com campo específico
func RespondValidationError(w http.ResponseWriter, field string, message string) {
	envelope := Envelope{
		Data: nil,
		Errors: []ErrorDetail{
			{
				Code:    ErrCodeValidationFailed,
				Message: message,
				Field:   StringPtr(field),
			},
		},
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(envelope)
}

// RespondMultipleErrors envia múltiplos erros
func RespondMultipleErrors(w http.ResponseWriter, status int, errors []ErrorDetail) {
	envelope := Envelope{
		Data:   nil,
		Errors: errors,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(envelope)
}

// StringPtr retorna um ponteiro para string (helper para campos opcionais)
func StringPtr(s string) *string {
	return &s
}
