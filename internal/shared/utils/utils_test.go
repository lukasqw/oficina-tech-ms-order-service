package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// --- StringPtr ---

func TestStringPtr_ReturnsPointer(t *testing.T) {
	s := "hello"
	p := StringPtr(s)
	if p == nil || *p != s {
		t.Errorf("expected pointer to %q, got %v", s, p)
	}
}

func TestStringPtr_EmptyString(t *testing.T) {
	p := StringPtr("")
	if p == nil || *p != "" {
		t.Error("expected pointer to empty string")
	}
}

// --- RespondSuccess ---

func TestRespondSuccess_SetsStatusAndBody(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondSuccess(rec, http.StatusOK, map[string]string{"key": "value"})

	if rec.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("want application/json, got %s", ct)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if env.Data == nil {
		t.Error("expected non-nil data in envelope")
	}
}

func TestRespondSuccess_Created(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondSuccess(rec, http.StatusCreated, "created")
	if rec.Code != http.StatusCreated {
		t.Errorf("want 201, got %d", rec.Code)
	}
}

// --- RespondErrorEnvelope ---

func TestRespondErrorEnvelope_SetsStatusAndError(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondErrorEnvelope(rec, http.StatusBadRequest, ErrCodeValidationFailed, "campo inválido")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(env.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(env.Errors))
	}
	if env.Errors[0].Code != ErrCodeValidationFailed {
		t.Errorf("want code %s, got %s", ErrCodeValidationFailed, env.Errors[0].Code)
	}
}

func TestRespondErrorEnvelope_Unauthorized(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondErrorEnvelope(rec, http.StatusUnauthorized, ErrCodeUnauthorized, "sem token")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rec.Code)
	}
}

// --- RespondValidationError ---

func TestRespondValidationError_IncludesField(t *testing.T) {
	rec := httptest.NewRecorder()
	RespondValidationError(rec, "email", "e-mail inválido")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rec.Code)
	}

	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Errors) == 0 {
		t.Fatal("expected errors in envelope")
	}
	if env.Errors[0].Field == nil || *env.Errors[0].Field != "email" {
		t.Errorf("expected field 'email', got %v", env.Errors[0].Field)
	}
}

// --- MapDomainError ---

func TestMapDomainError_Nil(t *testing.T) {
	m := MapDomainError(nil)
	if m.StatusCode != http.StatusInternalServerError {
		t.Errorf("nil error should map to 500, got %d", m.StatusCode)
	}
}

func TestMapDomainError_NotFound(t *testing.T) {
	m := MapDomainError(errors.New("ordem de serviço não encontrada"))
	if m.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", m.StatusCode)
	}
	if m.Code != ErrCodeNotFound {
		t.Errorf("want NOT_FOUND, got %s", m.Code)
	}
}

func TestMapDomainError_VehicleOwnership(t *testing.T) {
	m := MapDomainError(errors.New("veículo não pertence ao cliente da ordem de serviço"))
	if m.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400, got %d", m.StatusCode)
	}
}

func TestMapDomainError_InvalidTransition(t *testing.T) {
	m := MapDomainError(errors.New("transição de status inválida"))
	if m.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", m.StatusCode)
	}
}

func TestMapDomainError_AlreadyExists(t *testing.T) {
	m := MapDomainError(errors.New("resource already exists"))
	if m.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", m.StatusCode)
	}
}

func TestMapDomainError_Unauthorized(t *testing.T) {
	m := MapDomainError(errors.New("acesso não autorizado a esta ordem de serviço"))
	if m.StatusCode != http.StatusForbidden {
		t.Errorf("want 403, got %d", m.StatusCode)
	}
}

func TestMapDomainError_InvalidInput(t *testing.T) {
	m := MapDomainError(errors.New("invalid input value"))
	if m.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 for validation error, got %d", m.StatusCode)
	}
}

func TestMapDomainError_UnknownError_FallsBackTo500(t *testing.T) {
	m := MapDomainError(errors.New("something completely unexpected"))
	if m.StatusCode != http.StatusInternalServerError {
		t.Errorf("want 500 for unknown error, got %d", m.StatusCode)
	}
}

func TestMapDomainError_CannotModifyClosedOrder(t *testing.T) {
	m := MapDomainError(errors.New("não é possível modificar ordem fechada"))
	if m.StatusCode != http.StatusConflict {
		t.Errorf("want 409, got %d", m.StatusCode)
	}
}

func TestMapDomainError_ProductNotFound(t *testing.T) {
	m := MapDomainError(errors.New("produto não encontrado"))
	if m.StatusCode != http.StatusNotFound {
		t.Errorf("want 404, got %d", m.StatusCode)
	}
}

// --- FormatTimeRFC3339 / ParseTimeRFC3339 ---

func TestFormatTimeRFC3339_NonEmpty(t *testing.T) {
	s := FormatTimeRFC3339(time.Now())
	if s == "" {
		t.Fatal("expected non-empty formatted time")
	}
}

func TestFormatTimeRFC3339_RoundTrip(t *testing.T) {
	now := time.Now().Truncate(time.Second).UTC()
	s := FormatTimeRFC3339(now)
	ts, err := ParseTimeRFC3339(s)
	if err != nil {
		t.Fatalf("round-trip parse failed: %v", err)
	}
	if !ts.Equal(now) {
		t.Errorf("round-trip mismatch: %v vs %v", now, ts)
	}
}

func TestParseTimeRFC3339_Valid(t *testing.T) {
	ts, err := ParseTimeRFC3339("2024-01-15T10:30:00Z")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ts.IsZero() {
		t.Error("expected non-zero time")
	}
}

func TestParseTimeRFC3339_Invalid(t *testing.T) {
	if _, err := ParseTimeRFC3339("not a time"); err == nil {
		t.Fatal("expected error for invalid time string")
	}
}

// --- UUID utilities ---

func TestGenerateUUIDv7_Valid(t *testing.T) {
	id := GenerateUUIDv7()
	if id == "" {
		t.Fatal("expected non-empty UUID")
	}
	if err := ValidateUUID(id); err != nil {
		t.Errorf("generated UUID is invalid: %v", err)
	}
}

func TestValidateUUID_Valid(t *testing.T) {
	if err := ValidateUUID("550e8400-e29b-41d4-a716-446655440000"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateUUID_Invalid(t *testing.T) {
	if err := ValidateUUID("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

func TestParseUUID_Valid(t *testing.T) {
	u, err := ParseUUID("550e8400-e29b-41d4-a716-446655440000")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if u.String() == "" {
		t.Error("expected non-empty UUID string")
	}
}

func TestParseUUID_Invalid(t *testing.T) {
	if _, err := ParseUUID("not-a-uuid"); err == nil {
		t.Fatal("expected error for invalid UUID")
	}
}

// --- RespondMultipleErrors ---

func TestRespondMultipleErrors_SetsBodyAndStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	errs := []ErrorDetail{
		{Code: ErrCodeValidationFailed, Message: "campo A inválido"},
		{Code: ErrCodeNotFound, Message: "recurso não encontrado"},
	}
	RespondMultipleErrors(rec, http.StatusUnprocessableEntity, errs)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("want 422, got %d", rec.Code)
	}
	var env Envelope
	if err := json.NewDecoder(rec.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(env.Errors) != 2 {
		t.Errorf("expected 2 errors, got %d", len(env.Errors))
	}
}
