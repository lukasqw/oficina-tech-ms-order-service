// Package main implementa um mock mínimo da Mercado Pago Orders API para o suite BDD/E2E.
//
// Endpoints consumidos pelo MS2 (Orders API v1):
//
//	POST /v1/orders                      → cria order, retorna {id, status, transactions.payments[0].payment_method.redirect_url}
//	GET  /v1/orders/{id}                 → retorna estado atual do order
//	POST /v1/orders/{id}/cancel          → cancela order
//	POST /v1/orders/{id}/refund          → estorna order
//	GET  /v1/payments/{id}               → retorna payment (compat. com pkg/payment do SDK)
//
// Endpoints admin (somente testes):
//
//	POST /__mock/orders/{id}             → configura status/payment_status/status_detail do order
//	POST /__mock/orders/{id}/trigger     → dispara webhook manualmente ao MS2
//	GET  /__mock/orders                  → lista todos os orders criados
//	POST /__mock/reset                   → limpa estado
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
	ID                string
	Status            string // created, processed, action_required, cancelled
	ExternalReference string
	TotalAmount       string
	Currency          string
	PaymentID         string
	PaymentStatus     string // pending, approved, rejected, cancelled
	PaymentStatusDetail string
	RedirectURL       string
	CancelledAt       *time.Time
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

	// Orders API
	mux.HandleFunc("POST /v1/orders", s.handleCreateOrder)
	mux.HandleFunc("GET /v1/orders/{id}", s.handleGetOrder)
	mux.HandleFunc("POST /v1/orders/{id}/cancel", s.handleCancelOrder)
	mux.HandleFunc("POST /v1/orders/{id}/refund", s.handleRefundOrder)

	// Payments API (compatibilidade com SDK pkg/payment)
	mux.HandleFunc("GET /v1/payments/{id}", s.handleGetPayment)

	// Admin
	mux.HandleFunc("POST /__mock/orders/{id}", s.handleSetOrder)
	mux.HandleFunc("POST /__mock/orders/{id}/trigger", s.handleTriggerWebhook)
	mux.HandleFunc("GET /__mock/orders", s.handleListOrders)
	mux.HandleFunc("POST /__mock/reset", s.handleReset)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	log.Printf("mp-mock (Orders API) listening on %s", addr)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("mp-mock failed: %v", err)
	}
}

// ─── Orders API ──────────────────────────────────────────────────────────────

func (s *store) handleCreateOrder(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Type              string `json:"type"`
		TotalAmount       string `json:"total_amount"`
		ExternalReference string `json:"external_reference"`
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
		TotalAmount:       body.TotalAmount,
		Currency:          body.Currency,
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
		t := time.Now()
		rec.CancelledAt = &t
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
	_, ok := s.orders[id]
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
	if err := sendWebhook(notificationURL, id); err != nil {
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
		out = append(out, orderResponse(rec))
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
		"total_amount":       rec.TotalAmount,
		"currency":           rec.Currency,
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

func sendWebhook(notificationURL, orderID string) error {
	secret := os.Getenv("MP_WEBHOOK_SECRET")
	ts := fmt.Sprintf("%d", time.Now().UnixMilli())
	requestID := fmt.Sprintf("mock-%s-%d", orderID, time.Now().UnixNano())
	manifest := "id:" + orderID + ";request-id:" + requestID + ";ts:" + ts + ";"
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(manifest))
	sig := hex.EncodeToString(mac.Sum(nil))

	payload := map[string]any{
		"type":   "order",
		"action": "order.updated",
		"data":   map[string]string{"id": orderID},
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
