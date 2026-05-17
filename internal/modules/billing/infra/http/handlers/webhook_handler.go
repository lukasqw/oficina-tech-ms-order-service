package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

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

	log := observability.LoggerFromContext(ctx)

	var payload mercadoPagoWebhookPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("mp_webhook: payload inválido", slog.String("error", err.Error()))
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	// Ignora tipos de webhook que não são pagamentos (ex.: topic_merchant_order_wh).
	// Retorna 200 para evitar retentativas desnecessárias do MP.
	if payload.Type != "" && payload.Type != "payment" {
		log.Info("mp_webhook: tipo ignorado",
			slog.String("webhook_type", payload.Type),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	paymentID := r.URL.Query().Get("data.id")
	if paymentID == "" {
		paymentID = stringifyID(payload.Data.ID)
	}
	if paymentID == "" {
		log.Warn("mp_webhook: payment_id ausente no payload",
			slog.String("webhook_type", payload.Type),
		)
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	xSig := r.Header.Get("x-signature")
	xReqID := r.Header.Get("x-request-id")

	log.Info("mp_webhook: recebido",
		slog.String("payment_id", paymentID),
		slog.String("webhook_type", payload.Type),
		slog.String("x_request_id", xReqID),
		slog.Bool("has_x_signature", xSig != ""),
		slog.String("sig_ts", sigTS(xSig)),
		slog.String("remote_addr", r.RemoteAddr),
	)

	if err := h.validator.Validate(xSig, xReqID, paymentID); err != nil {
		log.Warn("mp_webhook: assinatura inválida",
			slog.String("payment_id", paymentID),
			slog.String("x_request_id", xReqID),
			slog.Bool("has_x_signature", xSig != ""),
			slog.String("sig_ts", sigTS(xSig)),
			slog.String("validation_error", err.Error()),
		)
		utils.RespondErrorEnvelope(w, http.StatusUnauthorized, utils.ErrCodeUnauthorized, payment.ErrInvalidWebhookSignature.Error())
		return
	}

	output, err := h.useCase.Execute(ctx, usecases.HandlePaymentWebhookInput{
		MPOrderID:         paymentID,
		ExternalReference: payload.ExternalReference,
	})
	if err != nil {
		log.Error("mp_webhook: erro ao processar",
			slog.String("payment_id", paymentID),
			slog.String("error", err.Error()),
		)
		span.RecordError(err)
		mapping := utils.MapDomainError(err)
		utils.RespondErrorEnvelope(w, mapping.StatusCode, mapping.Code, err.Error())
		return
	}

	log.Info("mp_webhook: processado",
		slog.String("payment_id", paymentID),
		slog.String("order_id", output.OrderID),
		slog.String("status", output.Status),
		slog.Bool("processed", output.Processed),
	)

	utils.RespondSuccess(w, http.StatusOK, map[string]any{
		"processed": output.Processed,
		"status":    output.Status,
		"order_id":  output.OrderID,
	})
}

// sigTS extrai apenas o campo ts do header x-signature para logging seguro (sem expor o hash).
func sigTS(sig string) string {
	for part := range strings.SplitSeq(sig, ",") {
		k, v, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok && k == "ts" {
			return v
		}
	}
	return ""
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
