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

// --- NoOpClient ---

func TestNoOpClientCreateOrder(t *testing.T) {
	client := NewNoOpClient()
	order, err := client.CreateOrder(context.Background(),
		[]payment.OrderItem{{Title: "item", Quantity: 1, UnitPrice: 100}},
		payment.PayerInfo{Email: "a@b.com"},
		"external-ref",
	)
	if err != nil {
		t.Fatalf("CreateOrder() error = %v", err)
	}
	if order.ID != "mock-order-external-ref" {
		t.Errorf("expected mock-order-external-ref, got %s", order.ID)
	}
	if order.RedirectURL == "" {
		t.Error("expected non-empty RedirectURL")
	}
}

func TestNoOpClientGetOrder(t *testing.T) {
	client := NewNoOpClient()
	order, err := client.GetOrder(context.Background(), "order-123")
	if err != nil {
		t.Fatalf("GetOrder() error = %v", err)
	}
	if order.ID != "order-123" || order.Status != "processed" {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestNoOpClientCancelOrder(t *testing.T) {
	client := NewNoOpClient()
	order, err := client.CancelOrder(context.Background(), "order-123")
	if err != nil {
		t.Fatalf("CancelOrder() error = %v", err)
	}
	if order.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %s", order.Status)
	}
}

func TestNoOpClientRefundOrder(t *testing.T) {
	client := NewNoOpClient()
	order, err := client.RefundOrder(context.Background(), "order-123", nil)
	if err != nil {
		t.Fatalf("RefundOrder() error = %v", err)
	}
	if order.Status != "refunded" {
		t.Fatalf("expected refunded, got %s", order.Status)
	}
}

func TestNoOpClientGetPayment(t *testing.T) {
	client := NewNoOpClient()
	p, err := client.GetPayment(context.Background(), "pay-123")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if p.ID != "pay-123" || p.Status != "approved" {
		t.Fatalf("unexpected payment: %+v", p)
	}
}

// --- NewSDKClientFromEnv ---

func TestNewSDKClientFromEnv_BasicToken(t *testing.T) {
	t.Setenv("MP_ACCESS_TOKEN", "TEST-fake-token")
	t.Setenv("MP_NOTIFICATION_URL", "https://example.com/webhook")
	t.Setenv("MP_CALLBACK_BASE_URL", "https://example.com")
	t.Setenv("MP_BASE_URL", "")
	t.Setenv("MP_SANDBOX", "")

	client, err := NewSDKClientFromEnv()
	if err != nil {
		t.Fatalf("NewSDKClientFromEnv() error = %v", err)
	}
	if client == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestNewSDKClientFromEnv_WithBaseURL(t *testing.T) {
	t.Setenv("MP_ACCESS_TOKEN", "TEST-fake-token")
	t.Setenv("MP_NOTIFICATION_URL", "https://example.com/webhook")
	t.Setenv("MP_CALLBACK_BASE_URL", "https://example.com")
	t.Setenv("MP_BASE_URL", "https://mp-api.example.com")
	t.Setenv("MP_SANDBOX", "")

	client, err := NewSDKClientFromEnv()
	if err != nil {
		t.Fatalf("NewSDKClientFromEnv() error = %v", err)
	}
	if client.apiBaseURL == "" {
		t.Error("expected apiBaseURL to be set from MP_BASE_URL")
	}
}

func TestNewSDKClientFromEnv_WithSandboxMode(t *testing.T) {
	t.Setenv("MP_ACCESS_TOKEN", "TEST-fake-token")
	t.Setenv("MP_NOTIFICATION_URL", "https://example.com/webhook")
	t.Setenv("MP_CALLBACK_BASE_URL", "https://example.com")
	t.Setenv("MP_BASE_URL", "")
	t.Setenv("MP_SANDBOX", "true")

	client, err := NewSDKClientFromEnv()
	if err != nil {
		t.Fatalf("NewSDKClientFromEnv() error = %v", err)
	}
	if !client.isSandbox {
		t.Error("expected isSandbox=true when MP_SANDBOX=true")
	}
}

func TestNewSDKClientFromEnv_MissingToken(t *testing.T) {
	t.Setenv("MP_ACCESS_TOKEN", "")
	_, err := NewSDKClientFromEnv()
	if err == nil {
		t.Fatal("expected error for missing access token")
	}
}

// --- RefundOrder branches ---

func TestSDKClientRefundOrderEmptyID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call for empty payment ID")
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	order, err := client.RefundOrder(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("expected no error for empty ID, got %v", err)
	}
	if order == nil {
		t.Fatal("expected non-nil order")
	}
}

func TestSDKClientRefundOrderInvalidAmount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected HTTP call for invalid amount")
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	badAmt := "not-a-float"
	_, err := client.RefundOrder(context.Background(), "12345", &badAmt)
	if err == nil {
		t.Fatal("expected error for invalid amount string")
	}
}

func TestSDKClientRefundOrderPartial(t *testing.T) {
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
	amt := "50.00"
	order, err := client.RefundOrder(context.Background(), "12345", &amt)
	if err != nil {
		t.Fatalf("RefundOrder() partial error = %v", err)
	}
	if order.Status != "refunded" {
		t.Errorf("expected status 'refunded', got %q", order.Status)
	}
}

// --- GetPayment branches ---

func TestSDKClientGetPaymentNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	_, err := client.GetPayment(context.Background(), "999")
	if err != payment.ErrOrderNotFound {
		t.Fatalf("expected ErrOrderNotFound, got %v", err)
	}
}

func TestSDKClientGetPaymentHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := newTestSDKClient(t, server)
	_, err := client.GetPayment(context.Background(), "123")
	if err == nil {
		t.Fatal("expected error for non-200 status")
	}
}

func TestFirstWord_NoSpace(t *testing.T) {
	if got := firstWord("hello"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestFirstWord_WithSpace(t *testing.T) {
	if got := firstWord("hello world"); got != "hello" {
		t.Errorf("expected 'hello', got %q", got)
	}
}

func TestRestWords_NoSpace(t *testing.T) {
	if got := restWords("hello"); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestRestWords_WithSpace(t *testing.T) {
	if got := restWords("hello world"); got != "world" {
		t.Errorf("expected 'world', got %q", got)
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
