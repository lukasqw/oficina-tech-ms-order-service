package handlers

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	billingUsecases "oficina-tech/internal/modules/billing/application/usecases"
	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/billing/infra/mercado_pago"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
)

func TestWebhookHandlerPaymentStatuses(t *testing.T) {
	tests := []struct {
		name       string
		mpStatus   string
		wantStatus service_order.OrderStatus
	}{
		{"approved", "approved", service_order.StatusPaid},
		{"rejected", "rejected", service_order.StatusCompleted},
		{"pending", "pending", service_order.StatusAwaitingPayment},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newHandlerOrderRepo(handlerAwaitingPaymentOrder(t))
			handler := newTestWebhookHandler(repo, &handlerMPClient{
				payment: &payment.Payment{ID: "pay-1", Status: tt.mpStatus, ExternalReference: handlerOrderID},
			})

			req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook", bytes.NewBufferString(`{"data":{"id":"pay-1"}}`))
			signRequest(req, "pay-1")
			rr := httptest.NewRecorder()

			handler.Handle(rr, req)

			if rr.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
			}
			if repo.orders[handlerOrderID].Status() != tt.wantStatus {
				t.Fatalf("expected status %s, got %s", tt.wantStatus, repo.orders[handlerOrderID].Status())
			}
		})
	}
}

func TestWebhookHandlerRejectsInvalidSignature(t *testing.T) {
	handler := newTestWebhookHandler(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t)), &handlerMPClient{
		payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: handlerOrderID},
	})

	req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook", bytes.NewBufferString(`{"data":{"id":"pay-1"}}`))
	req.Header.Set("x-request-id", "request-id")
	req.Header.Set("x-signature", "ts=123,v1=bad")
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rr.Code)
	}
}

func TestWebhookHandlerRejectsMalformedPayload(t *testing.T) {
	handler := newTestWebhookHandler(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t)), &handlerMPClient{})
	req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook", bytes.NewBufferString(`{`))
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestWebhookHandlerUsesQueryPaymentIDForSignature(t *testing.T) {
	repo := newHandlerOrderRepo(handlerAwaitingPaymentOrder(t))
	handler := newTestWebhookHandler(repo, &handlerMPClient{
		payment: &payment.Payment{ID: "pay-query", Status: "approved", ExternalReference: handlerOrderID},
	})
	req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook?data.id=pay-query", bytes.NewBufferString(`{"data":{"id":"ignored"}}`))
	signRequest(req, "pay-query")
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if repo.orders[handlerOrderID].Status() != service_order.StatusPaid {
		t.Fatalf("expected paid, got %s", repo.orders[handlerOrderID].Status())
	}
}

func TestWebhookHandlerReturnsUseCaseError(t *testing.T) {
	handler := newTestWebhookHandler(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t)), &handlerMPClient{
		err: errors.New("mp unavailable"),
	})
	req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook", bytes.NewBufferString(`{"data":{"id":"pay-1"}}`))
	signRequest(req, "pay-1")
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rr.Code)
	}
}

func TestWebhookHandlerRejectsPayloadWithoutPaymentID(t *testing.T) {
	handler := newTestWebhookHandler(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t)), &handlerMPClient{})
	req := httptest.NewRequest(http.MethodPost, "/payments/mp-webhook", bytes.NewBufferString(`{"data":{}}`))
	rr := httptest.NewRecorder()

	handler.Handle(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestStringifyID(t *testing.T) {
	if stringifyID("abc") != "abc" {
		t.Fatalf("string id failed")
	}
	if stringifyID(float64(123)) != "123" {
		t.Fatalf("numeric id failed")
	}
	if stringifyID(nil) != "" {
		t.Fatalf("nil id failed")
	}
	if stringifyID(true) != "true" {
		t.Fatalf("default id failed")
	}
}

const (
	handlerOrderID    = "11111111-1111-4111-8111-111111111111"
	handlerCustomerID = "22222222-2222-4222-8222-222222222222"
	handlerVehicleID  = "33333333-3333-4333-8333-333333333333"
	handlerSecret     = "secret"
)

func newTestWebhookHandler(repo *handlerOrderRepo, mpClient *handlerMPClient) *WebhookHandler {
	return NewWebhookHandler(
		mercado_pago.NewSignatureValidator(handlerSecret),
		billingUsecases.NewHandlePaymentWebhook(mpClient, repo, &handlerHistoryRepo{}, &handlerCustomerAdapter{}, email.NewMockEmailService()),
	)
}

func signRequest(req *http.Request, paymentID string) {
	requestID := "request-id"
	ts := "1742505638683"
	manifest := "id:" + paymentID + ";request-id:" + requestID + ";ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(handlerSecret))
	_, _ = mac.Write([]byte(manifest))
	req.Header.Set("x-request-id", requestID)
	req.Header.Set("x-signature", "ts="+ts+",v1="+hex.EncodeToString(mac.Sum(nil)))
}

type handlerMPClient struct {
	payment *payment.Payment
	err     error
}

func (c *handlerMPClient) CreatePreference(context.Context, string, []payment.PreferenceItem, string) (*payment.Preference, error) {
	return nil, nil
}

func (c *handlerMPClient) GetPayment(context.Context, string) (*payment.Payment, error) {
	if c.err != nil {
		return nil, c.err
	}
	return c.payment, nil
}

type handlerCustomerAdapter struct{}

func (a *handlerCustomerAdapter) GetCustomerByID(context.Context, string) (*dto.CustomerDTO, error) {
	return &dto.CustomerDTO{ID: handlerCustomerID, Name: "Maria", Email: "maria@example.com"}, nil
}

type handlerOrderRepo struct {
	orders map[string]*service_order.ServiceOrder
}

func newHandlerOrderRepo(order *service_order.ServiceOrder) *handlerOrderRepo {
	return &handlerOrderRepo{orders: map[string]*service_order.ServiceOrder{order.ID(): order}}
}

func (r *handlerOrderRepo) Save(_ context.Context, order *service_order.ServiceOrder) error {
	r.orders[order.ID()] = order
	return nil
}

func (r *handlerOrderRepo) SaveWithItems(ctx context.Context, order *service_order.ServiceOrder) error {
	return r.Save(ctx, order)
}

func (r *handlerOrderRepo) FindByID(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.orders[id], nil
}

func (r *handlerOrderRepo) FindByIDWithItems(ctx context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(ctx, id)
}

func (r *handlerOrderRepo) FindAll(context.Context) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *handlerOrderRepo) FindAllWithFilters(context.Context, service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *handlerOrderRepo) FindByCustomerID(context.Context, string) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *handlerOrderRepo) FindByStatus(context.Context, service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *handlerOrderRepo) FindBySagaStatus(context.Context, string) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *handlerOrderRepo) Delete(context.Context, string) error {
	return nil
}

func (r *handlerOrderRepo) UpdateItemsHistoryID(context.Context, []string, string) error {
	return nil
}

type handlerHistoryRepo struct{}

func (r *handlerHistoryRepo) Save(context.Context, *service_order.History) error {
	return nil
}

func (r *handlerHistoryRepo) FindByServiceOrderID(context.Context, string) ([]*service_order.History, error) {
	return nil, nil
}

func (r *handlerHistoryRepo) FindByID(context.Context, string) (*service_order.History, error) {
	return nil, nil
}

func handlerAwaitingPaymentOrder(t *testing.T) *service_order.ServiceOrder {
	t.Helper()
	prefID := "pref-1"
	paymentURL := "https://pay/pref-1"
	order, err := service_order.ReconstructServiceOrder(
		handlerOrderID, handlerCustomerID, handlerVehicleID, "test",
		service_order.StatusAwaitingPayment, service_order.SagaStatusAwaitingPayment,
		nil, nil, nil, &prefID, nil, &paymentURL, nil, nil, time.Now(), time.Now(), nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	return order
}
