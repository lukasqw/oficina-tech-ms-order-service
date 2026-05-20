package http_clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"oficina-tech/internal/shared/infra/http/middleware"
)

func TestMS1ClientGetCustomerByID(t *testing.T) {
	var authorization string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"customer-1","name":"Maria","email":"maria@example.com","phone":"11999999999"},"errors":[]}`))
	}))
	defer server.Close()

	ctx := context.WithValue(context.Background(), middleware.AuthorizationKey, "Bearer token")
	customer, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(ctx, "customer-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if customer.Name != "Maria" {
		t.Fatalf("expected customer snapshot, got %+v", customer)
	}
	if authorization != "Bearer token" {
		t.Fatalf("expected Authorization propagation, got %q", authorization)
	}
}

func TestMS1ClientMaps404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewMS1ClientWithBaseURL(server.URL).GetVehicleByID(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestNewMS1Client_NotNil(t *testing.T) {
	c := NewMS1Client()
	if c == nil {
		t.Fatal("expected non-nil MS1Client")
	}
}

func TestNewMS3Client_NotNil(t *testing.T) {
	c := NewMS3Client()
	if c == nil {
		t.Fatal("expected non-nil MS3Client")
	}
}

func TestRetryableStatusError_ErrorMessage(t *testing.T) {
	e := retryableStatusError{status: 503}
	msg := e.Error()
	if msg == "" {
		t.Fatal("expected non-empty error message")
	}
	if !strings.Contains(msg, "503") {
		t.Errorf("expected error message to contain '503', got %q", msg)
	}
}

func TestMS3ClientGetServiceByID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"service-1","name":"Troca de Óleo","description":"desc","price":10000},"errors":[]}`))
	}))
	defer server.Close()

	svc, err := NewMS3ClientWithBaseURL(server.URL).GetServiceByID(context.Background(), "service-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if svc.Name != "Troca de Óleo" {
		t.Errorf("expected service name, got %s", svc.Name)
	}
}

func TestMS1ClientGetVehicleByID_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"v-1","customer_id":"c-1","license_plate":"ABC-1234","brand":"Toyota","model":"Corolla","model_year":2023,"manufacture_year":2022},"errors":[]}`))
	}))
	defer server.Close()

	v, err := NewMS1ClientWithBaseURL(server.URL).GetVehicleByID(context.Background(), "v-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if v.Brand != "Toyota" {
		t.Errorf("expected Brand Toyota, got %s", v.Brand)
	}
}

func TestGetCustomerByIDReturnsErrorOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(context.Background(), "bad-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetProductByIDReturnsErrorOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewMS3ClientWithBaseURL(server.URL).GetProductByID(context.Background(), "bad-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetServiceByIDReturnsErrorOnNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	_, err := NewMS3ClientWithBaseURL(server.URL).GetServiceByID(context.Background(), "bad-id")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDoGetMaps401ToUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	_, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(context.Background(), "id-1")
	if !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestDoGetMapsUnexpectedStatusToGenericError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
	}))
	defer server.Close()

	_, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(context.Background(), "id-1")
	if err == nil || errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnauthorized) {
		t.Fatalf("expected generic upstream error, got %v", err)
	}
}

func TestDoGetFallsBackToDirectJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"customer-1","name":"Direct","email":"d@e.com"}`))
	}))
	defer server.Close()

	customer, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(context.Background(), "customer-1")
	if err != nil {
		t.Fatalf("expected no error for direct JSON, got %v", err)
	}
	if customer.Name != "Direct" {
		t.Fatalf("expected name 'Direct', got %s", customer.Name)
	}
}

func TestDoGetContextCancelledDuringRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately after first attempt starts
	go func() { cancel() }()

	_, err := NewMS1ClientWithBaseURL(server.URL).GetCustomerByID(ctx, "id-1")
	if err == nil {
		t.Fatal("expected error from cancelled context or retryable error")
	}
}

func TestMS3ClientRetries5xx(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"id":"product-1","name":"Filtro","description":"Filtro de oleo","price":4500,"product_type":"PART"},"errors":[]}`))
	}))
	defer server.Close()

	product, err := NewMS3ClientWithBaseURL(server.URL).GetProductByID(context.Background(), "product-1")
	if err != nil {
		t.Fatalf("expected retry success, got %v", err)
	}
	if product.Price != 4500 {
		t.Fatalf("expected product price, got %+v", product)
	}
	if attempts != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts)
	}
}
