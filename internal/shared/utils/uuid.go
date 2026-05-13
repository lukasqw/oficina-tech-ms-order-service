package utils

import (
	"fmt"

	"github.com/google/uuid"
)

// GenerateUUIDv7 gera um novo UUID versão 7
func GenerateUUIDv7() string {
	return uuid.Must(uuid.NewV7()).String()
}

// ValidateUUID valida se uma string é um UUID válido
func ValidateUUID(id string) error {
	_, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid UUID format: %w", err)
	}
	return nil
}

// ParseUUID converte string para uuid.UUID
func ParseUUID(id string) (uuid.UUID, error) {
	return uuid.Parse(id)
}
