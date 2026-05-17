package handlers

import (
	"net/http"

	"oficina-tech/internal/modules/billing/application/usecases"
	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/shared/infra/observability"
	"oficina-tech/internal/shared/utils"
)

type PaymentHandler struct {
	getPaymentStatus *usecases.GetPaymentStatus
	retryPayment     *usecases.RetryPayment
}

func NewPaymentHandler(getPaymentStatus *usecases.GetPaymentStatus, retryPayment *usecases.RetryPayment) *PaymentHandler {
	return &PaymentHandler{getPaymentStatus: getPaymentStatus, retryPayment: retryPayment}
}

func (h *PaymentHandler) GetServiceOrderPayment(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "billing.get_service_order_payment")
	defer span.End()

	serviceOrderID := r.PathValue("id")
	if err := utils.ValidateUUID(serviceOrderID); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	output, err := h.getPaymentStatus.Execute(ctx, serviceOrderID)
	if err != nil {
		span.RecordError(err)
		if err == payment.ErrPaymentURLNotAvailable {
			utils.RespondErrorEnvelope(w, http.StatusNotFound, utils.ErrCodeNotFound, err.Error())
			return
		}
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	utils.RespondSuccess(w, http.StatusOK, map[string]string{
		"payment_url": output.PaymentURL,
		"mp_order_id": output.OrderID,
		"status":      output.Status,
	})
}

// RetryPayment cria um novo Order MP para uma OS em PAYMENT_REJECTED,
// permitindo que o cliente tente pagar novamente.
// POST /service-orders/{id}/retry-payment
func (h *PaymentHandler) RetryPayment(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "billing.retry_payment")
	defer span.End()

	serviceOrderID := r.PathValue("id")
	if err := utils.ValidateUUID(serviceOrderID); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidUUID, "Invalid service order ID format")
		return
	}

	if h.retryPayment == nil {
		utils.RespondErrorEnvelope(w, http.StatusInternalServerError, utils.ErrCodeInternalError, "retry payment not configured")
		return
	}

	output, err := h.retryPayment.Execute(ctx, serviceOrderID)
	if err != nil {
		span.RecordError(err)
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	utils.RespondSuccess(w, http.StatusAccepted, map[string]string{
		"payment_url": output.PaymentURL,
		"mp_order_id": output.MPOrderID,
	})
}
