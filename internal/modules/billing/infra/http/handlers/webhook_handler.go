package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"oficina-tech/internal/modules/billing/application/usecases"
	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/billing/infra/mercado_pago"
	"oficina-tech/internal/shared/infra/observability"
	"oficina-tech/internal/shared/utils"
)

type WebhookHandler struct {
	validator *mercado_pago.SignatureValidator
	useCase   *usecases.HandlePaymentWebhook
}

func NewWebhookHandler(validator *mercado_pago.SignatureValidator, useCase *usecases.HandlePaymentWebhook) *WebhookHandler {
	return &WebhookHandler{validator: validator, useCase: useCase}
}

type mercadoPagoWebhookPayload struct {
	Type              string `json:"type"`
	ExternalReference string `json:"external_reference"`
	Data              struct {
		ID any `json:"id"`
	} `json:"data"`
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "billing.mp_webhook")
	defer span.End()

	var payload mercadoPagoWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	paymentID := r.URL.Query().Get("data.id")
	if paymentID == "" {
		paymentID = stringifyID(payload.Data.ID)
	}
	if paymentID == "" {
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	if err := h.validator.Validate(r.Header.Get("x-signature"), r.Header.Get("x-request-id"), paymentID); err != nil {
		utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, payment.ErrInvalidWebhookSignature.Error())
		return
	}

	output, err := h.useCase.Execute(ctx, usecases.HandlePaymentWebhookInput{
		PaymentID:         paymentID,
		ExternalReference: payload.ExternalReference,
	})
	if err != nil {
		span.RecordError(err)
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	utils.RespondSuccess(w, http.StatusOK, map[string]any{
		"processed": output.Processed,
		"status":    output.Status,
		"order_id":  output.OrderID,
	})
}

func stringifyID(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case float64:
		return fmt.Sprintf("%.0f", typed)
	default:
		return fmt.Sprint(typed)
	}
}
