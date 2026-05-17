package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// RegisterPaymentSteps wires the Mercado Pago payment scenarios.
func RegisterPaymentSteps(ctx *godog.ScenarioContext, w *World) {
	ctx.Step(`^advance dispara criação de preferência MP \(MP mock retorna preference_id\)$`, w.triggerPaymentPreference)
	ctx.Step(`^MP webhook chega com status=approved \(mock dispara\)$`, w.triggerPaymentWebhook)
	ctx.Step(`^MP webhook chega com status=rejected \(mock dispara\)$`, w.triggerRejectedWebhook)
	ctx.Step(`^MP webhook chega com assinatura inválida$`, w.triggerInvalidSignatureWebhook)
	ctx.Step(`^o webhook é rejeitado com status 4xx$`, w.assertLastWebhookRejected)
	ctx.Step(`^o webhook do MP retoma o fluxo normalmente$`, w.triggerPaymentWebhook)
}

// triggerPaymentPreference advances the OS from COMPLETED → AWAITING_PAYMENT.
// MS2 calls the MP mock to create a preference, persists the payment URL,
// and parks the OS in AWAITING_PAYMENT until the webhook lands.
func (w *World) triggerPaymentPreference(ctx context.Context) error {
	if err := w.advanceServiceOrder(ctx, "AWAITING_PAYMENT"); err != nil {
		return err
	}
	return w.assertOrderStatusEventually(ctx, "AWAITING_PAYMENT")
}

// triggerPaymentWebhook configures the MP mock to respond `approved` for the
// preference's payment_id and POSTs a signed webhook to MS2.
func (w *World) triggerPaymentWebhook(ctx context.Context) error {
	prefID, err := w.lookupPreferenceID(ctx)
	if err != nil {
		return err
	}
	paymentID := strings.TrimPrefix(prefID, "pref-") + "-payment"

	// 1. Tell the mock that GET /v1/payments/{paymentID} should return approved.
	body := map[string]string{
		"status":             "approved",
		"external_reference": w.OrderID,
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost,
		w.MPMockURL+"/__mock/payments/"+paymentID, body, "")
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("mock setup returned %d: %s", status, string(raw))
	}

	// 2. POST the webhook to MS2 with the signature MP would set.
	signature, requestID := signMercadoPagoWebhook(w.WebhookSec, paymentID)
	webhookBody := map[string]any{
		"type":               "payment",
		"data":               map[string]any{"id": paymentID},
		"external_reference": w.OrderID,
	}
	payload, _ := json.Marshal(webhookBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.MS2URL+"/payments/mp-webhook?data.id="+paymentID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-signature", signature)
	req.Header.Set("x-request-id", requestID)

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// triggerRejectedWebhook configures the mock to respond "rejected" for the
// payment, sends the signed webhook to MS2, and captures the HTTP status.
// A rejected payment must not advance the OS past AWAITING_PAYMENT.
func (w *World) triggerRejectedWebhook(ctx context.Context) error {
	prefID, err := w.lookupPreferenceID(ctx)
	if err != nil {
		return err
	}
	paymentID := strings.TrimPrefix(prefID, "pref-") + "-payment"

	mockBody := map[string]string{
		"status":             "rejected",
		"external_reference": w.OrderID,
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost,
		w.MPMockURL+"/__mock/payments/"+paymentID, mockBody, "")
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("mock setup retornou %d: %s", status, string(raw))
	}

	signature, requestID := signMercadoPagoWebhook(w.WebhookSec, paymentID)
	webhookBody := map[string]any{
		"type":               "payment",
		"data":               map[string]any{"id": paymentID},
		"external_reference": w.OrderID,
	}
	payload, _ := json.Marshal(webhookBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.MS2URL+"/payments/mp-webhook?data.id="+paymentID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-signature", signature)
	req.Header.Set("x-request-id", requestID)

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	w.LastWebhookResponseStatus = resp.StatusCode
	return nil
}

// triggerInvalidSignatureWebhook sends a webhook with a tampered signature.
// MS2 must reject it with 4xx without changing the OS status.
func (w *World) triggerInvalidSignatureWebhook(ctx context.Context) error {
	prefID, err := w.lookupPreferenceID(ctx)
	if err != nil {
		return err
	}
	paymentID := strings.TrimPrefix(prefID, "pref-") + "-payment"

	webhookBody := map[string]any{
		"type":               "payment",
		"data":               map[string]any{"id": paymentID},
		"external_reference": w.OrderID,
	}
	payload, _ := json.Marshal(webhookBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.MS2URL+"/payments/mp-webhook?data.id="+paymentID, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-signature", "ts=0,v1=assinatura-invalida")
	req.Header.Set("x-request-id", "req-invalido")

	resp, err := w.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	w.LastWebhookResponseStatus = resp.StatusCode
	return nil
}

func (w *World) assertLastWebhookRejected(_ context.Context) error {
	if w.LastWebhookResponseStatus < 400 {
		return fmt.Errorf("esperado 4xx, webhook retornou %d", w.LastWebhookResponseStatus)
	}
	return nil
}

// lookupPreferenceID waits for MS2 to populate mp_preference_id on the OS
// after AWAITING_PAYMENT is reached, then returns it.
func (w *World) lookupPreferenceID(ctx context.Context) (string, error) {
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := w.fetchOrderRaw(ctx)
		if err == nil {
			if id, ok := raw["mp_preference_id"].(string); ok && id != "" {
				return id, nil
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", fmt.Errorf("mp_preference_id never populated on order %s", w.OrderID)
}
