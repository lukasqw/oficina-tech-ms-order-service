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
}

func NewPaymentHandler(getPaymentStatus *usecases.GetPaymentStatus) *PaymentHandler {
	return &PaymentHandler{getPaymentStatus: getPaymentStatus}
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
		"payment_url":      output.PaymentURL,
		"mp_preference_id": output.PreferenceID,
		"status":           output.Status,
	})
}
