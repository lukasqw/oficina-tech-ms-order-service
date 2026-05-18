package handlers

import (
	"encoding/json"
	"fmt"
	"io"
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
	Action            string `json:"action"`
	APIVersion        string `json:"api_version"`
	LiveMode          bool   `json:"live_mode"`
	ExternalReference string `json:"external_reference"`
	Data              struct {
		ID any `json:"id"`
	} `json:"data"`
}

func (h *WebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	ctx, span := observability.SpanHandler(r.Context(), "billing.mp_webhook")
	defer span.End()

	log := observability.LoggerFromContext(ctx)

	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		log.Warn("mp_webhook: erro ao ler body", slog.String("error", err.Error()))
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	var payload mercadoPagoWebhookPayload
	if err := json.Unmarshal(rawBody, &payload); err != nil {
		log.Warn("mp_webhook: payload inválido",
			slog.String("error", err.Error()),
			slog.String("raw_body", string(rawBody)),
			slog.String("query_raw", r.URL.RawQuery),
			slog.String("remote_addr", r.RemoteAddr),
		)
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	// Extrai tudo antes de qualquer filtro para garantir log completo.
	xSig := r.Header.Get("x-signature")
	xReqID := r.Header.Get("x-request-id")
	paymentID := r.URL.Query().Get("data.id")
	if paymentID == "" {
		paymentID = stringifyID(payload.Data.ID)
	}

	log.Info("mp_webhook: recebido",
		slog.String("payment_id", paymentID),
		slog.String("webhook_type", payload.Type),
		slog.String("action", payload.Action),
		slog.String("api_version", payload.APIVersion),
		slog.Bool("live_mode", payload.LiveMode),
		slog.String("query_raw", r.URL.RawQuery),
		slog.String("x_request_id", xReqID),
		slog.Bool("has_x_signature", xSig != ""),
		slog.String("x_signature_raw", xSig),
		slog.String("sig_ts", sigTS(xSig)),
		slog.String("remote_addr", r.RemoteAddr),
		slog.String("payload_json", string(rawBody)),
	)

	// Aceita "order" (Orders API) e "payment" (Payments API legacy).
	// Ignora demais tipos (topic_merchant_order_wh, etc.) com 200 para suprimir retentativas.
	if payload.Type != "" && payload.Type != "order" && payload.Type != "payment" {
		log.Info("mp_webhook: tipo ignorado",
			slog.String("webhook_type", payload.Type),
			slog.String("action", payload.Action),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	// Formato legado "MercadoPago Feed v2.0": envia ?id=X&topic=merchant_order em vez de
	// ?data.id=X com type no body. Retorna 200 para suprimir retentativas.
	if topic := r.URL.Query().Get("topic"); topic != "" {
		log.Info("mp_webhook: formato legado ignorado",
			slog.String("topic", topic),
			slog.String("legacy_id", r.URL.Query().Get("id")),
			slog.String("user_agent", r.Header.Get("User-Agent")),
		)
		w.WriteHeader(http.StatusOK)
		return
	}

	if paymentID == "" {
		log.Warn("mp_webhook: payment_id ausente no payload",
			slog.String("webhook_type", payload.Type),
			slog.String("action", payload.Action),
		)
		utils.RespondErrorEnvelope(w, http.StatusBadRequest, utils.ErrCodeInvalidRequest, payment.ErrMalformedWebhook.Error())
		return
	}

	if err := h.validator.Validate(xSig, xReqID, paymentID); err != nil {
		log.Warn("mp_webhook: assinatura inválida",
			slog.String("payment_id", paymentID),
			slog.String("action", payload.Action),
			slog.String("x_request_id", xReqID),
			slog.Bool("has_x_signature", xSig != ""),
			slog.String("sig_ts", sigTS(xSig)),
			slog.String("validation_error", err.Error()),
		)
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
