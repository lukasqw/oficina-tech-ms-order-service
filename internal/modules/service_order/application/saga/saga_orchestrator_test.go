package saga

import (
	"context"
	"errors"
	"testing"
	"time"

	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
)

const (
	testOrderID    = "11111111-1111-4111-8111-111111111111"
	testCustomerID = "22222222-2222-4222-8222-222222222222"
	testVehicleID  = "33333333-3333-4333-8333-333333333333"
	testProductID  = "44444444-4444-4444-8444-444444444444"
)

func TestStartSagaReservePublishesInventoryRequest(t *testing.T) {
	ctx := context.Background()
	repo := newSagaRepo(orderWithStatus(t, service_order.StatusDiagnosing, true))
	history := &mockHistoryRepo{}
	publisher := &mockPublisher{}
	orchestrator := NewOrchestrator(repo, history, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())

	result, err := orchestrator.StartSaga(ctx, testOrderID, dto.StockOpReserve, service_order.StatusPendingAuthorization, nil)
	if err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}

	if !result.Async || result.SagaID == "" {
		t.Fatalf("expected async saga with id, got async=%v saga=%q", result.Async, result.SagaID)
	}
	if result.Order.SagaStatus() != service_order.SagaStatusAwaitingInventory {
		t.Fatalf("saga status = %s", result.Order.SagaStatus())
	}
	if len(publisher.requests) != 1 {
		t.Fatalf("published requests = %d", len(publisher.requests))
	}
	if publisher.requests[0].Operation != dto.StockOpReserve || publisher.requests[0].Items[0].ProductID != testProductID {
		t.Fatalf("unexpected published request: %+v", publisher.requests[0])
	}
}

func TestHandleSucceededReserveAdvancesOrderAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	order := orderWithStatus(t, service_order.StatusDiagnosing, true)
	repo := newSagaRepo(order)
	history := &mockHistoryRepo{}
	emailSvc := email.NewMockEmailService()
	orchestrator := NewOrchestrator(repo, history, &mockPublisher{}, &mockCustomerAdapter{}, emailSvc)

	result, err := orchestrator.StartSaga(ctx, testOrderID, dto.StockOpReserve, service_order.StatusPendingAuthorization, nil)
	if err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	event := events.OrderInventoryOperationSucceeded{
		Event:      events.EventOrderInventoryOperationSucceeded,
		SagaID:     result.SagaID,
		OrderID:    testOrderID,
		Operation:  string(dto.StockOpReserve),
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	}
	if err := orchestrator.HandleSucceeded(ctx, event); err != nil {
		t.Fatalf("HandleSucceeded() error = %v", err)
	}
	if order.Status() != service_order.StatusPendingAuthorization || order.SagaStatus() != service_order.SagaStatusIdle {
		t.Fatalf("order after success status=%s saga=%s", order.Status(), order.SagaStatus())
	}
	if len(history.saved) != 1 || len(emailSvc.SentEmails) != 1 {
		t.Fatalf("history=%d emails=%d", len(history.saved), len(emailSvc.SentEmails))
	}

	if err := orchestrator.HandleSucceeded(ctx, event); err != nil {
		t.Fatalf("duplicate HandleSucceeded() error = %v", err)
	}
	if len(history.saved) != 1 {
		t.Fatalf("duplicate event should be discarded, history=%d", len(history.saved))
	}
}

func TestHandleFailedLeavesStatusAndMarksSagaFailed(t *testing.T) {
	ctx := context.Background()
	order := orderWithStatus(t, service_order.StatusInProgress, true)
	repo := newSagaRepo(order)
	history := &mockHistoryRepo{}
	orchestrator := NewOrchestrator(repo, history, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())

	result, err := orchestrator.StartSaga(ctx, testOrderID, dto.StockOpReservedDecrease, service_order.StatusCompleted, nil)
	if err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	reason := "reserved stock is insufficient"
	err = orchestrator.HandleFailed(ctx, events.OrderInventoryOperationFailed{
		Event:      events.EventOrderInventoryOperationFailed,
		SagaID:     result.SagaID,
		OrderID:    testOrderID,
		Operation:  string(dto.StockOpReservedDecrease),
		Reason:     reason,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("HandleFailed() error = %v", err)
	}
	if order.Status() != service_order.StatusInProgress || order.SagaStatus() != service_order.SagaStatusFailed {
		t.Fatalf("order after failure status=%s saga=%s", order.Status(), order.SagaStatus())
	}
	if got := history.saved[0].Metadata()["failure_reason"]; got != reason {
		t.Fatalf("failure reason = %v", got)
	}
}

func TestCancelOrderChoosesCompensationOperation(t *testing.T) {
	cases := []struct {
		name      string
		status    service_order.OrderStatus
		wantAsync bool
		wantOp    dto.StockOperationType
	}{
		{name: "received local", status: service_order.StatusReceived, wantAsync: false},
		{name: "authorized cancels reservation", status: service_order.StatusAuthorized, wantAsync: true, wantOp: dto.StockOpCancelReserved},
		{name: "completed cancels confirmed", status: service_order.StatusCompleted, wantAsync: true, wantOp: dto.StockOpCancelConfirmed},
		{name: "paid cancels confirmed", status: service_order.StatusPaid, wantAsync: true, wantOp: dto.StockOpCancelConfirmed},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			order := orderWithStatus(t, tt.status, true)
			repo := newSagaRepo(order)
			history := &mockHistoryRepo{}
			publisher := &mockPublisher{}
			orchestrator := NewOrchestrator(repo, history, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())

			result, err := orchestrator.CancelOrder(context.Background(), testOrderID, nil)
			if err != nil {
				t.Fatalf("CancelOrder() error = %v", err)
			}
			if result.Async != tt.wantAsync {
				t.Fatalf("async = %v, want %v", result.Async, tt.wantAsync)
			}
			if tt.wantAsync && publisher.requests[0].Operation != tt.wantOp {
				t.Fatalf("operation = %s, want %s", publisher.requests[0].Operation, tt.wantOp)
			}
			if !tt.wantAsync && order.Status() != service_order.StatusCanceled {
				t.Fatalf("local cancel status = %s", order.Status())
			}
		})
	}
}

func TestRecoverAwaitingSagasRepublishesSameSagaID(t *testing.T) {
	order := orderWithStatus(t, service_order.StatusInProgress, true)
	sagaID := "55555555-5555-4555-8555-555555555555"
	if err := order.StartSaga(sagaID, service_order.StatusCompleted, nil); err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	repo := newSagaRepo(order)
	publisher := &mockPublisher{}
	orchestrator := NewOrchestrator(repo, &mockHistoryRepo{}, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())

	if err := orchestrator.RecoverAwaitingSagas(context.Background()); err != nil {
		t.Fatalf("RecoverAwaitingSagas() error = %v", err)
	}
	if len(publisher.requests) != 1 {
		t.Fatalf("published requests = %d", len(publisher.requests))
	}
	if publisher.requests[0].SagaID != sagaID || publisher.requests[0].Operation != dto.StockOpReservedDecrease {
		t.Fatalf("unexpected recovery request: %+v", publisher.requests[0])
	}
}

func TestStartSagaWithoutProductItemsTransitionsLocally(t *testing.T) {
	order := orderWithStatus(t, service_order.StatusDiagnosing, false)
	repo := newSagaRepo(order)
	history := &mockHistoryRepo{}
	publisher := &mockPublisher{}
	orchestrator := NewOrchestrator(repo, history, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())

	result, err := orchestrator.StartSaga(context.Background(), testOrderID, dto.StockOpReserve, service_order.StatusPendingAuthorization, nil)
	if err != nil {
		t.Fatalf("StartSaga() error = %v", err)
	}
	if result.Async || len(publisher.requests) != 0 {
		t.Fatalf("no product items should transition locally")
	}
	if order.Status() != service_order.StatusPendingAuthorization || len(history.saved) != 1 {
		t.Fatalf("status=%s history=%d", order.Status(), len(history.saved))
	}
}

func TestStartSagaRejectsOrdersAlreadyAwaitingInventory(t *testing.T) {
	order := orderWithStatus(t, service_order.StatusDiagnosing, true)
	if err := order.StartSaga("55555555-5555-4555-8555-555555555555", service_order.StatusPendingAuthorization, nil); err != nil {
		t.Fatalf("StartSaga setup error = %v", err)
	}
	orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())

	_, err := orchestrator.StartSaga(context.Background(), testOrderID, dto.StockOpReserve, service_order.StatusPendingAuthorization, nil)
	if !errors.Is(err, service_order.ErrSagaAlreadyInProgress) {
		t.Fatalf("StartSaga() error = %v, want ErrSagaAlreadyInProgress", err)
	}
}

func TestIsCurrentSaga(t *testing.T) {
	order := orderWithStatus(t, service_order.StatusDiagnosing, true)
	sagaID := "55555555-5555-4555-8555-555555555555"
	if err := order.StartSaga(sagaID, service_order.StatusPendingAuthorization, nil); err != nil {
		t.Fatalf("StartSaga setup error = %v", err)
	}
	orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())

	current, err := orchestrator.IsCurrentSaga(context.Background(), testOrderID, sagaID)
	if err != nil {
		t.Fatalf("IsCurrentSaga() error = %v", err)
	}
	if !current {
		t.Fatalf("expected saga to be current")
	}
	current, err = orchestrator.IsCurrentSaga(context.Background(), testOrderID, "66666666-6666-4666-8666-666666666666")
	if err != nil {
		t.Fatalf("IsCurrentSaga() error = %v", err)
	}
	if current {
		t.Fatalf("unexpected saga should not be current")
	}
}

func TestTargetStatusFallbacksAndInferOperation(t *testing.T) {
	orchestrator := NewOrchestrator(newSagaRepo(), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())

	fallbacks := []struct {
		op   dto.StockOperationType
		want service_order.OrderStatus
	}{
		{dto.StockOpReserve, service_order.StatusPendingAuthorization},
		{dto.StockOpReservedDecrease, service_order.StatusCompleted},
		{dto.StockOpCancelReserved, service_order.StatusCanceled},
		{dto.StockOpCancelConfirmed, service_order.StatusCanceled},
	}
	for _, tt := range fallbacks {
		order := orderWithStatus(t, service_order.StatusDiagnosing, true)
		got, err := orchestrator.targetStatusForSuccess(order, tt.op)
		if err != nil {
			t.Fatalf("targetStatusForSuccess(%s) error = %v", tt.op, err)
		}
		if got != tt.want {
			t.Fatalf("targetStatusForSuccess(%s) = %s, want %s", tt.op, got, tt.want)
		}
	}
	if _, err := orchestrator.targetStatusForSuccess(orderWithStatus(t, service_order.StatusDiagnosing, true), dto.StockOperationType("UNKNOWN")); err == nil {
		t.Fatalf("expected unsupported operation error")
	}

	inferred := []struct {
		status service_order.OrderStatus
		target service_order.OrderStatus
		want   dto.StockOperationType
	}{
		{service_order.StatusDiagnosing, service_order.StatusPendingAuthorization, dto.StockOpReserve},
		{service_order.StatusInProgress, service_order.StatusCompleted, dto.StockOpReservedDecrease},
		{service_order.StatusPendingAuthorization, service_order.StatusAuthorizationDenied, dto.StockOpCancelReserved},
		{service_order.StatusAuthorized, service_order.StatusCanceled, dto.StockOpCancelReserved},
		{service_order.StatusCompleted, service_order.StatusCanceled, dto.StockOpCancelConfirmed},
	}
	for _, tt := range inferred {
		order := orderWithStatus(t, tt.status, true)
		if err := order.StartSaga("55555555-5555-4555-8555-555555555555", tt.target, nil); err != nil {
			t.Fatalf("StartSaga setup error = %v", err)
		}
		got, err := inferOperation(order)
		if err != nil {
			t.Fatalf("inferOperation(%s -> %s) error = %v", tt.status, tt.target, err)
		}
		if got != tt.want {
			t.Fatalf("inferOperation(%s -> %s) = %s, want %s", tt.status, tt.target, got, tt.want)
		}
	}
}

func TestSagaErrorBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("publish error is returned after saga persistence", func(t *testing.T) {
		order := orderWithStatus(t, service_order.StatusDiagnosing, true)
		publisher := &mockPublisher{err: errors.New("sqs down")}
		orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())
		_, err := orchestrator.StartSaga(ctx, testOrderID, dto.StockOpReserve, service_order.StatusPendingAuthorization, nil)
		if err == nil {
			t.Fatalf("expected publish error")
		}
		if order.SagaStatus() != service_order.SagaStatusAwaitingInventory {
			t.Fatalf("saga should remain recoverable after publish failure")
		}
	})

	t.Run("duplicate succeeded event is ignored", func(t *testing.T) {
		order := orderWithStatus(t, service_order.StatusDiagnosing, true)
		orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())
		err := orchestrator.HandleSucceeded(ctx, events.OrderInventoryOperationSucceeded{
			SagaID:    "unknown",
			OrderID:   testOrderID,
			Operation: string(dto.StockOpReserve),
		})
		if err != nil {
			t.Fatalf("HandleSucceeded duplicate error = %v", err)
		}
	})

	t.Run("duplicate failed event is ignored", func(t *testing.T) {
		order := orderWithStatus(t, service_order.StatusDiagnosing, true)
		orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())
		err := orchestrator.HandleFailed(ctx, events.OrderInventoryOperationFailed{
			SagaID:    "unknown",
			OrderID:   testOrderID,
			Operation: string(dto.StockOpReserve),
			Reason:    "duplicate",
		})
		if err != nil {
			t.Fatalf("HandleFailed duplicate error = %v", err)
		}
	})

	t.Run("delivered order cannot be canceled", func(t *testing.T) {
		order := orderWithStatus(t, service_order.StatusDelivered, true)
		orchestrator := NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, email.NewMockEmailService())
		_, err := orchestrator.CancelOrder(ctx, testOrderID, nil)
		if !errors.Is(err, service_order.ErrInvalidStatusTransition) {
			t.Fatalf("CancelOrder() error = %v", err)
		}
	})

	t.Run("recovery skips orders without saga id", func(t *testing.T) {
		order := orderWithStatus(t, service_order.StatusInProgress, true)
		order.FailSaga()
		// Force the status through the repository filter path without a current saga.
		repo := &sagaRepo{orders: map[string]*service_order.ServiceOrder{testOrderID: order}, forceAwaiting: true}
		publisher := &mockPublisher{}
		orchestrator := NewOrchestrator(repo, &mockHistoryRepo{}, publisher, &mockCustomerAdapter{}, email.NewMockEmailService())
		if err := orchestrator.RecoverAwaitingSagas(ctx); err != nil {
			t.Fatalf("RecoverAwaitingSagas() error = %v", err)
		}
		if len(publisher.requests) != 0 {
			t.Fatalf("order without current saga id should be skipped")
		}
	})
}

func TestSendStatusUpdateEmailBranchesDoNotBlock(t *testing.T) {
	order := orderWithStatus(t, service_order.StatusAuthorized, true)

	NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, nil, nil).
		sendStatusUpdateEmail(context.Background(), order, service_order.StatusPendingAuthorization, service_order.StatusAuthorized)

	NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{err: errors.New("customer down")}, email.NewMockEmailService()).
		sendStatusUpdateEmail(context.Background(), order, service_order.StatusPendingAuthorization, service_order.StatusAuthorized)

	emailSvc := email.NewMockEmailService()
	emailSvc.ShouldFail = true
	NewOrchestrator(newSagaRepo(order), &mockHistoryRepo{}, &mockPublisher{}, &mockCustomerAdapter{}, emailSvc).
		sendStatusUpdateEmail(context.Background(), order, service_order.StatusPendingAuthorization, service_order.StatusAuthorized)
}

type sagaRepo struct {
	orders        map[string]*service_order.ServiceOrder
	forceAwaiting bool
}

func newSagaRepo(orders ...*service_order.ServiceOrder) *sagaRepo {
	repo := &sagaRepo{orders: map[string]*service_order.ServiceOrder{}}
	for _, order := range orders {
		repo.orders[order.ID()] = order
	}
	return repo
}

func (r *sagaRepo) Save(context.Context, *service_order.ServiceOrder) error { return nil }
func (r *sagaRepo) SaveWithItems(context.Context, *service_order.ServiceOrder) error {
	return nil
}
func (r *sagaRepo) FindByID(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	order, ok := r.orders[id]
	if !ok {
		return nil, service_order.ErrServiceOrderNotFound
	}
	return order, nil
}
func (r *sagaRepo) FindByIDWithItems(ctx context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(ctx, id)
}
func (r *sagaRepo) FindAll(context.Context) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}
func (r *sagaRepo) FindAllWithFilters(context.Context, service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}
func (r *sagaRepo) FindByCustomerID(context.Context, string) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}
func (r *sagaRepo) FindByStatus(context.Context, service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}
func (r *sagaRepo) FindBySagaStatus(_ context.Context, sagaStatus string) ([]*service_order.ServiceOrder, error) {
	orders := []*service_order.ServiceOrder{}
	for _, order := range r.orders {
		if order.SagaStatus() == sagaStatus || r.forceAwaiting {
			orders = append(orders, order)
		}
	}
	return orders, nil
}
func (r *sagaRepo) Delete(context.Context, string) error { return nil }
func (r *sagaRepo) UpdateItemsHistoryID(context.Context, []string, string) error {
	return nil
}

type mockHistoryRepo struct {
	saved []*service_order.History
}

func (r *mockHistoryRepo) Save(_ context.Context, history *service_order.History) error {
	r.saved = append(r.saved, history)
	return nil
}
func (r *mockHistoryRepo) FindByServiceOrderID(context.Context, string) ([]*service_order.History, error) {
	return r.saved, nil
}
func (r *mockHistoryRepo) FindByID(_ context.Context, id string) (*service_order.History, error) {
	for _, history := range r.saved {
		if history.ID() == id {
			return history, nil
		}
	}
	return nil, service_order.ErrHistoryNotFound
}

type publishedRequest struct {
	SagaID    string
	OrderID   string
	Operation dto.StockOperationType
	Items     []events.InventoryItem
}

type mockPublisher struct {
	requests []publishedRequest
	err      error
}

func (p *mockPublisher) PublishRequest(_ context.Context, sagaID string, orderID string, operation dto.StockOperationType, items []events.InventoryItem) error {
	if p.err != nil {
		return p.err
	}
	p.requests = append(p.requests, publishedRequest{SagaID: sagaID, OrderID: orderID, Operation: operation, Items: items})
	return nil
}

type mockCustomerAdapter struct {
	err error
}

func (a *mockCustomerAdapter) GetCustomerByID(context.Context, string) (*dto.CustomerDTO, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &dto.CustomerDTO{ID: testCustomerID, Name: "Maria Silva", Email: "maria@example.com"}, nil
}

func orderWithStatus(t *testing.T, status service_order.OrderStatus, withProduct bool) *service_order.ServiceOrder {
	t.Helper()
	now := time.Now()
	order, err := service_order.ReconstructServiceOrder(
		testOrderID,
		testCustomerID,
		testVehicleID,
		"test order",
		status,
		service_order.SagaStatusIdle,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		now,
		now,
		nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	if withProduct {
		item, err := service_order.NewServiceOrderItem(testOrderID, service_order.ItemTypeProduct, testProductID, "Filtro", 2, 1000)
		if err != nil {
			t.Fatalf("NewServiceOrderItem() error = %v", err)
		}
		if err := item.SetID("66666666-6666-4666-8666-666666666666"); err != nil {
			t.Fatalf("SetID() error = %v", err)
		}
		if err := order.AddItem(item); err != nil && !errors.Is(err, service_order.ErrCannotModifyItemsAfterPending) && !errors.Is(err, service_order.ErrCannotModifyClosedOrder) {
			t.Fatalf("AddItem() error = %v", err)
		}
		if order.Status() != service_order.StatusReceived {
			order, err = service_order.ReconstructServiceOrder(
				testOrderID, testCustomerID, testVehicleID, "test order", status, service_order.SagaStatusIdle,
				nil, nil, nil, nil, nil, nil, []*service_order.ServiceOrderItem{item}, order.ClosedAt(), now, now, nil,
			)
			if err != nil {
				t.Fatalf("ReconstructServiceOrder(with items) error = %v", err)
			}
		}
	}
	return order
}
