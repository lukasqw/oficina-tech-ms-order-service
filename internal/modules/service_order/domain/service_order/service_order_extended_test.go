package service_order

import (
	"testing"
	"time"
)

// --- NewServiceOrder ---

func TestNewServiceOrder_Valid(t *testing.T) {
	so, err := NewServiceOrder("cust-1", "veh-1", "Troca de óleo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.CustomerID() != "cust-1" {
		t.Errorf("want CustomerID 'cust-1', got %s", so.CustomerID())
	}
	if so.VehicleID() != "veh-1" {
		t.Errorf("want VehicleID 'veh-1', got %s", so.VehicleID())
	}
	if so.Status() != StatusReceived {
		t.Errorf("want status RECEIVED, got %s", so.Status())
	}
	if so.SagaStatus() != SagaStatusIdle {
		t.Errorf("want saga IDLE, got %s", so.SagaStatus())
	}
}

func TestNewServiceOrder_EmptyCustomerID(t *testing.T) {
	if _, err := NewServiceOrder("", "veh-1", "desc"); err == nil {
		t.Fatal("expected error for empty customerID")
	}
}

func TestNewServiceOrder_EmptyVehicleID(t *testing.T) {
	if _, err := NewServiceOrder("cust-1", "", "desc"); err == nil {
		t.Fatal("expected error for empty vehicleID")
	}
}

func TestNewServiceOrder_EmptyDescription_OK(t *testing.T) {
	so, err := NewServiceOrder("cust-1", "veh-1", "")
	if err != nil {
		t.Fatalf("empty description should be allowed: %v", err)
	}
	if so.Description() != "" {
		t.Errorf("expected empty description, got %s", so.Description())
	}
}

// --- ReconstructServiceOrder ---

func TestReconstructServiceOrder_Valid(t *testing.T) {
	now := time.Now()
	so, err := ReconstructServiceOrder(
		"id-1", "cust-1", "veh-1", "desc",
		StatusReceived, SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*ServiceOrderItem{},
		nil, now, now, nil,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.ID() != "id-1" {
		t.Errorf("want ID 'id-1', got %s", so.ID())
	}
}

func TestReconstructServiceOrder_EmptyID(t *testing.T) {
	now := time.Now()
	if _, err := ReconstructServiceOrder("", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestReconstructServiceOrder_EmptyCustomerID(t *testing.T) {
	now := time.Now()
	if _, err := ReconstructServiceOrder("id-1", "", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil); err == nil {
		t.Fatal("expected error for empty customerID")
	}
}

func TestReconstructServiceOrder_EmptyVehicleID(t *testing.T) {
	now := time.Now()
	if _, err := ReconstructServiceOrder("id-1", "cust-1", "", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil); err == nil {
		t.Fatal("expected error for empty vehicleID")
	}
}

func TestReconstructServiceOrder_InvalidStatus(t *testing.T) {
	now := time.Now()
	if _, err := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", OrderStatus("INVALID"), SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

// --- UpdateStatus ---

func TestUpdateStatus_ValidTransition(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateStatus(StatusDiagnosing); err != nil {
		t.Fatalf("valid transition RECEIVED→DIAGNOSING failed: %v", err)
	}
	if so.Status() != StatusDiagnosing {
		t.Errorf("want DIAGNOSING, got %s", so.Status())
	}
}

func TestUpdateStatus_InvalidTransition(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateStatus(StatusDelivered); err == nil {
		t.Fatal("expected error for invalid transition RECEIVED→DELIVERED")
	}
}

func TestUpdateStatus_InvalidStatus(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateStatus(OrderStatus("BOGUS")); err == nil {
		t.Fatal("expected error for bogus status")
	}
}

// --- OrderStatus helpers ---

func TestNewOrderStatus_Valid(t *testing.T) {
	for _, s := range []string{
		"RECEIVED", "DIAGNOSING", "PENDING_AUTHORIZATION", "AUTHORIZED",
		"IN_PROGRESS", "COMPLETED", "AWAITING_PAYMENT", "PAID",
		"DELIVERED", "CANCELED", "AUTHORIZATION_DENIED",
	} {
		if _, err := NewOrderStatus(s); err != nil {
			t.Errorf("valid status %q returned error: %v", s, err)
		}
	}
}

func TestNewOrderStatus_Invalid(t *testing.T) {
	if _, err := NewOrderStatus("BOGUS_STATUS"); err == nil {
		t.Fatal("expected error for invalid status string")
	}
}

func TestOrderStatus_String(t *testing.T) {
	if StatusReceived.String() != "RECEIVED" {
		t.Errorf("want 'RECEIVED', got %s", StatusReceived.String())
	}
}

func TestOrderStatus_IsValid(t *testing.T) {
	if !StatusReceived.IsValid() {
		t.Error("RECEIVED should be valid")
	}
	if OrderStatus("INVALID").IsValid() {
		t.Error("'INVALID' should not be valid")
	}
}

func TestOrderStatus_NextStatus_FullChain(t *testing.T) {
	chain := []OrderStatus{
		StatusReceived, StatusDiagnosing, StatusPendingAuthorization,
		StatusAuthorized, StatusInProgress, StatusCompleted,
		StatusAwaitingPayment, StatusPaid,
	}
	for _, s := range chain {
		if _, err := s.NextStatus(); err != nil {
			t.Errorf("NextStatus(%s) returned error: %v", s, err)
		}
	}
}

func TestOrderStatus_NextStatus_FinalStates(t *testing.T) {
	for _, s := range []OrderStatus{StatusDelivered, StatusCanceled, StatusAuthorizationDenied} {
		if _, err := s.NextStatus(); err == nil {
			t.Errorf("expected error for NextStatus(%s)", s)
		}
	}
}

func TestCanTransitionTo_AuthorizationDenied(t *testing.T) {
	if !StatusPendingAuthorization.CanTransitionTo(StatusAuthorizationDenied) {
		t.Error("PENDING_AUTHORIZATION should transition to AUTHORIZATION_DENIED")
	}
	if StatusReceived.CanTransitionTo(StatusAuthorizationDenied) {
		t.Error("RECEIVED should not transition to AUTHORIZATION_DENIED")
	}
}

func TestCanTransitionTo_CancelFromDelivered_NotAllowed(t *testing.T) {
	if StatusDelivered.CanTransitionTo(StatusCanceled) {
		t.Error("DELIVERED should not be cancellable")
	}
}

// --- NewServiceOrderItem ---

func TestNewServiceOrderItem_Valid(t *testing.T) {
	item, err := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Óleo Motor", 2, 5000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Quantity() != 2 {
		t.Errorf("want quantity 2, got %d", item.Quantity())
	}
	if item.Subtotal() != 10000 {
		t.Errorf("want subtotal 10000, got %d", item.Subtotal())
	}
}

func TestNewServiceOrderItem_InvalidType(t *testing.T) {
	if _, err := NewServiceOrderItem("so-1", ItemType("INVALID"), "ref-1", "Name", 1, 100); err == nil {
		t.Fatal("expected error for invalid item type")
	}
}

func TestNewServiceOrderItem_EmptyReferenceID(t *testing.T) {
	if _, err := NewServiceOrderItem("so-1", ItemTypeProduct, "", "Name", 1, 100); err == nil {
		t.Fatal("expected error for empty referenceID")
	}
}

func TestNewServiceOrderItem_EmptyName(t *testing.T) {
	if _, err := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "", 1, 100); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestNewServiceOrderItem_ZeroQuantity(t *testing.T) {
	if _, err := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Name", 0, 100); err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestNewServiceOrderItem_NegativePrice(t *testing.T) {
	if _, err := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Name", 1, -1); err == nil {
		t.Fatal("expected error for negative unit price")
	}
}

func TestItemType_IsValid(t *testing.T) {
	if !ItemTypeProduct.IsValid() {
		t.Error("PRODUCT should be valid")
	}
	if !ItemTypeService.IsValid() {
		t.Error("SERVICE should be valid")
	}
	if ItemType("OTHER").IsValid() {
		t.Error("'OTHER' should not be valid")
	}
}

// --- BuildHistoryMetadata ---

func TestBuildHistoryMetadata_NoChanges(t *testing.T) {
	now := time.Now()
	old, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	new_, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	meta := BuildHistoryMetadata(old, new_)
	if len(meta) != 0 {
		t.Errorf("expected empty metadata for no changes, got %v", meta)
	}
}

func TestBuildHistoryMetadata_StatusChange(t *testing.T) {
	now := time.Now()
	old, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	new_, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusDiagnosing, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	meta := BuildHistoryMetadata(old, new_)
	if _, ok := meta["status"]; !ok {
		t.Errorf("expected 'status' in metadata, got %v", meta)
	}
}

func TestBuildHistoryMetadataWithItems_NoItemChange(t *testing.T) {
	now := time.Now()
	old, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, []*ServiceOrderItem{}, nil, now, now, nil)
	new_, _ := ReconstructServiceOrder("id-1", "cust-1", "veh-1", "desc", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, []*ServiceOrderItem{}, nil, now, now, nil)
	meta := BuildHistoryMetadataWithItems(old, new_)
	if _, ok := meta["items_changed"]; ok {
		t.Error("should not flag items_changed when items are identical")
	}
}

// --- NewHistory ---

func TestNewHistory_Valid(t *testing.T) {
	h, err := NewHistory("so-1", map[string]any{"status": "changed"}, StatusDiagnosing)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h.ServiceOrderID() != "so-1" {
		t.Errorf("want ServiceOrderID 'so-1', got %s", h.ServiceOrderID())
	}
	if h.Status() != StatusDiagnosing {
		t.Errorf("want status DIAGNOSING, got %s", h.Status())
	}
}

func TestNewHistory_EmptyServiceOrderID(t *testing.T) {
	if _, err := NewHistory("", nil, StatusReceived); err == nil {
		t.Fatal("expected error for empty serviceOrderID")
	}
}

func TestReconstructHistory_Getters(t *testing.T) {
	now := time.Now()
	h := ReconstructHistory("hist-1", "so-1", map[string]any{"key": "val"}, StatusReceived, now)
	if h.ID() != "hist-1" {
		t.Errorf("want ID 'hist-1', got %s", h.ID())
	}
	if h.ServiceOrderID() != "so-1" {
		t.Errorf("want ServiceOrderID 'so-1', got %s", h.ServiceOrderID())
	}
}

func TestHistory_SetID_Valid(t *testing.T) {
	h, _ := NewHistory("so-1", nil, StatusReceived)
	if err := h.SetID("new-id"); err != nil {
		t.Fatalf("SetID() error: %v", err)
	}
	if h.ID() != "new-id" {
		t.Errorf("want 'new-id', got %s", h.ID())
	}
}

func TestHistory_SetID_Empty(t *testing.T) {
	h, _ := NewHistory("so-1", nil, StatusReceived)
	if err := h.SetID(""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}
