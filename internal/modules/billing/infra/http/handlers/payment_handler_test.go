package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	billingUsecases "oficina-tech/internal/modules/billing/application/usecases"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

func TestPaymentHandlerReturnsPaymentURL(t *testing.T) {
	handler := NewPaymentHandler(billingUsecases.NewGetPaymentStatus(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t))), nil)
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+handlerOrderID+"/payment", nil)
	req.SetPathValue("id", handlerOrderID)
	rr := httptest.NewRecorder()

	handler.GetServiceOrderPayment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "https://pay/pref-1") {
		t.Fatalf("response does not include payment URL: %s", rr.Body.String())
	}
}

func TestPaymentHandlerReturns404WhenNotAwaitingPayment(t *testing.T) {
	handler := NewPaymentHandler(billingUsecases.NewGetPaymentStatus(newHandlerOrderRepo(handlerCompletedOrder(t))), nil)
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+handlerOrderID+"/payment", nil)
	req.SetPathValue("id", handlerOrderID)
	rr := httptest.NewRecorder()

	handler.GetServiceOrderPayment(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestPaymentHandlerRejectsInvalidUUID(t *testing.T) {
	handler := NewPaymentHandler(billingUsecases.NewGetPaymentStatus(newHandlerOrderRepo(handlerAwaitingPaymentOrder(t))), nil)
	req := httptest.NewRequest(http.MethodGet, "/service-orders/bad/payment", nil)
	req.SetPathValue("id", "bad")
	rr := httptest.NewRecorder()

	handler.GetServiceOrderPayment(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func handlerCompletedOrder(t *testing.T) *service_order.ServiceOrder {
	t.Helper()
	order, err := service_order.ReconstructServiceOrder(
		handlerOrderID, handlerCustomerID, handlerVehicleID, "test",
		service_order.StatusCompleted, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil, nil, nil, time.Now(), time.Now(), nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	return order
}
