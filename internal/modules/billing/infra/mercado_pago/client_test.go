package mercado_pago

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"oficina-tech/internal/modules/billing/domain/payment"
)

func TestClientCreatePreferenceAndGetPayment(t *testing.T) {
	var preferenceAttempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing authorization header")
		}
		switch r.URL.Path {
		case "/checkout/preferences":
			preferenceAttempts++
			if preferenceAttempts == 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			var req preferenceRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode preference request: %v", err)
			}
			if req.ExternalReference != "order-1" || req.NotificationURL != "https://example.com/payments/mp-webhook" {
				t.Fatalf("unexpected preference request: %+v", req)
			}
			_ = json.NewEncoder(w).Encode(preferenceResponse{ID: "pref-1", InitPoint: "https://pay/pref-1"})
		case "/v1/payments/123":
			_ = json.NewEncoder(w).Encode(paymentResponse{ID: 123, Status: "approved", ExternalReference: "order-1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientFromEnv(
		WithBaseURL(server.URL),
		WithAccessToken("token"),
		WithNotificationURL("https://example.com/payments/mp-webhook"),
	)

	preference, err := client.CreatePreference(context.Background(), "order-1", []payment.PreferenceItem{
		{Title: "Troca de óleo", Quantity: 1, UnitPrice: 150},
	}, "order-1")
	if err != nil {
		t.Fatalf("CreatePreference() error = %v", err)
	}
	if preference.ID != "pref-1" || preference.InitURL != "https://pay/pref-1" {
		t.Fatalf("unexpected preference: %+v", preference)
	}
	if preferenceAttempts != 2 {
		t.Fatalf("expected one retry, got %d attempts", preferenceAttempts)
	}

	mpPayment, err := client.GetPayment(context.Background(), "123")
	if err != nil {
		t.Fatalf("GetPayment() error = %v", err)
	}
	if mpPayment.ID != "123" || mpPayment.Status != "approved" || mpPayment.ExternalReference != "order-1" {
		t.Fatalf("unexpected payment: %+v", mpPayment)
	}
}

func TestClientRequiresAccessToken(t *testing.T) {
	client := NewClientFromEnv(WithAccessToken(""))
	if _, err := client.GetPayment(context.Background(), "123"); err != payment.ErrMissingAccessToken {
		t.Fatalf("expected ErrMissingAccessToken, got %v", err)
	}
}

func TestClientReturnsMercadoPagoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
	defer server.Close()

	client := NewClientFromEnv(WithBaseURL(server.URL), WithAccessToken("token"))
	if _, err := client.GetPayment(context.Background(), "123"); err == nil {
		t.Fatalf("expected Mercado Pago error")
	}
}

func TestClientUsesSandboxInitPointFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(preferenceResponse{ID: "pref-2", SandboxInitPoint: "https://sandbox/pref-2"})
	}))
	defer server.Close()

	client := NewClientFromEnv(WithBaseURL(server.URL), WithAccessToken("token"))
	preference, err := client.CreatePreference(context.Background(), "order-2", nil, "")
	if err != nil {
		t.Fatalf("CreatePreference() error = %v", err)
	}
	if preference.InitURL != "https://sandbox/pref-2" {
		t.Fatalf("expected sandbox fallback, got %+v", preference)
	}
}
