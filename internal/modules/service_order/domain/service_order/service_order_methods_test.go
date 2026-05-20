package service_order

import (
	"testing"
	"time"

	"oficina-tech/internal/shared/dto"
)

// --- ServiceOrder setters ---

func TestUpdateCustomer_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateCustomer("cust-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.CustomerID() != "cust-2" {
		t.Errorf("want 'cust-2', got %s", so.CustomerID())
	}
}

func TestUpdateCustomer_Empty(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateCustomer(""); err == nil {
		t.Fatal("expected error for empty customerID")
	}
}

func TestUpdateVehicle_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateVehicle("veh-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.VehicleID() != "veh-2" {
		t.Errorf("want 'veh-2', got %s", so.VehicleID())
	}
}

func TestUpdateVehicle_Empty(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateVehicle(""); err == nil {
		t.Fatal("expected error for empty vehicleID")
	}
}

func TestUpdateDescription(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "original")
	so.UpdateDescription("updated")
	if so.Description() != "updated" {
		t.Errorf("want 'updated', got %s", so.Description())
	}
}

func TestSetID_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.SetID("new-id"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.ID() != "new-id" {
		t.Errorf("want 'new-id', got %s", so.ID())
	}
}

func TestSetID_Empty(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.SetID(""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

// --- MarkAsDeleted, IsDeleted, IsClosed, CanModifyItems ---

func TestMarkAsDeleted(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if so.IsDeleted() {
		t.Fatal("new order should not be deleted")
	}
	so.MarkAsDeleted()
	if !so.IsDeleted() {
		t.Fatal("order should be deleted after MarkAsDeleted")
	}
	if so.DeletedAt() == nil {
		t.Fatal("DeletedAt should be set")
	}
}

func TestIsClosed_FinalStatuses(t *testing.T) {
	for _, status := range []OrderStatus{StatusPaid, StatusDelivered, StatusCanceled, StatusAuthorizationDenied} {
		now := time.Now()
		so, _ := ReconstructServiceOrder("id", "c", "v", "d", status, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
		if !so.IsClosed() {
			t.Errorf("expected IsClosed for status %s", status)
		}
	}
}

func TestIsClosed_NonFinalStatuses(t *testing.T) {
	for _, status := range []OrderStatus{StatusReceived, StatusDiagnosing, StatusInProgress, StatusCompleted} {
		now := time.Now()
		so, _ := ReconstructServiceOrder("id", "c", "v", "d", status, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
		if so.IsClosed() {
			t.Errorf("expected not IsClosed for status %s", status)
		}
	}
}

func TestCanModifyItems_Allowed(t *testing.T) {
	now := time.Now()
	for _, s := range []OrderStatus{StatusReceived, StatusDiagnosing} {
		so, _ := ReconstructServiceOrder("id", "c", "v", "d", s, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
		if !so.CanModifyItems() {
			t.Errorf("expected CanModifyItems=true for %s", s)
		}
	}
}

func TestCanModifyItems_Forbidden(t *testing.T) {
	now := time.Now()
	for _, s := range []OrderStatus{StatusPendingAuthorization, StatusAuthorized, StatusInProgress} {
		so, _ := ReconstructServiceOrder("id", "c", "v", "d", s, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
		if so.CanModifyItems() {
			t.Errorf("expected CanModifyItems=false for %s", s)
		}
	}
}

// --- AddItem ---

func TestAddItem_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 1000)
	if err := so.AddItem(item); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(so.Items()) != 1 {
		t.Errorf("expected 1 item, got %d", len(so.Items()))
	}
}

func TestAddItem_Duplicate_Merges(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 1000)
	_ = item1.SetID("item-1")
	item2, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 3, 1000)
	_ = item2.SetID("item-2")

	_ = so.AddItem(item1)
	_ = so.AddItem(item2)

	if len(so.Items()) != 1 {
		t.Errorf("expected 1 merged item, got %d", len(so.Items()))
	}
	if so.Items()[0].Quantity() != 5 {
		t.Errorf("expected merged quantity 5, got %d", so.Items()[0].Quantity())
	}
}

func TestAddItem_Nil_Error(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.AddItem(nil); err == nil {
		t.Fatal("expected error for nil item")
	}
}

func TestAddItem_ClosedOrder_Error(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusCanceled, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 1000)
	if err := so.AddItem(item); err == nil {
		t.Fatal("expected error adding item to closed order")
	}
}

func TestAddItem_AfterPending_Error(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusPendingAuthorization, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 1000)
	if err := so.AddItem(item); err == nil {
		t.Fatal("expected error adding item after PENDING_AUTHORIZATION")
	}
}

// --- RemoveItem ---

func TestRemoveItem_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 1000)
	_ = item.SetID("item-1")
	_ = so.AddItem(item)
	if err := so.RemoveItem("item-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !so.Items()[0].IsDeleted() {
		t.Error("item should be marked as deleted")
	}
}

func TestRemoveItem_NotFound(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.RemoveItem("no-such-id"); err == nil {
		t.Fatal("expected error for missing item")
	}
}

func TestRemoveItem_EmptyID(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.RemoveItem(""); err == nil {
		t.Fatal("expected error for empty itemID")
	}
}

// --- UpdateItemQuantity ---

func TestUpdateItemQuantity_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 1000)
	_ = item.SetID("item-1")
	_ = so.AddItem(item)
	if err := so.UpdateItemQuantity("item-1", 5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Items()[0].Quantity() != 5 {
		t.Errorf("expected quantity 5, got %d", so.Items()[0].Quantity())
	}
}

func TestUpdateItemQuantity_NotFound(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateItemQuantity("no-such", 5); err == nil {
		t.Fatal("expected error for missing item")
	}
}

func TestUpdateItemQuantity_EmptyID(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.UpdateItemQuantity("", 5); err == nil {
		t.Fatal("expected error for empty itemID")
	}
}

// --- GetProductItems, TotalAmount, ClearItems ---

func TestGetProductItems(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	p1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-p1", "Product 1", 2, 500)
	_ = p1.SetID("p-1")
	s1, _ := NewServiceOrderItem("", ItemTypeService, "ref-s1", "Service 1", 1, 1000)
	_ = s1.SetID("s-1")
	_ = so.AddItem(p1)
	_ = so.AddItem(s1)

	products := so.GetProductItems()
	if len(products) != 1 {
		t.Errorf("expected 1 product item, got %d", len(products))
	}
	if products[0].ReferenceID() != "ref-p1" {
		t.Errorf("wrong product item: %s", products[0].ReferenceID())
	}
}

func TestTotalAmount(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	p1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 500)
	_ = p1.SetID("p-1")
	p2, _ := NewServiceOrderItem("", ItemTypeService, "ref-2", "Item B", 1, 1000)
	_ = p2.SetID("s-1")
	_ = so.AddItem(p1)
	_ = so.AddItem(p2)

	if total := so.TotalAmount(); total != 2000 {
		t.Errorf("expected TotalAmount 2000, got %d", total)
	}
}

func TestTotalAmount_ExcludesDeleted(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	p1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 500)
	_ = p1.SetID("p-1")
	p2, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-2", "Item B", 1, 1000)
	_ = p2.SetID("p-2")
	_ = so.AddItem(p1)
	_ = so.AddItem(p2)
	_ = so.RemoveItem("p-1")

	if total := so.TotalAmount(); total != 1000 {
		t.Errorf("expected TotalAmount 1000 after soft delete, got %d", total)
	}
}

func TestClearItems(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	p1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 500)
	_ = p1.SetID("p-1")
	_ = so.AddItem(p1)
	so.ClearItems()
	if len(so.Items()) != 0 {
		t.Errorf("expected 0 items after ClearItems, got %d", len(so.Items()))
	}
}

// --- Saga methods ---

func TestStartSaga_Valid(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.StartSaga("saga-1", StatusPendingAuthorization, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.SagaStatus() != SagaStatusAwaitingInventory {
		t.Errorf("want AWAITING_INVENTORY, got %s", so.SagaStatus())
	}
	if so.CurrentSagaID() == nil || *so.CurrentSagaID() != "saga-1" {
		t.Error("want currentSagaID 'saga-1'")
	}
}

func TestStartSaga_EmptySagaID(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.StartSaga("", StatusPendingAuthorization, nil); err == nil {
		t.Fatal("expected error for empty sagaID")
	}
}

func TestStartSaga_InvalidStatus(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.StartSaga("saga-1", OrderStatus("INVALID"), nil); err == nil {
		t.Fatal("expected error for invalid status")
	}
}

func TestCompleteSaga(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	_ = so.StartSaga("saga-1", StatusPendingAuthorization, nil)
	so.CompleteSaga()
	if so.SagaStatus() != SagaStatusIdle {
		t.Errorf("want IDLE after CompleteSaga, got %s", so.SagaStatus())
	}
	if so.CurrentSagaID() != nil {
		t.Error("currentSagaID should be nil after CompleteSaga")
	}
}

func TestFailSaga(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	_ = so.StartSaga("saga-1", StatusPendingAuthorization, nil)
	so.FailSaga()
	if so.SagaStatus() != SagaStatusFailed {
		t.Errorf("want FAILED after FailSaga, got %s", so.SagaStatus())
	}
}

func TestCanProcessSaga_True(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	_ = so.StartSaga("saga-1", StatusPendingAuthorization, nil)
	if !so.CanProcessSaga("saga-1") {
		t.Error("expected CanProcessSaga=true")
	}
}

func TestCanProcessSaga_WrongID(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	_ = so.StartSaga("saga-1", StatusPendingAuthorization, nil)
	if so.CanProcessSaga("other-saga") {
		t.Error("expected CanProcessSaga=false for wrong sagaID")
	}
}

func TestCanProcessSaga_NotInSaga(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if so.CanProcessSaga("any") {
		t.Error("expected CanProcessSaga=false when not in saga")
	}
}

// --- Payment methods ---

func TestAwaitPayment_Valid(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusCompleted, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.AwaitPayment("pref-123", "http://pay.me"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Status() != StatusAwaitingPayment {
		t.Errorf("want AWAITING_PAYMENT, got %s", so.Status())
	}
	if so.SagaStatus() != SagaStatusAwaitingPayment {
		t.Errorf("want AWAITING_PAYMENT saga, got %s", so.SagaStatus())
	}
}

func TestAwaitPayment_EmptyPreference(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusCompleted, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.AwaitPayment("", "http://pay.me"); err == nil {
		t.Fatal("expected error for empty preferenceID")
	}
}

func TestAwaitPayment_EmptyURL(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusCompleted, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.AwaitPayment("pref-123", ""); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestConfirmPayment_Valid(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusAwaitingPayment, SagaStatusAwaitingPayment, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.ConfirmPayment("pay-123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Status() != StatusPaid {
		t.Errorf("want PAID, got %s", so.Status())
	}
}

func TestConfirmPayment_EmptyID(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusAwaitingPayment, SagaStatusAwaitingPayment, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.ConfirmPayment(""); err == nil {
		t.Fatal("expected error for empty paymentID")
	}
}

func TestRejectPayment_Valid(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusAwaitingPayment, SagaStatusAwaitingPayment, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.RejectPayment(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if so.Status() != StatusPaymentRejected {
		t.Errorf("want PAYMENT_REJECTED after RejectPayment, got %s", so.Status())
	}
}

// --- ServiceOrderItem methods ---

func TestServiceOrderItem_SetID_Valid(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetID("item-id"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ID() != "item-id" {
		t.Errorf("want 'item-id', got %s", item.ID())
	}
}

func TestServiceOrderItem_SetID_Empty(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetID(""); err == nil {
		t.Fatal("expected error for empty ID")
	}
}

func TestServiceOrderItem_SetServiceOrderID_Valid(t *testing.T) {
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetServiceOrderID("so-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.ServiceOrderID() != "so-1" {
		t.Errorf("want 'so-1', got %s", item.ServiceOrderID())
	}
}

func TestServiceOrderItem_SetServiceOrderID_Empty(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetServiceOrderID(""); err == nil {
		t.Fatal("expected error for empty serviceOrderID")
	}
}

func TestServiceOrderItem_SetHistoryID_Valid(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetHistoryID("hist-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.HistoryID() == nil || *item.HistoryID() != "hist-1" {
		t.Error("want historyID 'hist-1'")
	}
}

func TestServiceOrderItem_SetHistoryID_Empty(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.SetHistoryID(""); err == nil {
		t.Fatal("expected error for empty historyID")
	}
}

func TestServiceOrderItem_UpdateQuantity_Valid(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.UpdateQuantity(5); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if item.Quantity() != 5 {
		t.Errorf("expected quantity 5, got %d", item.Quantity())
	}
}

func TestServiceOrderItem_UpdateQuantity_Zero(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if err := item.UpdateQuantity(0); err == nil {
		t.Fatal("expected error for zero quantity")
	}
}

func TestServiceOrderItem_MarkAsDeleted(t *testing.T) {
	item, _ := NewServiceOrderItem("so-1", ItemTypeProduct, "ref-1", "Item", 1, 100)
	if item.IsDeleted() {
		t.Fatal("new item should not be deleted")
	}
	item.MarkAsDeleted()
	if !item.IsDeleted() {
		t.Fatal("item should be deleted after MarkAsDeleted")
	}
	if item.DeletedAt() == nil {
		t.Fatal("DeletedAt should be set")
	}
}

func TestReconstructServiceOrderItem_Getters(t *testing.T) {
	now := time.Now()
	histID := "hist-1"
	item := ReconstructServiceOrderItem(
		"item-id", "so-id", &histID,
		ItemTypeService, "ref-1", "Name", 3, 500,
		now, now, nil,
	)
	if item.ID() != "item-id" {
		t.Errorf("wrong ID: %s", item.ID())
	}
	if item.ServiceOrderID() != "so-id" {
		t.Errorf("wrong ServiceOrderID: %s", item.ServiceOrderID())
	}
	if item.ItemType() != ItemTypeService {
		t.Errorf("wrong ItemType: %s", item.ItemType())
	}
	if item.ReferenceID() != "ref-1" {
		t.Errorf("wrong ReferenceID: %s", item.ReferenceID())
	}
	if item.Name() != "Name" {
		t.Errorf("wrong Name: %s", item.Name())
	}
	if item.Quantity() != 3 {
		t.Errorf("wrong Quantity: %d", item.Quantity())
	}
	if item.UnitPrice() != 500 {
		t.Errorf("wrong UnitPrice: %d", item.UnitPrice())
	}
	if item.Subtotal() != 1500 {
		t.Errorf("wrong Subtotal: %d", item.Subtotal())
	}
	if item.HistoryID() == nil || *item.HistoryID() != "hist-1" {
		t.Error("wrong HistoryID")
	}
	if item.IsDeleted() {
		t.Error("item should not be deleted")
	}
	if item.UpdatedAt().IsZero() {
		t.Error("UpdatedAt should be set")
	}
}

// --- ServiceOrder CreatedAt / UpdatedAt ---

func TestServiceOrder_CreatedAt_UpdatedAt(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if so.CreatedAt().IsZero() {
		t.Error("CreatedAt() should not be zero")
	}
	if so.UpdatedAt().IsZero() {
		t.Error("UpdatedAt() should not be zero")
	}
}

// --- ServiceOrderItem CreatedAt ---

func TestServiceOrderItem_CreatedAt(t *testing.T) {
	now := time.Now()
	item := ReconstructServiceOrderItem("item-1", "so-1", nil, ItemTypeProduct, "ref-1", "Name", 1, 100, now, now, nil)
	if item.CreatedAt().IsZero() {
		t.Error("CreatedAt() should not be zero")
	}
}

// --- History.Metadata / History.CreatedAt ---

func TestHistory_Metadata(t *testing.T) {
	meta := map[string]any{"key": "value"}
	h, _ := NewHistory("so-1", meta, StatusReceived)
	if h.Metadata()["key"] != "value" {
		t.Error("Metadata() should return stored map")
	}
}

func TestHistory_CreatedAt(t *testing.T) {
	h, _ := NewHistory("so-1", nil, StatusReceived)
	if h.CreatedAt().IsZero() {
		t.Error("CreatedAt() should not be zero")
	}
}

// --- BuildStatusOnlyMetadata ---

func TestBuildStatusOnlyMetadata(t *testing.T) {
	meta := BuildStatusOnlyMetadata(StatusReceived, StatusDiagnosing)
	entry, ok := meta["status"]
	if !ok {
		t.Fatal("expected 'status' key in metadata")
	}
	m, ok := entry.(map[string]string)
	if !ok {
		t.Fatal("status entry should be map[string]string")
	}
	if m["old"] != "RECEIVED" {
		t.Errorf("want old=RECEIVED, got %s", m["old"])
	}
	if m["new"] != "DIAGNOSING" {
		t.Errorf("want new=DIAGNOSING, got %s", m["new"])
	}
}

// --- CaptureOrderState ---

func TestCaptureOrderState(t *testing.T) {
	now := time.Now()
	item, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item", 1, 100)
	_ = item.SetID("item-1")
	so, _ := ReconstructServiceOrder(
		"id-1", "cust-1", "veh-1", "desc",
		StatusReceived, SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*ServiceOrderItem{item}, nil, now, now, nil,
	)

	snapshot := CaptureOrderState(so)

	if snapshot.ID() != so.ID() {
		t.Errorf("snapshot ID mismatch: want %s, got %s", so.ID(), snapshot.ID())
	}
	if snapshot.Status() != so.Status() {
		t.Errorf("snapshot Status mismatch")
	}
	if len(snapshot.Items()) != len(so.Items()) {
		t.Errorf("snapshot items count mismatch: want %d, got %d", len(so.Items()), len(snapshot.Items()))
	}
}

// --- CancelAfterRefund ---

func TestCancelAfterRefund_FromPaid(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder("id", "cust-1", "veh-1", "d", StatusPaid, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if err := so.CancelAfterRefund(); err != nil {
		t.Fatalf("CancelAfterRefund() error = %v", err)
	}
	if so.Status() != StatusCanceled {
		t.Errorf("expected CANCELED, got %s", so.Status())
	}
}

func TestCancelAfterRefund_InvalidStatus(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	if err := so.CancelAfterRefund(); err != ErrInvalidStatusTransition {
		t.Fatalf("expected ErrInvalidStatusTransition, got %v", err)
	}
}

// --- SetCustomerSnapshot / CustomerEmail / CustomerName ---

func TestSetCustomerSnapshot(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	so.SetCustomerSnapshot("test@example.com", "John Doe")
	if so.CustomerEmail() != "test@example.com" {
		t.Errorf("expected test@example.com, got %s", so.CustomerEmail())
	}
	if so.CustomerName() != "John Doe" {
		t.Errorf("expected John Doe, got %s", so.CustomerName())
	}
}

// --- BuildHistoryMetadataWithItems / hasItemsChanged / filterActiveItems / buildItemSignature ---

func TestBuildHistoryMetadataWithItems_ItemsAdded(t *testing.T) {
	old, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	newOrder, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item, _ := NewServiceOrderItem("", ItemTypeService, "ref-1", "Oil Change", 1, 10000)
	_ = item.SetID("item-1")
	_ = newOrder.AddItem(item)
	changes := BuildHistoryMetadataWithItems(old, newOrder)
	if _, ok := changes["items_changed"]; !ok {
		t.Fatalf("expected items_changed in metadata when items differ")
	}
}

func TestBuildHistoryMetadataWithItems_NoChange(t *testing.T) {
	so, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	changes := BuildHistoryMetadataWithItems(so, so)
	if _, ok := changes["items_changed"]; ok {
		t.Fatalf("unexpected items_changed when nothing changed")
	}
}

func TestBuildHistoryMetadataWithItems_SameContent(t *testing.T) {
	old, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 50)
	_ = item1.SetID("item-1")
	_ = old.AddItem(item1)

	newOrder, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item2, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 50)
	_ = item2.SetID("item-2")
	_ = newOrder.AddItem(item2)

	changes := BuildHistoryMetadataWithItems(old, newOrder)
	if _, ok := changes["items_changed"]; ok {
		t.Fatalf("unexpected items_changed when item content is the same")
	}
}

func TestBuildHistoryMetadataWithItems_DifferentCount(t *testing.T) {
	old, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 2, 50)
	_ = item1.SetID("item-1")
	_ = old.AddItem(item1)

	newOrder, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	changes := BuildHistoryMetadataWithItems(old, newOrder)
	if _, ok := changes["items_changed"]; !ok {
		t.Fatalf("expected items_changed when item count differs")
	}
}

func TestBuildHistoryMetadataWithItems_DeletedItemsExcluded(t *testing.T) {
	order, _ := NewServiceOrder("cust-1", "veh-1", "desc")
	item1, _ := NewServiceOrderItem("", ItemTypeProduct, "ref-1", "Item A", 1, 50)
	_ = item1.SetID("item-1")
	_ = order.AddItem(item1)
	_ = order.RemoveItem("item-1")

	changes := BuildHistoryMetadataWithItems(order, order)
	if _, ok := changes["items_changed"]; ok {
		t.Fatalf("deleted items should be excluded from comparison")
	}
}

func TestCaptureOrderState_NoItems(t *testing.T) {
	now := time.Now()
	so, _ := ReconstructServiceOrder(
		"id-2", "cust-2", "veh-2", "desc",
		StatusDiagnosing, SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*ServiceOrderItem{}, nil, now, now, nil,
	)
	snapshot := CaptureOrderState(so)
	if len(snapshot.Items()) != 0 {
		t.Errorf("expected 0 items in snapshot, got %d", len(snapshot.Items()))
	}
}

// --- DetermineInventoryOperation ---

func TestDetermineInventoryOperation_ToPendingAuthorization(t *testing.T) {
	op := DetermineInventoryOperation(StatusReceived, StatusPendingAuthorization)
	if op.Type != dto.StockOpReserve {
		t.Errorf("want StockOpReserve, got %s", op.Type)
	}
}

func TestDetermineInventoryOperation_ToCompleted(t *testing.T) {
	op := DetermineInventoryOperation(StatusInProgress, StatusCompleted)
	if op.Type != dto.StockOpReservedDecrease {
		t.Errorf("want StockOpReservedDecrease, got %s", op.Type)
	}
}

func TestDetermineInventoryOperation_CanceledFromReserved(t *testing.T) {
	for _, old := range []OrderStatus{StatusPendingAuthorization, StatusInProgress, StatusAuthorized, StatusDiagnosing, StatusReceived} {
		op := DetermineInventoryOperation(old, StatusCanceled)
		if op.Type != dto.StockOpCancelReserved {
			t.Errorf("from %s: want StockOpCancelReserved, got %s", old, op.Type)
		}
	}
}

func TestDetermineInventoryOperation_CanceledFromConfirmed(t *testing.T) {
	for _, old := range []OrderStatus{StatusCompleted, StatusAwaitingPayment, StatusPaid} {
		op := DetermineInventoryOperation(old, StatusCanceled)
		if op.Type != dto.StockOpCancelConfirmed {
			t.Errorf("from %s: want StockOpCancelConfirmed, got %s", old, op.Type)
		}
	}
}

func TestDetermineInventoryOperation_AuthorizationDenied(t *testing.T) {
	op := DetermineInventoryOperation(StatusPendingAuthorization, StatusAuthorizationDenied)
	if op.Type != dto.StockOpCancelReserved {
		t.Errorf("want StockOpCancelReserved, got %s", op.Type)
	}
}

func TestDetermineInventoryOperation_NoOp(t *testing.T) {
	op := DetermineInventoryOperation(StatusPendingAuthorization, StatusAuthorized)
	if op.Type != InventoryOpNone {
		t.Errorf("want InventoryOpNone, got %s", op.Type)
	}
}

// --- test_helpers.go coverage ---

func TestNewTestServiceOrder(t *testing.T) {
	so := NewTestServiceOrder("cust-1", "veh-1")
	if so.ID() != "test-order-id" {
		t.Errorf("want 'test-order-id', got %s", so.ID())
	}
	if so.CustomerID() != "cust-1" {
		t.Errorf("want 'cust-1', got %s", so.CustomerID())
	}
}

func TestNewTestServiceOrderWithID(t *testing.T) {
	so := NewTestServiceOrderWithID("my-id", "cust-1", "veh-1")
	if so.ID() != "my-id" {
		t.Errorf("want 'my-id', got %s", so.ID())
	}
}

func TestNewTestServiceOrderWithStatus_NonReceived(t *testing.T) {
	so := NewTestServiceOrderWithStatus("cust-1", "veh-1", StatusDiagnosing)
	if so.Status() != StatusDiagnosing {
		t.Errorf("want DIAGNOSING, got %s", so.Status())
	}
}

func TestNewTestServiceOrderWithStatus_Received(t *testing.T) {
	so := NewTestServiceOrderWithStatus("cust-1", "veh-1", StatusReceived)
	if so.Status() != StatusReceived {
		t.Errorf("want RECEIVED, got %s", so.Status())
	}
}

func TestNewTestServiceOrderItem(t *testing.T) {
	item := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 2)
	if item.ID() != "test-item-id" {
		t.Errorf("want 'test-item-id', got %s", item.ID())
	}
	if item.Quantity() != 2 {
		t.Errorf("want 2, got %d", item.Quantity())
	}
}

func TestNewTestServiceOrderItemWithPrice(t *testing.T) {
	item := NewTestServiceOrderItemWithPrice(ItemTypeService, "ref-1", 1, 5000)
	if item.UnitPrice() != 5000 {
		t.Errorf("want 5000, got %d", item.UnitPrice())
	}
}

func TestNewTestServiceOrderItemWithName(t *testing.T) {
	item := NewTestServiceOrderItemWithName(ItemTypeProduct, "ref-1", "Custom Name", 1, 1000)
	if item.Name() != "Custom Name" {
		t.Errorf("want 'Custom Name', got %s", item.Name())
	}
}

func TestNewTestServiceOrderItemFull(t *testing.T) {
	item := NewTestServiceOrderItemFull("item-id", "so-id", ItemTypeProduct, "ref-1", "Name", 3, 2000)
	if item.ID() != "item-id" {
		t.Errorf("want 'item-id', got %s", item.ID())
	}
	if item.ServiceOrderID() != "so-id" {
		t.Errorf("want 'so-id', got %s", item.ServiceOrderID())
	}
}

func TestNewTestHistory(t *testing.T) {
	h := NewTestHistory("so-1", StatusDiagnosing)
	if h.ID() != "test-history-id" {
		t.Errorf("want 'test-history-id', got %s", h.ID())
	}
	if h.Status() != StatusDiagnosing {
		t.Errorf("want DIAGNOSING, got %s", h.Status())
	}
}

func TestNewTestHistoryWithMetadata(t *testing.T) {
	meta := map[string]any{"key": "val"}
	h := NewTestHistoryWithMetadata("so-1", StatusReceived, meta)
	if h.Metadata()["key"] != "val" {
		t.Error("metadata mismatch")
	}
}

func TestNewTestCustomerDTO(t *testing.T) {
	c := NewTestCustomerDTO()
	if c.ID != "customer-123" {
		t.Errorf("want 'customer-123', got %s", c.ID)
	}
}

func TestNewTestCustomerDTOWithID(t *testing.T) {
	c := NewTestCustomerDTOWithID("my-cust")
	if c.ID != "my-cust" {
		t.Errorf("want 'my-cust', got %s", c.ID)
	}
}

func TestNewTestVehicleDTO(t *testing.T) {
	v := NewTestVehicleDTO("cust-1")
	if v.CustomerID != "cust-1" {
		t.Errorf("want 'cust-1', got %s", v.CustomerID)
	}
}

func TestNewTestVehicleDTOWithID(t *testing.T) {
	v := NewTestVehicleDTOWithID("veh-id", "cust-id")
	if v.ID != "veh-id" {
		t.Errorf("want 'veh-id', got %s", v.ID)
	}
}

func TestNewTestProductDTO(t *testing.T) {
	p := NewTestProductDTO()
	if p.ID != "product-789" {
		t.Errorf("want 'product-789', got %s", p.ID)
	}
}

func TestNewTestProductDTOWithID(t *testing.T) {
	p := NewTestProductDTOWithID("prod-1")
	if p.ID != "prod-1" {
		t.Errorf("want 'prod-1', got %s", p.ID)
	}
}

func TestNewTestProductDTOWithPrice(t *testing.T) {
	p := NewTestProductDTOWithPrice("prod-1", 9999)
	if p.Price != 9999 {
		t.Errorf("want 9999, got %d", p.Price)
	}
}

func TestNewTestServiceDTO(t *testing.T) {
	s := NewTestServiceDTO()
	if s.ID != "service-123" {
		t.Errorf("want 'service-123', got %s", s.ID)
	}
}

func TestNewTestServiceDTOWithID(t *testing.T) {
	s := NewTestServiceDTOWithID("svc-1")
	if s.ID != "svc-1" {
		t.Errorf("want 'svc-1', got %s", s.ID)
	}
}

func TestNewTestServiceDTOWithPrice(t *testing.T) {
	s := NewTestServiceDTOWithPrice("svc-1", 8888)
	if s.Price != 8888 {
		t.Errorf("want 8888, got %d", s.Price)
	}
}

func TestFixedTime(t *testing.T) {
	ft := FixedTime()
	if ft.IsZero() {
		t.Error("FixedTime() should not be zero")
	}
}

func TestFixedTimePtr(t *testing.T) {
	ftp := FixedTimePtr()
	if ftp == nil {
		t.Error("FixedTimePtr() should not be nil")
	}
}

func TestTimeAfter(t *testing.T) {
	ta := TimeAfter(time.Hour)
	if !ta.After(FixedTime()) {
		t.Error("TimeAfter(1h) should be after FixedTime()")
	}
}

func TestTimeAfterPtr(t *testing.T) {
	tap := TimeAfterPtr(time.Hour)
	if tap == nil {
		t.Error("TimeAfterPtr() should not be nil")
	}
}

func TestAssertServiceOrderEqual_Match(t *testing.T) {
	so1 := NewTestServiceOrderWithID("id", "cust", "veh")
	so2 := NewTestServiceOrderWithID("id", "cust", "veh")
	if !AssertServiceOrderEqual(so1, so2) {
		t.Error("expected equal service orders")
	}
}

func TestAssertServiceOrderEqual_IDMismatch(t *testing.T) {
	so1 := NewTestServiceOrderWithID("id-1", "cust", "veh")
	so2 := NewTestServiceOrderWithID("id-2", "cust", "veh")
	if AssertServiceOrderEqual(so1, so2) {
		t.Error("expected mismatch for different IDs")
	}
}

func TestAssertServiceOrderEqual_CustomerMismatch(t *testing.T) {
	so1 := NewTestServiceOrderWithID("id", "cust-1", "veh")
	so2 := NewTestServiceOrderWithID("id", "cust-2", "veh")
	if AssertServiceOrderEqual(so1, so2) {
		t.Error("expected mismatch for different customer IDs")
	}
}

func TestAssertServiceOrderEqual_VehicleMismatch(t *testing.T) {
	so1 := NewTestServiceOrderWithID("id", "cust", "veh-1")
	so2 := NewTestServiceOrderWithID("id", "cust", "veh-2")
	if AssertServiceOrderEqual(so1, so2) {
		t.Error("expected mismatch for different vehicle IDs")
	}
}

func TestAssertServiceOrderEqual_StatusMismatch(t *testing.T) {
	now := time.Now()
	so1, _ := ReconstructServiceOrder("id", "cust", "veh", "", StatusReceived, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	so2, _ := ReconstructServiceOrder("id", "cust", "veh", "", StatusDiagnosing, SagaStatusIdle, nil, nil, nil, nil, nil, nil, nil, nil, now, now, nil)
	if AssertServiceOrderEqual(so1, so2) {
		t.Error("expected mismatch for different statuses")
	}
}

func TestAssertServiceOrderItemEqual_Match(t *testing.T) {
	i1 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 2)
	i2 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 2)
	if !AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected equal items")
	}
}

func TestAssertServiceOrderItemEqual_TypeMismatch(t *testing.T) {
	i1 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 1)
	i2 := NewTestServiceOrderItem(ItemTypeService, "ref-1", 1)
	if AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected mismatch for different types")
	}
}

func TestAssertServiceOrderItemEqual_RefMismatch(t *testing.T) {
	i1 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 1)
	i2 := NewTestServiceOrderItem(ItemTypeProduct, "ref-2", 1)
	if AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected mismatch for different reference IDs")
	}
}

func TestAssertServiceOrderItemEqual_NameMismatch(t *testing.T) {
	i1 := NewTestServiceOrderItemWithName(ItemTypeProduct, "ref-1", "Name A", 1, 100)
	i2 := NewTestServiceOrderItemWithName(ItemTypeProduct, "ref-1", "Name B", 1, 100)
	if AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected mismatch for different names")
	}
}

func TestAssertServiceOrderItemEqual_QuantityMismatch(t *testing.T) {
	i1 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 1)
	i2 := NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 2)
	if AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected mismatch for different quantities")
	}
}

func TestAssertServiceOrderItemEqual_PriceMismatch(t *testing.T) {
	i1 := NewTestServiceOrderItemWithPrice(ItemTypeProduct, "ref-1", 1, 100)
	i2 := NewTestServiceOrderItemWithPrice(ItemTypeProduct, "ref-1", 1, 200)
	if AssertServiceOrderItemEqual(i1, i2) {
		t.Error("expected mismatch for different prices")
	}
}

func TestAssertHistoryEqual_Match(t *testing.T) {
	h1 := NewTestHistory("so-1", StatusReceived)
	h2 := NewTestHistory("so-1", StatusReceived)
	if !AssertHistoryEqual(h1, h2) {
		t.Error("expected equal histories")
	}
}

func TestAssertHistoryEqual_ServiceOrderIDMismatch(t *testing.T) {
	h1 := NewTestHistory("so-1", StatusReceived)
	h2 := NewTestHistory("so-2", StatusReceived)
	if AssertHistoryEqual(h1, h2) {
		t.Error("expected mismatch for different service order IDs")
	}
}

func TestAssertHistoryEqual_StatusMismatch(t *testing.T) {
	h1 := NewTestHistory("so-1", StatusReceived)
	h2 := NewTestHistory("so-1", StatusDiagnosing)
	if AssertHistoryEqual(h1, h2) {
		t.Error("expected mismatch for different statuses")
	}
}

func TestServiceOrderBuilder_Full(t *testing.T) {
	item := NewTestServiceOrderItem(ItemTypeProduct, "ref-extra", 1)
	so := NewServiceOrderBuilder().
		WithID("custom-id").
		WithCustomerID("new-cust").
		WithVehicleID("new-veh").
		WithStatus(StatusDiagnosing).
		WithItem(item).
		Build()
	if so.ID() != "custom-id" {
		t.Errorf("want 'custom-id', got %s", so.ID())
	}
	if so.CustomerID() != "new-cust" {
		t.Errorf("want 'new-cust', got %s", so.CustomerID())
	}
	if so.Status() != StatusDiagnosing {
		t.Errorf("want DIAGNOSING, got %s", so.Status())
	}
}

func TestServiceOrderBuilder_WithProductAndServiceItems(t *testing.T) {
	so := NewServiceOrderBuilder().
		WithProductItem("prod-ref", 2, 5000).
		WithServiceItem("svc-ref", 1, 10000).
		Build()
	if len(so.Items()) != 2 {
		t.Errorf("want 2 items, got %d", len(so.Items()))
	}
}

func TestServiceOrderItemBuilder_Full(t *testing.T) {
	item := NewServiceOrderItemBuilder().
		WithID("item-id").
		WithServiceOrderID("so-id").
		WithItemType(ItemTypeService).
		WithReferenceID("new-ref").
		WithName("New Name").
		WithQuantity(3).
		WithUnitPrice(2000).
		Build()
	if item.ID() != "item-id" {
		t.Errorf("want 'item-id', got %s", item.ID())
	}
	if item.ItemType() != ItemTypeService {
		t.Errorf("want SERVICE, got %s", item.ItemType())
	}
	if item.Quantity() != 3 {
		t.Errorf("want 3, got %d", item.Quantity())
	}
	if item.UnitPrice() != 2000 {
		t.Errorf("want 2000, got %d", item.UnitPrice())
	}
}

func TestNewTestServiceOrderWithItems(t *testing.T) {
	items := []*ServiceOrderItem{
		NewTestServiceOrderItem(ItemTypeProduct, "ref-1", 1),
	}
	so := NewTestServiceOrderWithItems("cust-1", "veh-1", StatusReceived, items)
	if len(so.Items()) != 1 {
		t.Errorf("want 1 item, got %d", len(so.Items()))
	}
}

func TestNewTestServiceOrderItemFull2(t *testing.T) {
	item := NewTestServiceOrderItemFull2("so-id", ItemTypeService, "ref-1", "Name", 2, 3000)
	if item.ID() != "test-item-id-ref-1" {
		t.Errorf("want 'test-item-id-ref-1', got %s", item.ID())
	}
	if item.Quantity() != 2 {
		t.Errorf("want 2, got %d", item.Quantity())
	}
}

func TestNewTestServiceOrderItemDeleted(t *testing.T) {
	item := NewTestServiceOrderItemDeleted("so-id", ItemTypeProduct, "ref-1", "Name", 1, 1000)
	if !item.IsDeleted() {
		t.Error("item should be deleted")
	}
	if item.DeletedAt() == nil {
		t.Error("DeletedAt should be set")
	}
}

// --- ServiceOrder pointer getters (full coverage of optional fields) ---

func TestServiceOrder_PointerGetters(t *testing.T) {
	now := time.Now()
	sagaID := "saga-1"
	targetStatus := StatusAuthorized
	notes := "some notes"
	prefID := "pref-1"
	payID := "pay-1"
	payURL := "http://pay"
	closed := now
	deleted := now

	so, _ := ReconstructServiceOrder(
		"id-1", "cust-1", "veh-1", "desc",
		StatusReceived, SagaStatusIdle,
		&sagaID, &targetStatus, &notes, &prefID, &payID, &payURL,
		nil, &closed, now, now, &deleted,
	)

	if so.CurrentSagaID() == nil || *so.CurrentSagaID() != "saga-1" {
		t.Error("CurrentSagaID wrong")
	}
	if so.SagaTargetStatus() == nil || *so.SagaTargetStatus() != StatusAuthorized {
		t.Error("SagaTargetStatus wrong")
	}
	if so.SagaNotes() == nil || *so.SagaNotes() != "some notes" {
		t.Error("SagaNotes wrong")
	}
	if so.MPPreferenceID() == nil || *so.MPPreferenceID() != "pref-1" {
		t.Error("MPPreferenceID wrong")
	}
	if so.MPPaymentID() == nil || *so.MPPaymentID() != "pay-1" {
		t.Error("MPPaymentID wrong")
	}
	if so.PaymentURL() == nil || *so.PaymentURL() != "http://pay" {
		t.Error("PaymentURL wrong")
	}
	if so.ClosedAt() == nil {
		t.Error("ClosedAt should be set")
	}
	if so.DeletedAt() == nil {
		t.Error("DeletedAt should be set")
	}
}
