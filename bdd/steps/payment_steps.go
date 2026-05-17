package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/cucumber/godog"
)

// RegisterPaymentSteps wires the Mercado Pago payment scenarios.
func RegisterPaymentSteps(ctx *godog.ScenarioContext, w *World) {
	// Step antigo mantido para compatibilidade com saga_compensation e service_order_lifecycle.
	ctx.Step(`^advance dispara criação de preferência MP \(MP mock retorna preference_id\)$`, w.triggerPaymentPreference)
	// Novo step — Orders API (usado em payment_flow.feature).
	ctx.Step(`^advance cria Order MP \(mock retorna mp_order_id\)$`, w.triggerPaymentPreference)

	ctx.Step(`^MP webhook chega com status=approved \(mock dispara\)$`, w.triggerPaymentWebhook)
	ctx.Step(`^MP webhook chega com status=rejected \(mock dispara\)$`, w.triggerRejectedWebhook)
	ctx.Step(`^MP webhook chega com assinatura inválida$`, w.triggerInvalidSignatureWebhook)
	ctx.Step(`^o webhook é rejeitado com status 4xx$`, w.assertLastWebhookRejected)
	// Mantido para saga_recovery.feature.
	ctx.Step(`^o webhook do MP retoma o fluxo normalmente$`, w.triggerPaymentWebhook)

	ctx.Step(`^retry-payment é chamado$`, w.retryPayment)
	ctx.Step(`^o mock recebeu chamada de cancel no MP$`, w.assertMockReceivedCancel)
	ctx.Step(`^o mock recebeu chamada de refund no MP$`, w.assertMockReceivedRefund)
}

// triggerPaymentPreference advances the OS from COMPLETED → AWAITING_PAYMENT.
// MS2 calls the MP mock to create an Order, persists the payment URL,
// and parks the OS in AWAITING_PAYMENT until the webhook lands.
func (w *World) triggerPaymentPreference(ctx context.Context) error {
	if err := w.advanceServiceOrder(ctx, "AWAITING_PAYMENT"); err != nil {
		return err
	}
	return w.assertOrderStatusEventually(ctx, "AWAITING_PAYMENT")
}

// triggerPaymentWebhook configures the MP mock to return `approved` for the
// Order and POSTs a signed Orders API webhook to MS2.
func (w *World) triggerPaymentWebhook(ctx context.Context) error {
	orderID, err := w.lookupOrderID(ctx)
	if err != nil {
		return err
	}

	// Tell the mock that GET /v1/orders/{orderID} should return approved.
	status, raw, err := doJSON(ctx, w, http.MethodPost,
		w.MPMockURL+"/__mock/orders/"+orderID,
		map[string]string{"payment_status": "approved"}, "")
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("mock setup returned %d: %s", status, string(raw))
	}

	return w.postOrderWebhook(ctx, orderID)
}

// triggerRejectedWebhook configures the mock to return "rejected" and sends
// the signed webhook to MS2. The OS must transition to PAYMENT_REJECTED.
func (w *World) triggerRejectedWebhook(ctx context.Context) error {
	orderID, err := w.lookupOrderID(ctx)
	if err != nil {
		return err
	}

	status, raw, err := doJSON(ctx, w, http.MethodPost,
		w.MPMockURL+"/__mock/orders/"+orderID,
		map[string]string{"payment_status": "rejected"}, "")
	if err != nil {
		return err
	}
	if status != http.StatusNoContent {
		return fmt.Errorf("mock setup retornou %d: %s", status, string(raw))
	}

	return w.postOrderWebhook(ctx, orderID)
}

// triggerInvalidSignatureWebhook sends a webhook with a tampered signature.
// MS2 must reject it with 4xx without changing the OS status.
func (w *World) triggerInvalidSignatureWebhook(ctx context.Context) error {
	orderID, err := w.lookupOrderID(ctx)
	if err != nil {
		return err
	}

	webhookBody := map[string]any{
		"type":   "order",
		"action": "order.updated",
		"data":   map[string]any{"id": orderID},
	}
	payload, _ := json.Marshal(webhookBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.MS2URL+"/payments/mp-webhook?data.id="+orderID, bytes.NewReader(payload))
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

// retryPayment calls POST /service-orders/{id}/retry-payment.
// Applicable when OS is in PAYMENT_REJECTED; transitions it back to AWAITING_PAYMENT.
func (w *World) retryPayment(ctx context.Context) error {
	url := fmt.Sprintf("%s/service-orders/%s/retry-payment", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodPost, url, map[string]any{}, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusAccepted {
		return fmt.Errorf("retry-payment returned %d: %s", status, string(raw))
	}
	return nil
}

// assertMockReceivedCancel verifies that the MP mock registered a Cancel call
// for the current OS (payment_status = "cancelled").
func (w *World) assertMockReceivedCancel(ctx context.Context) error {
	return w.assertMockOrderPaymentStatus(ctx, "cancelled")
}

// assertMockReceivedRefund verifies that the MP mock registered a Refund call
// for the current OS (payment_status = "refunded").
func (w *World) assertMockReceivedRefund(ctx context.Context) error {
	return w.assertMockOrderPaymentStatus(ctx, "refunded")
}

// assertMockOrderPaymentStatus polls GET /__mock/orders, finds the order whose
// external_reference matches the current OS, and asserts the payment status.
func (w *World) assertMockOrderPaymentStatus(ctx context.Context, expected string) error {
	deadline := time.Now().Add(10 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		status, raw, err := doJSON(ctx, w, http.MethodGet, w.MPMockURL+"/__mock/orders", nil, "")
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		if status != http.StatusOK {
			time.Sleep(300 * time.Millisecond)
			continue
		}
		// Mock returns a plain JSON array (no envelope).
		var orders []map[string]any
		if err := json.Unmarshal(raw, &orders); err != nil {
			return fmt.Errorf("decode mock orders: %w", err)
		}
		for _, o := range orders {
			extRef, _ := o["external_reference"].(string)
			if extRef != w.OrderID {
				continue
			}
			txs, _ := o["transactions"].(map[string]any)
			payments, _ := txs["payments"].([]any)
			if len(payments) == 0 {
				return fmt.Errorf("mock order has no payments for OS %s", w.OrderID)
			}
			p, _ := payments[0].(map[string]any)
			got, _ := p["status"].(string)
			if got == expected {
				return nil
			}
			last = got
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("mock payment status: expected %q, last=%q for OS %s", expected, last, w.OrderID)
}

// postOrderWebhook signs and delivers an Orders API webhook for the given orderID.
func (w *World) postOrderWebhook(ctx context.Context, orderID string) error {
	signature, requestID := signMercadoPagoWebhook(w.WebhookSec, orderID)
	webhookBody := map[string]any{
		"type":   "order",
		"action": "order.updated",
		"data":   map[string]any{"id": orderID},
	}
	payload, _ := json.Marshal(webhookBody)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		w.MS2URL+"/payments/mp-webhook?data.id="+orderID, bytes.NewReader(payload))
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

// lookupOrderID polls GET /service-orders/{id}/payment until mp_order_id is
// populated, then returns it. Uses the dedicated payment endpoint (not the
// full order endpoint which does not expose MP fields).
func (w *World) lookupOrderID(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/service-orders/%s/payment", w.MS2URL, w.OrderID)
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		status, raw, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
		if err == nil && status == http.StatusOK {
			var resp map[string]string
			if err := decodeData(raw, &resp); err == nil {
				if id := resp["mp_order_id"]; id != "" {
					return id, nil
				}
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return "", fmt.Errorf("mp_order_id never populated on order %s", w.OrderID)
}
