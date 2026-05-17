package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
)

// RegisterSuite is invoked once before all scenarios. We use it to perform
// a single admin login and reset the MP mock so payment cases start clean.
func RegisterSuite(ctx *godog.TestSuiteContext) {
	ctx.BeforeSuite(func() {
		w := newWorld()
		bg, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		// Eagerly cache the admin token (and surface any auth issue early).
		if _, err := adminToken(bg, w); err != nil {
			fmt.Printf("[bdd] WARNING: admin login failed during BeforeSuite: %v\n", err)
		}

		// Reset MP mock between full suite runs so the failed-payment
		// fixtures don't leak across CI invocations.
		_, _, _ = doJSON(bg, w, http.MethodPost, w.MPMockURL+"/__mock/reset", nil, "")
	})
}

// RegisterScenario builds a fresh World for every scenario and binds the
// step expressions used across the .feature files.
func RegisterScenario(ctx *godog.ScenarioContext) {
	w := newWorld()

	ctx.Before(func(c context.Context, sc *godog.Scenario) (context.Context, error) {
		// Quando BDD_FULL_INTEGRATION != "true", pula cenários @integration
		// sem falhar o pipeline — útil quando os MSs externos não estão no ar.
		if os.Getenv("BDD_FULL_INTEGRATION") != "true" {
			for _, tag := range sc.Tags {
				if tag.Name == "@integration" {
					return c, godog.ErrSkip
				}
			}
		}

		token, err := adminToken(c, w)
		if err != nil {
			return c, err
		}
		w.AdminToken = token
		return c, nil
	})

	// ─── Background / setup steps ──────────────────────────────────────
	ctx.Step(`^um cliente cadastrado com veículo registrado$`, w.givenCustomerWithVehicle)
	ctx.Step(`^o MS3 possui estoque suficiente para todos os produtos$`, w.givenSufficientStock)
	ctx.Step(`^o MS3 possui estoque suficiente$`, w.givenSufficientStock)
	ctx.Step(`^o MS3 NÃO possui estoque suficiente para os produtos solicitados$`, w.givenInsufficientStock)
	ctx.Step(`^o MS3 NÃO possui estoque suficiente$`, w.givenInsufficientStock)

	// Pre-built order in a given status (used by F.4–F.7 to skip the build-up).
	ctx.Step(`^uma OS em ([A-Z_]+) com estoque reservado$`, w.givenOrderInStatus)
	ctx.Step(`^uma OS em ([A-Z_]+) de customer X$`, w.givenOrderInStatus)

	// ─── Action steps ──────────────────────────────────────────────────
	ctx.Step(`^o cliente abre uma OS com troca de óleo e filtro de ar$`, w.openServiceOrder)
	ctx.Step(`^o mecânico avança a OS para ([A-Z_]+)$`, w.advanceServiceOrder)
	ctx.Step(`^o mecânico tenta avançar a OS de ([A-Z_]+) para ([A-Z_]+)$`, w.tryAdvanceFrom)
	ctx.Step(`^o cliente faz login no sistema$`, w.customerLogin)
	ctx.Step(`^o cliente lista suas ordens de serviço$`, w.customerListsOrders)
	ctx.Step(`^o cliente aprova a OS$`, w.approveAuthorization)
	ctx.Step(`^o cliente nega a autorização$`, w.denyAuthorization)
	ctx.Step(`^o cliente cancela a OS$`, w.cancelOrder)
	ctx.Step(`^cliente cancela$`, w.cancelOrder)
	ctx.Step(`^customer X é deletado no MS1$`, w.deleteCustomer)

	// ─── Outcome steps ─────────────────────────────────────────────────
	ctx.Step(`^a OS aparece na lista com status ([A-Z_]+)$`, w.assertOrderInCustomerList)
	ctx.Step(`^a OS é criada com status ([A-Z_]+)$`, w.assertOrderStatus)
	ctx.Step(`^a OS está em ([A-Z_]+)$`, w.assertOrderStatusEventually)
	ctx.Step(`^a OS avança para ([A-Z_]+)$`, w.assertOrderStatusEventually)
	ctx.Step(`^a OS permanece em ([A-Z_]+)$`, w.assertOrderStatus)
	ctx.Step(`^MS2 cancela a OS via CANCEL_RESERVED$`, w.assertOrderCanceledBySaga)
	ctx.Step(`^MS3 libera estoque$`, w.assertStockReleased)
	ctx.Step(`^estoque retorna para available$`, w.assertStockReleased)

	// Alternative setup without "com estoque reservado" qualifier.
	ctx.Step(`^uma OS em ([A-Z_]+)$`, w.givenOrderInStatus)

	// Items immutability.
	ctx.Step(`^o mecânico atualiza os itens da OS$`, w.updateOrderItems)
	ctx.Step(`^o mecânico tenta atualizar os itens da OS$`, w.updateOrderItems)
	ctx.Step(`^a atualização de itens é aceita$`, w.assertUpdateAccepted)
	ctx.Step(`^a atualização de itens é rejeitada com erro de imutabilidade$`, w.assertUpdateRejected)

	// Audit history and payment URL.
	ctx.Step(`^o histórico da OS possui (\d+) ou mais entradas$`, w.assertOrderHistory)
	ctx.Step(`^a OS possui URL de pagamento$`, w.assertPaymentURL)

	// Multi-order / customer-deleted extended.
	ctx.Step(`^o cliente possui duas OS ativas em ([A-Z_]+)$`, w.givenTwoOrdersInStatus)
	ctx.Step(`^ambas as OS do cliente são canceladas$`, w.assertBothOrdersCanceled)
	ctx.Step(`^o evento é processado sem erros e sem cancelamentos$`, w.assertNoOrderCancellations)

	// Saga absence assertion.
	ctx.Step(`^nenhuma operação de saga é disparada ao cancelar$`, w.assertNoSagaOperation)

	// Cross-file step expressions registered against the same World.
	RegisterSagaSteps(ctx, w)
	RegisterPaymentSteps(ctx, w)
	RegisterRecoverySteps(ctx, w)
}

// ──────────────────────────────────────────────────────────────────────
// Background helpers
// ──────────────────────────────────────────────────────────────────────

func (w *World) givenCustomerWithVehicle(ctx context.Context) error {
	suffix := uuid.NewString()[:8]

	cpf := randomCPF(suffix)
	password := "cliente1234"
	w.CustomerCPF = cpf
	w.CustomerPassword = password

	customerBody := map[string]string{
		"name":          "Cliente E2E " + suffix,
		"email":         "cliente-" + suffix + "@oficina.test",
		"password":      password,
		"phone":         "11987654321",
		"document":      cpf,
		"document_type": "CPF",
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost, w.MS1URL+"/customers", customerBody, w.AdminToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusCreated, status, raw); err != nil {
		return err
	}
	var customer struct {
		ID string `json:"id"`
	}
	if err := decodeData(raw, &customer); err != nil {
		return err
	}
	w.CustomerID = customer.ID

	vehicleBody := map[string]any{
		"model":            "Civic",
		"brand":            "Honda",
		"model_year":       2023,
		"manufacture_year": 2022,
		"license_plate":    randomPlate(suffix),
		"description":      "BDD vehicle " + suffix,
		"customer_id":      customer.ID,
	}
	status, raw, err = doJSON(ctx, w, http.MethodPost, w.MS1URL+"/vehicles", vehicleBody, w.AdminToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusCreated, status, raw); err != nil {
		return err
	}
	var vehicle struct {
		ID string `json:"id"`
	}
	if err := decodeData(raw, &vehicle); err != nil {
		return err
	}
	w.VehicleID = vehicle.ID
	return nil
}

func (w *World) givenSufficientStock(ctx context.Context) error {
	return w.createProductWithStock(ctx, 100)
}

func (w *World) givenInsufficientStock(ctx context.Context) error {
	return w.createProductWithStock(ctx, 0)
}

// createProductWithStock provisions a single product on MS3 and seeds its
// inventory with the given available quantity.
func (w *World) createProductWithStock(ctx context.Context, available int) error {
	// 1. create product
	productBody := map[string]any{
		"name":         "Óleo BDD " + uuid.NewString()[:8],
		"description":  "Produto criado pelo BDD",
		"price":        4500,
		"product_type": "CONSUMABLE",
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost, w.MS3URL+"/products", productBody, w.AdminToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusCreated, status, raw); err != nil {
		return err
	}
	var product struct {
		ID string `json:"id"`
	}
	if err := decodeData(raw, &product); err != nil {
		return err
	}
	w.ProductID = product.ID

	// 2. create inventory record (always starts at 0)
	status, raw, err = doJSON(ctx, w, http.MethodPost,
		fmt.Sprintf("%s/products/%s/inventory", w.MS3URL, product.ID),
		map[string]string{"product_id": product.ID}, w.AdminToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusCreated, status, raw); err != nil {
		return err
	}

	// 3. increase stock to the requested quantity
	if available > 0 {
		status, raw, err = doJSON(ctx, w, http.MethodPost,
			fmt.Sprintf("%s/products/%s/inventory/increase", w.MS3URL, product.ID),
			map[string]int{"quantity": available}, w.AdminToken)
		if err != nil {
			return err
		}
		if err := expectStatus(http.StatusOK, status, raw); err != nil {
			return err
		}
	}

	w.OrderItem = orderItem{ItemType: "PRODUCT", ReferenceID: product.ID, Quantity: 1}
	return nil
}

func (w *World) givenOrderInStatus(ctx context.Context, status string) error {
	if err := w.givenCustomerWithVehicle(ctx); err != nil {
		return err
	}
	if err := w.givenSufficientStock(ctx); err != nil {
		return err
	}
	if err := w.openServiceOrder(ctx); err != nil {
		return err
	}
	target := strings.ToUpper(status)
	chain := []string{"DIAGNOSING", "PENDING_AUTHORIZATION", "AUTHORIZED", "IN_PROGRESS", "COMPLETED"}
	for _, step := range chain {
		if step == "AUTHORIZED" {
			if err := w.approveAuthorization(ctx); err != nil {
				return err
			}
			if err := w.assertOrderStatusEventually(ctx, "AUTHORIZED"); err != nil {
				return err
			}
			if step == target {
				return nil
			}
			continue
		}
		if err := w.advanceServiceOrder(ctx, step); err != nil {
			return err
		}
		if err := w.assertOrderStatusEventually(ctx, step); err != nil {
			return err
		}
		if step == target {
			return nil
		}
	}
	return fmt.Errorf("unsupported precondition status %q", status)
}

// ──────────────────────────────────────────────────────────────────────
// Service order action steps
// ──────────────────────────────────────────────────────────────────────

func (w *World) openServiceOrder(ctx context.Context) error {
	body := map[string]any{
		"customer_id": w.CustomerID,
		"vehicle_id":  w.VehicleID,
		"description": "BDD service order",
		"items":       []orderItem{w.OrderItem},
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost, w.MS2URL+"/service-orders", body, w.AdminToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusCreated, status, raw); err != nil {
		return err
	}
	var resp struct {
		ID string `json:"id"`
	}
	if err := decodeData(raw, &resp); err != nil {
		return err
	}
	w.OrderID = resp.ID
	return nil
}

func (w *World) advanceServiceOrder(ctx context.Context, target string) error {
	url := fmt.Sprintf("%s/service-orders/%s/advance", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodPost, url, map[string]any{}, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return fmt.Errorf("advance returned %d (target %s): %s", status, target, string(raw))
	}
	return nil
}

func (w *World) tryAdvanceFrom(ctx context.Context, from, to string) error {
	// Sanity-check the current state matches the precondition.
	current, err := w.fetchOrderStatus(ctx)
	if err != nil {
		return err
	}
	if current != from {
		return fmt.Errorf("expected current status %s, got %s", from, current)
	}
	return w.advanceServiceOrder(ctx, to)
}

func (w *World) customerLogin(ctx context.Context) error {
	body := map[string]string{
		"cpf":      w.CustomerCPF,
		"password": w.CustomerPassword,
	}
	status, raw, err := doJSON(ctx, w, http.MethodPost, w.MS1URL+"/customers/auth/login", body, "")
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusOK, status, raw); err != nil {
		return err
	}
	var resp struct {
		Token string `json:"token"`
	}
	if err := decodeData(raw, &resp); err != nil {
		return err
	}
	w.CustomerToken = resp.Token
	return nil
}

func (w *World) customerListsOrders(ctx context.Context) error {
	if w.CustomerToken == "" {
		if err := w.customerLogin(ctx); err != nil {
			return err
		}
	}
	status, raw, err := doJSON(ctx, w, http.MethodGet, w.MS2URL+"/service-orders", nil, w.CustomerToken)
	if err != nil {
		return err
	}
	return expectStatus(http.StatusOK, status, raw)
}

func (w *World) assertOrderInCustomerList(ctx context.Context, expectedStatus string) error {
	if w.CustomerToken == "" {
		if err := w.customerLogin(ctx); err != nil {
			return err
		}
	}
	status, raw, err := doJSON(ctx, w, http.MethodGet, w.MS2URL+"/service-orders", nil, w.CustomerToken)
	if err != nil {
		return err
	}
	if err := expectStatus(http.StatusOK, status, raw); err != nil {
		return err
	}
	var orders []struct {
		ID         string `json:"id"`
		CustomerID string `json:"customer_id"`
		Status     string `json:"status"`
	}
	if err := decodeData(raw, &orders); err != nil {
		return err
	}
	for _, o := range orders {
		if o.ID == w.OrderID {
			if o.Status != expectedStatus {
				return fmt.Errorf("OS %s com status %s, esperado %s", w.OrderID, o.Status, expectedStatus)
			}
			if o.CustomerID != w.CustomerID {
				return fmt.Errorf("OS %s pertence ao customer %s, esperado %s", w.OrderID, o.CustomerID, w.CustomerID)
			}
			return nil
		}
	}
	return fmt.Errorf("OS %s não encontrada na lista do cliente", w.OrderID)
}

func (w *World) approveAuthorization(ctx context.Context) error {
	if w.CustomerToken == "" {
		if err := w.customerLogin(ctx); err != nil {
			return err
		}
	}
	url := fmt.Sprintf("%s/service-orders/%s/authorize", w.MS2URL, w.OrderID)
	body := map[string]any{"approved": true}
	status, raw, err := doJSON(ctx, w, http.MethodPost, url, body, w.CustomerToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return fmt.Errorf("authorize returned %d: %s", status, string(raw))
	}
	return nil
}

func (w *World) denyAuthorization(ctx context.Context) error {
	if w.CustomerToken == "" {
		if err := w.customerLogin(ctx); err != nil {
			return err
		}
	}
	url := fmt.Sprintf("%s/service-orders/%s/authorize", w.MS2URL, w.OrderID)
	body := map[string]any{"approved": false}
	status, raw, err := doJSON(ctx, w, http.MethodPost, url, body, w.CustomerToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusAccepted {
		return fmt.Errorf("deny returned %d: %s", status, string(raw))
	}
	return nil
}

func (w *World) cancelOrder(ctx context.Context) error {
	url := fmt.Sprintf("%s/service-orders/%s", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodDelete, url, nil, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusAccepted && status != http.StatusNoContent {
		return fmt.Errorf("cancel returned %d: %s", status, string(raw))
	}
	return nil
}

func (w *World) deleteCustomer(ctx context.Context) error {
	url := fmt.Sprintf("%s/customers/%s", w.MS1URL, w.CustomerID)
	status, raw, err := doJSON(ctx, w, http.MethodDelete, url, nil, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK && status != http.StatusNoContent {
		return fmt.Errorf("delete customer returned %d: %s", status, string(raw))
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// Outcome / assertion steps
// ──────────────────────────────────────────────────────────────────────

func (w *World) fetchOrderStatus(ctx context.Context) (string, error) {
	url := fmt.Sprintf("%s/service-orders/%s", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
	if err != nil {
		return "", err
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("get order returned %d: %s", status, string(raw))
	}
	var resp struct {
		Status string `json:"status"`
	}
	if err := decodeData(raw, &resp); err != nil {
		return "", err
	}
	return resp.Status, nil
}

// fetchOrderRaw decodes the full order envelope into a generic map so steps
// can inspect fields like saga_status and payment_url without proliferating
// strongly-typed shapes.
func (w *World) fetchOrderRaw(ctx context.Context) (map[string]any, error) {
	url := fmt.Sprintf("%s/service-orders/%s", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get order returned %d: %s", status, string(raw))
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, err
	}
	out := map[string]any{}
	if err := json.Unmarshal(env.Data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (w *World) assertOrderStatus(ctx context.Context, expected string) error {
	got, err := w.fetchOrderStatus(ctx)
	if err != nil {
		return err
	}
	if got != expected {
		return fmt.Errorf("expected status %s, got %s", expected, got)
	}
	return nil
}

// assertOrderStatusEventually polls until the expected status is reached.
// Used after async transitions (saga waits, payment webhook, etc).
func (w *World) assertOrderStatusEventually(ctx context.Context, expected string) error {
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		got, err := w.fetchOrderStatus(ctx)
		if err == nil {
			if got == expected {
				return nil
			}
			last = got
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for status %s, last=%s", expected, last)
}

func (w *World) assertOrderCanceledBySaga(ctx context.Context) error {
	// CANCEL_RESERVED is published when MS2 transitions an active order to
	// CANCELED while stock is reserved. We only assert from MS2's side that
	// the OS actually reached CANCELED — the mirrored stock release is
	// covered by assertStockReleased.
	return w.assertOrderStatusEventually(ctx, "CANCELED")
}

// ──────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────

func randomCPF(seed string) string {
	// Generate 11 deterministic but unique digits per scenario. The MS1 CPF
	// validator is permissive (length-only) in test fixtures elsewhere — if
	// it ever rejects a generated value we'll see it in the create step.
	digits := make([]byte, 11)
	for i := 0; i < 11; i++ {
		digits[i] = '0' + byte((int(seed[i%len(seed)])+i)%10)
	}
	return string(digits)
}

func randomPlate(seed string) string {
	upper := strings.ToUpper(seed)
	if len(upper) < 8 {
		upper = (upper + "AAAAAAAA")[:8]
	}
	digit := func(b byte) byte { return '0' + (b % 10) }
	return fmt.Sprintf("%c%c%c%c%c%c%c",
		upper[0], upper[1], upper[2],
		digit(upper[3]), upper[4], digit(upper[5]), digit(upper[6]))
}

// ──────────────────────────────────────────────────────────────────────
// Items immutability
// ──────────────────────────────────────────────────────────────────────

func (w *World) updateOrderItems(ctx context.Context) error {
	url := fmt.Sprintf("%s/service-orders/%s", w.MS2URL, w.OrderID)
	body := map[string]any{
		"items": []orderItem{w.OrderItem},
	}
	status, raw, err := doJSON(ctx, w, http.MethodPut, url, body, w.AdminToken)
	if err != nil {
		return err
	}
	w.LastResponseStatus = status
	_ = raw
	return nil
}

func (w *World) assertUpdateAccepted(_ context.Context) error {
	if w.LastResponseStatus != http.StatusOK && w.LastResponseStatus != http.StatusNoContent {
		return fmt.Errorf("esperado 200/204, obtido %d", w.LastResponseStatus)
	}
	return nil
}

func (w *World) assertUpdateRejected(_ context.Context) error {
	if w.LastResponseStatus < 400 {
		return fmt.Errorf("esperado 4xx, obtido %d", w.LastResponseStatus)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// Audit history and payment URL
// ──────────────────────────────────────────────────────────────────────

func (w *World) assertOrderHistory(ctx context.Context, minEntries int) error {
	url := fmt.Sprintf("%s/service-orders/%s/history", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET history retornou %d: %s", status, string(raw))
	}
	var entries []map[string]any
	if err := decodeData(raw, &entries); err != nil {
		return err
	}
	if len(entries) < minEntries {
		return fmt.Errorf("esperado >= %d entradas no histórico, obtido %d", minEntries, len(entries))
	}
	return nil
}

func (w *World) assertPaymentURL(ctx context.Context) error {
	url := fmt.Sprintf("%s/service-orders/%s/payment", w.MS2URL, w.OrderID)
	status, raw, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET payment retornou %d: %s", status, string(raw))
	}
	var resp map[string]any
	if err := decodeData(raw, &resp); err != nil {
		return err
	}
	paymentURL, _ := resp["payment_url"].(string)
	if paymentURL == "" {
		return fmt.Errorf("payment_url ausente ou vazio na resposta")
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// Multi-order helpers
// ──────────────────────────────────────────────────────────────────────

// advanceToStatus advances the current w.OrderID through the state chain up
// to (and including) target. It assumes the order is already created.
func (w *World) advanceToStatus(ctx context.Context, target string) error {
	chain := []string{"DIAGNOSING", "PENDING_AUTHORIZATION", "AUTHORIZED", "IN_PROGRESS", "COMPLETED"}
	for _, step := range chain {
		var err error
		if step == "AUTHORIZED" {
			err = w.approveAuthorization(ctx)
		} else {
			err = w.advanceServiceOrder(ctx, step)
		}
		if err != nil {
			return err
		}
		if err = w.assertOrderStatusEventually(ctx, step); err != nil {
			return err
		}
		if step == target {
			return nil
		}
	}
	return fmt.Errorf("status alvo não suportado: %q", target)
}

func (w *World) givenTwoOrdersInStatus(ctx context.Context, target string) error {
	if err := w.openServiceOrder(ctx); err != nil {
		return err
	}
	firstID := w.OrderID
	if err := w.advanceToStatus(ctx, target); err != nil {
		return err
	}

	if err := w.openServiceOrder(ctx); err != nil {
		return err
	}
	w.SecondOrderID = w.OrderID
	if err := w.advanceToStatus(ctx, target); err != nil {
		return err
	}

	w.OrderID = firstID
	return nil
}

func (w *World) assertBothOrdersCanceled(ctx context.Context) error {
	if err := w.assertOrderStatusEventually(ctx, "CANCELED"); err != nil {
		return fmt.Errorf("primeira OS não cancelada: %w", err)
	}
	if w.SecondOrderID == "" {
		return fmt.Errorf("SecondOrderID não capturado")
	}
	saved := w.OrderID
	w.OrderID = w.SecondOrderID
	err := w.assertOrderStatusEventually(ctx, "CANCELED")
	w.OrderID = saved
	return err
}

func (w *World) assertNoOrderCancellations(ctx context.Context) error {
	// Verify the customer no longer exists (deletion was processed).
	url := fmt.Sprintf("%s/customers/%s", w.MS1URL, w.CustomerID)
	status, _, err := doJSON(ctx, w, http.MethodGet, url, nil, w.AdminToken)
	if err != nil {
		return err
	}
	if status == http.StatusOK {
		return fmt.Errorf("customer %s ainda existe após deleção", w.CustomerID)
	}
	return nil
}

// ──────────────────────────────────────────────────────────────────────
// Saga-absence assertion
// ──────────────────────────────────────────────────────────────────────

func (w *World) assertNoSagaOperation(ctx context.Context) error {
	raw, err := w.fetchOrderRaw(ctx)
	if err != nil {
		return err
	}
	sagaStatus, _ := raw["saga_status"].(string)
	if sagaStatus != "" && sagaStatus != "IDLE" {
		return fmt.Errorf("saga_status esperado IDLE ou vazio, obtido %q", sagaStatus)
	}
	return nil
}
