package mercado_pago

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"oficina-tech/internal/modules/billing/domain/payment"

	"github.com/mercadopago/sdk-go/pkg/config"
)

func newTestSDKClient(t *testing.T, server *httptest.Server) *SDKClient {
	t.Helper()
	client, err := NewSDKClient(
		"test-token",
		"https://example.com/v1/payments/mp-webhook",
		"https://example.com",
		config.WithHTTPClient(NewRewritingRequester(server.URL)),
	)
	if err != nil {
		t.Fatalf("NewSDKClient() error = %v", err)
	}
	return client
}

func TestSDKClientCreateOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing authorization header, got: %s", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/orders" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "order-abc",
			"status": "created",
			"transactions": map[string]any{
				"payments": []map[string]any{
					{
						"id":     "pay-1",
						"status": "pending",
						"payment_method": map[string]any{
							"redirect_url": "https://mp.com/checkout/order-abc",
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)

	order, err := client.CreateOrder(context.Background(),
		[]payment.OrderItem{{Title: "Troca de óleo", Quantity: 1, UnitPrice: 150.0}},
		payment.PayerInfo{Email: "cliente@test.com", CPF: "12345678909", Name: "Cliente Teste"},
		"os-uuid-1",
	)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.ID != "order-abc" {
		t.Errorf("expected order ID 'order-abc', got %q", order.ID)
	}
	if order.RedirectURL != "https://mp.com/checkout/order-abc" {
		t.Errorf("expected redirect URL, got %q", order.RedirectURL)
	}
	if order.PaymentID != "pay-1" {
		t.Errorf("expected payment ID 'pay-1', got %q", order.PaymentID)
	}
}

func TestSDKClientRequiresAccessToken(t *testing.T) {
	_, err := NewSDKClient("", "url", "base")
	if err != payment.ErrMissingAccessToken {
		t.Fatalf("expected ErrMissingAccessToken, got %v", err)
	}
}

func TestSDKClientGetOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasPrefix(r.URL.Path, "/v1/orders/") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":     "order-abc",
			"status": "processed",
			"transactions": map[string]any{
				"payments": []map[string]any{
					{"id": "pay-1", "status": "approved", "payment_method": map[string]any{"redirect_url": ""}},
				},
			},
		})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.GetOrder(context.Background(), "order-abc")
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if order.Status != "processed" {
		t.Errorf("expected status 'processed', got %q", order.Status)
	}
}

func TestSDKClientCancelOrder(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/cancel") {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "order-abc", "status": "cancelled",
			"transactions": map[string]any{"payments": []any{}},
		})
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.CancelOrder(context.Background(), "order-abc")
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

func TestTotalAmount(t *testing.T) {
	items := []payment.OrderItem{
		{Quantity: 2, UnitPrice: 50.0},
		{Quantity: 1, UnitPrice: 30.75},
	}
	if got := totalAmount(items); got != "130.75" {
		t.Errorf("expected '130.75', got %q", got)
	}
}
