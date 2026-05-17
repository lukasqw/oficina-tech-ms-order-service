package saga

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
)

type InventoryOperationPublisher interface {
	PublishRequest(ctx context.Context, sagaID string, orderID string, operation dto.StockOperationType, items []events.InventoryItem) error
}

type StartResult struct {
	Order  *service_order.ServiceOrder
	SagaID string
	Async  bool
}

type Orchestrator struct {
	orderRepo       service_order.Repository
	historyRepo     service_order.HistoryRepository
	publisher       InventoryOperationPublisher
	customerAdapter adapters.CustomerAdapter
	emailService    email.EmailService
}

func NewOrchestrator(
	orderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	publisher InventoryOperationPublisher,
	customerAdapter adapters.CustomerAdapter,
	emailService email.EmailService,
) *Orchestrator {
	return &Orchestrator{
		orderRepo:       orderRepo,
		historyRepo:     historyRepo,
		publisher:       publisher,
		customerAdapter: customerAdapter,
		emailService:    emailService,
	}
}

func (o *Orchestrator) StartSaga(ctx context.Context, orderID string, operation dto.StockOperationType, targetStatus service_order.OrderStatus, notes *string) (*StartResult, error) {
	order, err := o.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.IsDeleted() {
		return nil, service_order.ErrServiceOrderDeleted
	}
	if order.SagaStatus() == service_order.SagaStatusAwaitingInventory {
		return nil, service_order.ErrSagaAlreadyInProgress
	}

	items := inventoryItems(order)
	if len(items) == 0 {
		if err := o.transitionLocal(ctx, order, targetStatus, operation, "", notes); err != nil {
			return nil, err
		}
		return &StartResult{Order: order, Async: false}, nil
	}

	sagaID := uuid.Must(uuid.NewV7()).String()
	if err := order.StartSaga(sagaID, targetStatus, notes); err != nil {
		return nil, err
	}
	if err := o.orderRepo.Save(ctx, order); err != nil {
		return nil, err
	}

	if err := o.publisher.PublishRequest(ctx, sagaID, order.ID(), operation, items); err != nil {
		return nil, err
	}

	return &StartResult{Order: order, SagaID: sagaID, Async: true}, nil
}

func (o *Orchestrator) CancelOrder(ctx context.Context, orderID string, notes *string) (*StartResult, error) {
	order, err := o.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.IsDeleted() {
		return nil, service_order.ErrServiceOrderDeleted
	}
	if order.SagaStatus() == service_order.SagaStatusAwaitingInventory {
		return nil, service_order.ErrSagaAlreadyInProgress
	}

	switch order.Status() {
	case service_order.StatusReceived, service_order.StatusDiagnosing:
		if err := o.transitionLocal(ctx, order, service_order.StatusCanceled, service_order.InventoryOpNone, "", notes); err != nil {
			return nil, err
		}
		return &StartResult{Order: order, Async: false}, nil
	case service_order.StatusPendingAuthorization, service_order.StatusAuthorized, service_order.StatusInProgress:
		return o.StartSaga(ctx, orderID, dto.StockOpCancelReserved, service_order.StatusCanceled, notes)
	case service_order.StatusCompleted, service_order.StatusAwaitingPayment,
		service_order.StatusPaymentRejected, service_order.StatusPaid:
		return o.StartSaga(ctx, orderID, dto.StockOpCancelConfirmed, service_order.StatusCanceled, notes)
	default:
		return nil, service_order.ErrInvalidStatusTransition
	}
}

func (o *Orchestrator) IsCurrentSaga(ctx context.Context, orderID, sagaID string) (bool, error) {
	order, err := o.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return false, err
	}
	return order.CanProcessSaga(sagaID), nil
}

func (o *Orchestrator) HandleSucceeded(ctx context.Context, event events.OrderInventoryOperationSucceeded) error {
	order, err := o.orderRepo.FindByID(ctx, event.OrderID)
	if err != nil {
		return err
	}
	if !order.CanProcessSaga(event.SagaID) {
		return nil
	}

	operation := dto.StockOperationType(event.Operation)
	targetStatus, err := o.targetStatusForSuccess(order, operation)
	if err != nil {
		return err
	}

	notes := order.SagaNotes()
	oldStatus := order.Status()
	if oldStatus != targetStatus {
		if err := order.UpdateStatus(targetStatus); err != nil {
			return err
		}
	}
	order.CompleteSaga()

	if err := o.saveHistory(ctx, order, oldStatus, targetStatus, operation, event.SagaID, nil, notes); err != nil {
		return err
	}
	if err := o.orderRepo.Save(ctx, order); err != nil {
		return err
	}
	o.sendStatusUpdateEmail(ctx, order, oldStatus, targetStatus)
	return nil
}

func (o *Orchestrator) HandleFailed(ctx context.Context, event events.OrderInventoryOperationFailed) error {
	order, err := o.orderRepo.FindByID(ctx, event.OrderID)
	if err != nil {
		return err
	}
	if !order.CanProcessSaga(event.SagaID) {
		return nil
	}

	operation := dto.StockOperationType(event.Operation)
	oldStatus := order.Status()
	reason := event.Reason
	order.FailSaga()

	if err := o.saveHistory(ctx, order, oldStatus, oldStatus, operation, event.SagaID, &reason, nil); err != nil {
		return err
	}
	if err := o.orderRepo.Save(ctx, order); err != nil {
		return err
	}
	o.sendStatusUpdateEmail(ctx, order, oldStatus, oldStatus)
	return nil
}

func (o *Orchestrator) RecoverAwaitingSagas(ctx context.Context) error {
	orders, err := o.orderRepo.FindBySagaStatus(ctx, service_order.SagaStatusAwaitingInventory)
	if err != nil {
		return err
	}

	for _, order := range orders {
		if order.CurrentSagaID() == nil {
			continue
		}
		operation, err := inferOperation(order)
		if err != nil {
			return err
		}
		if err := o.publisher.PublishRequest(ctx, *order.CurrentSagaID(), order.ID(), operation, inventoryItems(order)); err != nil {
			return err
		}
		slog.Info("republished awaiting inventory saga", "order_id", order.ID(), "saga_id", *order.CurrentSagaID(), "operation", operation)
	}
	return nil
}

func (o *Orchestrator) transitionLocal(
	ctx context.Context,
	order *service_order.ServiceOrder,
	targetStatus service_order.OrderStatus,
	operation dto.StockOperationType,
	sagaID string,
	notes *string,
) error {
	oldStatus := order.Status()
	if oldStatus != targetStatus {
		if err := order.UpdateStatus(targetStatus); err != nil {
			return err
		}
	}
	order.CompleteSaga()

	if err := o.saveHistory(ctx, order, oldStatus, targetStatus, operation, sagaID, nil, notes); err != nil {
		return err
	}
	if err := o.orderRepo.Save(ctx, order); err != nil {
		return err
	}
	o.sendStatusUpdateEmail(ctx, order, oldStatus, targetStatus)
	return nil
}

func (o *Orchestrator) targetStatusForSuccess(order *service_order.ServiceOrder, operation dto.StockOperationType) (service_order.OrderStatus, error) {
	if target := order.SagaTargetStatus(); target != nil {
		return *target, nil
	}

	switch operation {
	case dto.StockOpReserve:
		return service_order.StatusPendingAuthorization, nil
	case dto.StockOpReservedDecrease:
		return service_order.StatusCompleted, nil
	case dto.StockOpCancelReserved, dto.StockOpCancelConfirmed:
		return service_order.StatusCanceled, nil
	default:
		return "", fmt.Errorf("unsupported saga operation %s", operation)
	}
}

func (o *Orchestrator) saveHistory(
	ctx context.Context,
	order *service_order.ServiceOrder,
	oldStatus service_order.OrderStatus,
	newStatus service_order.OrderStatus,
	operation dto.StockOperationType,
	sagaID string,
	failureReason *string,
	notes *string,
) error {
	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, newStatus)
	metadata["operation"] = string(operation)
	if sagaID != "" {
		metadata["saga_id"] = sagaID
	}
	if failureReason != nil && *failureReason != "" {
		metadata["failure_reason"] = *failureReason
	}
	if notes != nil && *notes != "" {
		metadata["notes"] = *notes
	}
	metadata["snapshot"] = snapshot(order)

	history, err := service_order.NewHistory(order.ID(), metadata, newStatus)
	if err != nil {
		return err
	}
	return o.historyRepo.Save(ctx, history)
}

func (o *Orchestrator) sendStatusUpdateEmail(ctx context.Context, order *service_order.ServiceOrder, oldStatus, newStatus service_order.OrderStatus) {
	if o.customerAdapter == nil || o.emailService == nil {
		return
	}
	customer, err := o.customerAdapter.GetCustomerByID(ctx, order.CustomerID())
	if err != nil {
		slog.Warn("failed to load customer for status email", "order_id", order.ID(), "customer_id", order.CustomerID(), "error", err)
		return
	}
	if err := o.emailService.SendStatusUpdateEmail(customer.Email, customer.Name, order.ID(), oldStatus.String(), newStatus.String()); err != nil {
		slog.Warn("failed to send status email", "order_id", order.ID(), "error", err)
	}
}

func inventoryItems(order *service_order.ServiceOrder) []events.InventoryItem {
	productItems := order.GetProductItems()
	items := make([]events.InventoryItem, 0, len(productItems))
	for _, item := range productItems {
		items = append(items, events.InventoryItem{
			ProductID: item.ReferenceID(),
			Quantity:  item.Quantity(),
		})
	}
	return items
}

func snapshot(order *service_order.ServiceOrder) map[string]any {
	items := make([]map[string]any, 0, len(order.Items()))
	for _, item := range order.Items() {
		if item.IsDeleted() {
			continue
		}
		items = append(items, map[string]any{
			"id":           item.ID(),
			"item_type":    string(item.ItemType()),
			"reference_id": item.ReferenceID(),
			"name":         item.Name(),
			"quantity":     item.Quantity(),
			"unit_price":   item.UnitPrice(),
			"subtotal":     item.Subtotal(),
		})
	}

	return map[string]any{
		"order_id":     order.ID(),
		"customer_id":  order.CustomerID(),
		"vehicle_id":   order.VehicleID(),
		"status":       order.Status().String(),
		"total_amount": order.TotalAmount(),
		"items":        items,
		"captured_at":  time.Now().UTC().Format(time.RFC3339),
	}
}

func inferOperation(order *service_order.ServiceOrder) (dto.StockOperationType, error) {
	target := order.SagaTargetStatus()
	if target == nil {
		return "", fmt.Errorf("order %s awaiting inventory without target status", order.ID())
	}

	switch *target {
	case service_order.StatusPendingAuthorization:
		return dto.StockOpReserve, nil
	case service_order.StatusCompleted:
		return dto.StockOpReservedDecrease, nil
	case service_order.StatusAuthorizationDenied:
		return dto.StockOpCancelReserved, nil
	case service_order.StatusCanceled:
		if order.Status() == service_order.StatusCompleted || order.Status() == service_order.StatusPaid {
			return dto.StockOpCancelConfirmed, nil
		}
		return dto.StockOpCancelReserved, nil
	default:
		return "", fmt.Errorf("cannot infer inventory operation for target %s", target.String())
	}
}
