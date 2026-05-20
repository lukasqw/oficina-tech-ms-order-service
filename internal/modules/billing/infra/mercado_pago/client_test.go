package mercado_pago

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"oficina-tech/internal/modules/billing/domain/payment"

	"github.com/mercadopago/sdk-go/pkg/config"
)

func newTestSDKClient(t *testing.T, server *httptest.Server) *SDKClient {
	t.Helper()
	// TEST- prefix → isSandbox=true, usando sandbox_init_point no CreateOrder.
	client, err := NewSDKClient(
		"TEST-token",
		"https://example.com/v1/payments/mp-webhook",
		"https://example.com",
		config.WithHTTPClient(NewRewritingRequester(server.URL)),
	)
	if err != nil {
		t.Fatalf("NewSDKClient() error = %v", err)
	}
	// GetPayment ainda usa HTTP direto via apiBaseURL.
	client.apiBaseURL = server.URL
	return client
}

func TestSDKClientCreateOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/checkout/preferences" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":                 "pref-abc",
			"init_point":         "https://mp.com/checkout/pref-abc",
			"sandbox_init_point": "https://mp.com/sandbox/pref-abc",
			"external_reference": "os-uuid-1",
		})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.CreateOrder(context.Background(),
		[]payment.OrderItem{{Title: "Troca de óleo", Quantity: 1, UnitPrice: 150.0}},
		payment.PayerInfo{Email: "cliente@testuser.com", CPF: "12345678909", Name: "Cliente Teste"},
		"os-uuid-1",
	)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.ID != "pref-abc" {
		t.Errorf("expected preference ID 'pref-abc', got %q", order.ID)
	}
	// isSandbox=true → sandbox_init_point deve ser usado como RedirectURL.
	if order.RedirectURL != "https://mp.com/sandbox/pref-abc" {
		t.Errorf("expected sandbox redirect URL, got %q", order.RedirectURL)
	}
}

func TestSDKClientRequiresAccessToken(t *testing.T) {
	_, err := NewSDKClient("", "url", "base")
	if err != payment.ErrMissingAccessToken {
		t.Fatalf("expected ErrMissingAccessToken, got %v", err)
	}
}

func TestSDKClientGetOrder(t *testing.T) {
	// GetOrder é stub na Preferences API — retorna ErrOrderNotFound sem chamada HTTP.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call in GetOrder")
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	_, err := client.GetOrder(context.Background(), "pref-abc")
	if err != payment.ErrOrderNotFound {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestSDKClientCancelOrder(t *testing.T) {
	// CancelOrder é no-op na Preferences API — nenhuma chamada HTTP esperada.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call in CancelOrder")
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.CancelOrder(context.Background(), "pref-abc")
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if order.Status != "cancelled" {
		t.Errorf("expected status 'cancelled', got %q", order.Status)
	}
}

func TestSDKClientGetPayment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/payments/123" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": 123, "status": "approved", "status_detail": "accredited",
			"external_reference": "os-uuid-1",
		})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	p, err := client.GetPayment(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if p.Status != "approved" {
		t.Errorf("expected status 'approved', got %q", p.Status)
	}
	if p.ExternalReference != "os-uuid-1" {
		t.Errorf("expected external_reference 'os-uuid-1', got %q", p.ExternalReference)
	}
}

func TestSDKClientRefundOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payments/12345/refunds" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 1, "status": "approved"})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.RefundOrder(context.Background(), "12345", nil)
	if err != nil {
		t.Fatalf("RefundOrder() error = %v", err)
	}
	if order.Status != "refunded" {
		t.Errorf("expected status 'refunded', got %q", order.Status)
	}
}

func TestSDKClientRefundOrderInvalidID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call for non-numeric payment ID")
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	_, err := client.RefundOrder(context.Background(), "pay-abc", nil)
	if err == nil {
		t.Fatal("expected error for non-numeric payment ID")
	}
}

func TestTaxIDType(t *testing.T) {
	if taxIDType("12345678909") != "CPF" {
		t.Error("11 digits should be CPF")
	}
	if taxIDType("12345678000195") != "CNPJ" {
		t.Error("14 digits should be CNPJ")
	}
	if taxIDType("123.456.789-09") != "CPF" {
		t.Error("formatted CPF should be detected as CPF")
	}
}
