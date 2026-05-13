package http_clients

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
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
