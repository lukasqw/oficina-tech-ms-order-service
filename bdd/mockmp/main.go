// Package main implements a tiny Mercado Pago API mock used by the BDD/E2E suite.
//
// It exposes two endpoints consumed by MS2:
//   - POST /checkout/preferences  → returns a deterministic preference_id and init_point.
//   - GET  /v1/payments/{id}      → returns the status configured via POST /__mock/payments/{id}.
//
// The mock also exposes admin helpers used only by tests:
//   - POST /__mock/payments/{id} body={"status": "approved", "external_reference": "<order_id>"}
//   - GET  /__mock/preferences   → returns every preference recorded so far
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type paymentRecord struct {
	ID                string `json:"id"`
	Status            string `json:"status"`
	ExternalReference string `json:"external_reference"`
}

type preferenceRecord struct {
	ID                string `json:"id"`
	ExternalReference string `json:"external_reference"`
	InitPoint         string `json:"init_point"`
}

type store struct {
	mu          sync.RWMutex
	payments    map[string]paymentRecord
	preferences map[string]preferenceRecord
}

func newStore() *store {
	return &store{
		payments:    map[string]paymentRecord{},
		preferences: map[string]preferenceRecord{},
	}
}

func main() {
	addr := os.Getenv("MOCK_MP_ADDR")
	if addr == "" {
		addr = ":9999"
	}

	s := newStore()
	mux := http.NewServeMux()

	mux.HandleFunc("POST /checkout/preferences", s.handleCreatePreference)
	mux.HandleFunc("GET /v1/payments/{id}", s.handleGetPayment)
	mux.HandleFunc("POST /__mock/payments/{id}", s.handleSetPayment)
	mux.HandleFunc("GET /__mock/preferences", s.handleListPreferences)
	mux.HandleFunc("POST /__mock/reset", s.handleReset)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	log.Printf("mp-mock listening on %s", addr)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Fatalf("mp-mock failed: %v", err)
	}
}

func (s *store) handleCreatePreference(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Items             []map[string]any `json:"items"`
		ExternalReference string           `json:"external_reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	id := "pref-" + body.ExternalReference
	if id == "pref-" {
		id = "pref-anon-" + time.Now().Format("150405.000000000")
	}
	s.mu.Lock()
	s.preferences[id] = preferenceRecord{
		ID:                id,
		ExternalReference: body.ExternalReference,
		InitPoint:         "https://mp-mock.local/checkout/" + id,
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                 id,
		"init_point":         "https://mp-mock.local/checkout/" + id,
		"sandbox_init_point": "https://mp-mock.local/sandbox/" + id,
		"external_reference": body.ExternalReference,
	})
}

func (s *store) handleGetPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	s.mu.RLock()
	record, ok := s.payments[id]
	s.mu.RUnlock()
	if !ok {
		http.Error(w, "payment not found: "+id, http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id":                 record.ID,
		"status":             record.Status,
		"external_reference": record.ExternalReference,
	})
}

func (s *store) handleSetPayment(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var body struct {
		Status            string `json:"status"`
		ExternalReference string `json:"external_reference"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Status == "" {
		body.Status = "approved"
	}
	s.mu.Lock()
	s.payments[id] = paymentRecord{
		ID:                id,
		Status:             strings.ToLower(body.Status),
		ExternalReference: body.ExternalReference,
	}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

func (s *store) handleListPreferences(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]preferenceRecord, 0, len(s.preferences))
	for _, p := range s.preferences {
		out = append(out, p)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *store) handleReset(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.payments = map[string]paymentRecord{}
	s.preferences = map[string]preferenceRecord{}
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
