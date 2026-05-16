// Package steps holds the Godog step definitions for the MS2 BDD/E2E suite.
//
// A new World is built per scenario (see RegisterScenario). It carries the
// HTTP base URLs, the admin JWT, and the IDs created during the scenario so
// later steps can reference them.
package steps

import (
	"net/http"
	"os"
	"sync"
	"time"
)

// World is the per-scenario state.
type World struct {
	HTTP *http.Client

	MS1URL     string
	MS2URL     string
	MS3URL     string
	MPMockURL  string
	WebhookSec string
	AdminEmail string
	AdminPass  string

	AdminToken    string
	CustomerToken string

	// Identifiers captured during the scenario.
	CustomerID       string
	CustomerCPF      string
	CustomerPassword string
	VehicleID        string
	ProductID  string
	ServiceID  string
	OrderID    string

	// Default item used by most scenarios — a product reference + quantity.
	OrderItem orderItem
	OrderItem2 orderItem
}

type orderItem struct {
	ItemType    string `json:"item_type"`
	ReferenceID string `json:"reference_id"`
	Quantity    int    `json:"quantity"`
}

func defaultEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newWorld() *World {
	return &World{
		HTTP: &http.Client{Timeout: 30 * time.Second},

		MS1URL:     defaultEnv("MS1_URL", "http://localhost:8081"),
		MS2URL:     defaultEnv("MS2_URL", "http://localhost:8082"),
		MS3URL:     defaultEnv("MS3_URL", "http://localhost:8083"),
		MPMockURL:  defaultEnv("MP_MOCK_URL", "http://localhost:9999"),
		WebhookSec: defaultEnv("MP_WEBHOOK_SECRET", "e2e-webhook-secret"),
		AdminEmail: defaultEnv("ADMIN_EMAIL", "admin@oficina.test"),
		AdminPass:  defaultEnv("ADMIN_PASSWORD", "admin1234"),
	}
}

// adminTokenOnce caches the admin login across scenarios in one go test run —
// the JWT TTL is comfortably longer than a full suite execution.
var (
	adminTokenOnce sync.Once
	cachedAdmin    string
	cachedAdminErr error
)
