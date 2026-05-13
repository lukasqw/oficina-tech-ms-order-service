package utils

import (
	"net/http"
	"strings"
)

// ErrorMapping mapeia erro de domínio para código e status HTTP
type ErrorMapping struct {
	Code       string
	StatusCode int
}

// MapDomainError mapeia erros de domínio para ErrorMapping
// Usa comparação por mensagem de erro para evitar ciclos de importação
func MapDomainError(err error) ErrorMapping {
	if err == nil {
		return ErrorMapping{ErrCodeInternalError, http.StatusInternalServerError}
	}

	errMsg := err.Error()

	if errMsg == "resource not found" {
		return ErrorMapping{ErrCodeNotFound, http.StatusNotFound}
	}
	if errMsg == "unauthorized" {
		return ErrorMapping{ErrCodeUnauthorized, http.StatusUnauthorized}
	}
	if errMsg == "saga de estoque ainda não implementada" {
		return ErrorMapping{ErrCodeInternalError, http.StatusNotImplemented}
	}

	// Not Found errors (404)
	switch errMsg {
	case "customer not found", "vehicle not found", "service not found",
		"product not found", "user not found", "ordem de serviço não encontrada",
		"produto não encontrado", "serviço não encontrado", "item não encontrado na ordem",
		"inventário do produto não encontrado", "ID do cliente inválido", "ID do veículo inválido":
		return ErrorMapping{ErrCodeNotFound, http.StatusNotFound}
	}

	// Vehicle ownership validation error (400)
	if errMsg == "veículo não pertence ao cliente da ordem de serviço" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusBadRequest}
	}

	// No next status available or invalid transition (409)
	if errMsg == "não há próximo status disponível" || errMsg == "transição de status inválida" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusConflict}
	}

	// Duplicate/Conflict errors (409)
	if strings.Contains(errMsg, "already exists") {
		return ErrorMapping{ErrCodeDuplicateResource, http.StatusConflict}
	}

	// Cannot modify closed order (409)
	if errMsg == "não é possível modificar ordem fechada" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusConflict}
	}

	// Cannot modify items after pending authorization (409)
	if errMsg == "não é possível modificar itens após status PENDING_AUTHORIZATION" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusConflict}
	}

	// Inventory operation errors (400)
	if errMsg == "estoque reservado insuficiente" || errMsg == "operação de estoque falhou" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusBadRequest}
	}

	// Authentication errors (401)
	if errMsg == "invalid credentials" || errMsg == "credenciais inválidas" {
		return ErrorMapping{ErrCodeInvalidCredentials, http.StatusUnauthorized}
	}

	// Authorization errors (403)
	if errMsg == "acesso não autorizado a esta ordem de serviço" {
		return ErrorMapping{ErrCodeUnauthorized, http.StatusForbidden}
	}

	// Service order status errors (409)
	if errMsg == "ordem de serviço não está aguardando autorização" {
		return ErrorMapping{ErrCodeValidationFailed, http.StatusConflict}
	}

	// Validation errors (400)
	// Check for common validation error patterns
	validationKeywords := []string{
		"invalid", "must be", "required", "format", "too short",
		"too long", "must have", "does not match", "cannot delete",
	}

	for _, keyword := range validationKeywords {
		if strings.Contains(strings.ToLower(errMsg), keyword) {
			return ErrorMapping{ErrCodeValidationFailed, http.StatusBadRequest}
		}
	}

	// Default fallback for unknown errors
	return ErrorMapping{ErrCodeInternalError, http.StatusInternalServerError}
}
