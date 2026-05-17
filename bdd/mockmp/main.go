// Package main implementa um mock mínimo da Mercado Pago Preferences API para o suite BDD/E2E.
//
// Endpoints consumidos pelo MS2 (Preferences API):
//
//	POST /checkout/preferences              → cria preference, retorna {id, sandbox_init_point}
//	GET  /v1/payments/{id}                  → retorna payment (status, external_reference)
//	POST /v1/payments/{id}/refunds          → estorna payment
//
// Endpoints legacy (Orders API) mantidos para compatibilidade:
//
//	POST /v1/orders                         → (não mais usado)
//	GET  /v1/orders/{id}                    → (não mais usado)
//	POST /v1/orders/{id}/cancel             → (não mais usado)
//	POST /v1/orders/{id}/refund             → (não mais usado)
//
// Endpoints admin (somente testes):
//
//	POST /__mock/orders/{id}                → configura status/payment_status/status_detail da preference
//	POST /__mock/orders/{id}/trigger        → dispara webhook de payment ao MS2
//	GET  /__mock/orders                     → lista todas as preferences criadas
//	POST /__mock/reset                      → limpa estado
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type orderRecord struct {
	ID                  string
	Status              string // created, cancelled
	ExternalReference   string
	PaymentID           string
	PaymentStatus       string // pending, approved, rejected, cancelled, refunded
	PaymentStatusDetail string
	RedirectURL         string
}

type store struct {
	mu     sync.RWMutex
	orders map[string]orderRecord
}

func newStore() *store {
	return &store{orders: map[string]orderRecord{}}
}

func main() {
	addr := os.Getenv("MOCK_MP_ADDR")
	if addr == "" {
		addr = ":9999"
	}

	s := newStore()
	mux := http.NewServeMux()

	// Preferences API
	mux.HandleFunc("POST /checkout/preferences", s.handleCreatePreference)

	// Payments API
	mux.HandleFunc("GET /v1/payments/{id}", s.handleGetPayment)
	mux.HandleFunc("POST /v1/payments/{id}/refunds", s.handleRefundPayment)

	// Orders API (legacy — mantido para compatibilidade com testes antigos)
	mux.HandleFunc("POST /v1/orders", s.handleCreateOrder)
	mux.HandleFunc("GET /v1/orders/{id}", s.handleGetOrder)
	mux.HandleFunc("POST /v1/orders/{id}/cancel", s.handleCancelOrder)
	mux.HandleFunc("POST /v1/orders/{id}/refund", s.handleRefundOrder)

	// Admin
	mux.HandleFunc("POST /__mock/orders/{id}", s.handleSetOrder)
	mux.HandleFunc("POST /__mock/orders/{id}/trigger", s.handleTriggerWebhook)
	mux.HandleFunc("GET /__mock/orders", s.handleListOrders)
	mux.HandleFunc("POST /__mock/reset", s.handleReset)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	log.Printf("mp-mock (Preferences API) listening on %s", addr)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("mp-mock failed: %v", err)
	}
}

// ─── Preferences API ─────────────────────────────────────────────────────────

func (s *store) handleCreatePreference(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExternalReference string `json:"external_reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	preferenceID := "pref-" + body.ExternalReference
	if body.ExternalReference == "" {
		preferenceID = fmt.Sprintf("pref-anon-%d", time.Now().UnixNano())
	}
	// Numeric payment ID simula o que o MP real retorna.
	paymentID := fmt.Sprintf("%d", time.Now().UnixNano()/1000)

	rec := orderRecord{
		ID:                preferenceID,
		Status:            "created",
		ExternalReference: body.ExternalReference,
		PaymentID:         paymentID,
		PaymentStatus:     "pending",
		RedirectURL:       "https://mp-mock.local/sandbox-checkout/" + preferenceID,
	}

	s.mu.Lock()
	s.orders[preferenceID] = rec
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                 preferenceID,
		"init_point":         "https://mp-mock.local/checkout/" + preferenceID,
		"sandbox_init_point": "https://mp-mock.local/sandbox-checkout/" + preferenceID,
		"external_reference": body.ExternalReference,
	})
}

// ─── Payments API ─────────────────────────────────────────────────────────────

func (s *store) handleGetPayment(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("id")
	s.mu.RLock()
	var found *orderRecord
	for _, rec := range s.orders {
		rec := rec
		if rec.PaymentID == paymentID {
			found = &rec
			break
		}
	}
	s.mu.RUnlock()
	if found == nil {
		http.Error(w, "payment not found: "+paymentID, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                 paymentID,
		"status":             found.PaymentStatus,
		"status_detail":      found.PaymentStatusDetail,
		"external_reference": found.ExternalReference,
	})
}

func (s *store) handleRefundPayment(w http.ResponseWriter, r *http.Request) {
	paymentID := r.PathValue("id")
	s.mu.Lock()
	var found bool
	for k, rec := range s.orders {
		if rec.PaymentID == paymentID {
			rec.PaymentStatus = "refunded"
			s.orders[k] = rec
			found = true
			break
		}
	}
	s.mu.Unlock()
	if !found {
		http.Error(w, "payment not found: "+paymentID, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":     paymentID,
		"status": "refunded",
	})
}

// ─── Orders API (legacy) ──────────────────────────────────────────────────────

func (s *store) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ExternalReference string `json:"external_reference"`
		TotalAmount       string `json:"total_amount"`
		Currency          string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	orderID := "order-" + body.ExternalReference
	if body.ExternalReference == "" {
		orderID = fmt.Sprintf("order-anon-%d", time.Now().UnixNano())
	}
	paymentID := "pay-" + orderID
	redirectURL := "https://mp-mock.local/checkout/" + orderID

	rec := orderRecord{
		ID:                orderID,
		Status:            "created",
		ExternalReference: body.ExternalReference,
		PaymentID:         paymentID,
		PaymentStatus:     "pending",
		RedirectURL:       redirectURL,
	}

	s.mu.Lock()
	s.orders[orderID] = rec
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orderResponse(rec))
}

func (s *store) handleGetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	rec, ok := s.orders[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "order not found: "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orderResponse(rec))
}

func (s *store) handleCancelOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	rec, ok := s.orders[id]
	if ok {
		rec.Status = "cancelled"
		rec.PaymentStatus = "cancelled"
		s.orders[id] = rec
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "order not found: "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orderResponse(rec))
}

func (s *store) handleRefundOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.Lock()
	rec, ok := s.orders[id]
	if ok {
		rec.Status = "cancelled"
		rec.PaymentStatus = "refunded"
		s.orders[id] = rec
	}
	s.mu.Unlock()
	if !ok {
		http.Error(w, "order not found: "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(orderResponse(rec))
}

// ─── Admin ───────────────────────────────────────────────────────────────────

func (s *store) handleSetOrder(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status        string `json:"status"`
		PaymentStatus string `json:"payment_status"`
		StatusDetail  string `json:"status_detail"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	rec, ok := s.orders[id]
	if ok {
		if body.Status != "" {
			rec.Status = strings.ToLower(body.Status)
		}
		if body.PaymentStatus != "" {
			rec.PaymentStatus = strings.ToLower(body.PaymentStatus)
		}
		rec.PaymentStatusDetail = body.StatusDetail
		s.orders[id] = rec
	}
	s.mu.Unlock()

	if !ok {
		http.Error(w, "order not found: "+id, http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *store) handleTriggerWebhook(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	rec, ok := s.orders[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "order not found: "+id, http.StatusNotFound)
		return
	}

	notificationURL := os.Getenv("MP_NOTIFICATION_URL")
	if notificationURL == "" {
		http.Error(w, "MP_NOTIFICATION_URL not configured", http.StatusInternalServerError)
		return
	}
	// Envia o payment ID no data.id para simular webhook da Preferences API.
	if err := sendWebhook(notificationURL, rec.PaymentID); err != nil {
		http.Error(w, "webhook delivery failed: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *store) handleListOrders(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]any, 0, len(s.orders))
	for _, rec := range s.orders {
		out = append(out, map[string]any{
			"id":                 rec.ID,
			"status":             rec.Status,
			"external_reference": rec.ExternalReference,
			"payment_id":         rec.PaymentID,
			"payment_status":     rec.PaymentStatus,
		})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *store) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.orders = map[string]orderRecord{}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func orderResponse(rec orderRecord) map[string]any {
	return map[string]any{
		"id":                 rec.ID,
		"status":             rec.Status,
		"external_reference": rec.ExternalReference,
		"transactions": map[string]any{
			"payments": []any{
				map[string]any{
					"id":           rec.PaymentID,
					"status":       rec.PaymentStatus,
					"status_detail": rec.PaymentStatusDetail,
					"payment_method": map[string]any{
						"redirect_url": rec.RedirectURL,
					},
				},
			},
		},
	}
}

// sendWebhook envia um webhook de payment ao MS2 com o paymentID no data.id.
// A assinatura usa o paymentID para construir o manifest — igual ao que a
// SignatureValidator do MS2 espera.
func sendWebhook(notificationURL, paymentID string) error {
	secret := os.Getenv("MP_WEBHOOK_SECRET")
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	requestID := fmt.Sprintf("mock-%s-%d", paymentID, time.Now().UnixNano())
	manifest := "id:" + paymentID + ";request-id:" + requestID + ";ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	sig := hex.EncodeToString(mac.Sum(nil))

	payload := map[string]any{
		"type":   "payment",
		"action": "payment.updated",
		"data":   map[string]string{"id": paymentID},
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, notificationURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-request-id", requestID)
	req.Header.Set("x-signature", "ts="+ts+",v1="+sig)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return nil
}
