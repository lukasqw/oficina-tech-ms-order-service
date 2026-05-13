package usecases

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	billingUsecases "oficina-tech/internal/modules/billing/application/usecases"
	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/email"
	"oficina-tech/internal/shared/infra/observability"
)

type AdvanceServiceOrderStatusInput struct {
	ServiceOrderID string
}

type AdvanceServiceOrderStatusOutput struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Description string
	Status      string
	SagaStatus  string
	SagaID      *string
	Async       bool
	PaymentURL  *string
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type AdvanceServiceOrderStatus struct {
	serviceOrderRepo service_order.Repository
	historyRepo      service_order.HistoryRepository
	sagaOrchestrator *appsaga.Orchestrator
	paymentUseCase   *billingUsecases.CreatePaymentPreference
	customerAdapter  adapters.CustomerAdapter
	emailService     email.EmailService
}

func NewAdvanceServiceOrderStatus(
	serviceOrderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	sagaOrchestrator *appsaga.Orchestrator,
	paymentUseCase *billingUsecases.CreatePaymentPreference,
	customerAdapter adapters.CustomerAdapter,
	emailService email.EmailService,
) *AdvanceServiceOrderStatus {
	return &AdvanceServiceOrderStatus{
		serviceOrderRepo: serviceOrderRepo,
		historyRepo:      historyRepo,
		sagaOrchestrator: sagaOrchestrator,
		paymentUseCase:   paymentUseCase,
		customerAdapter:  customerAdapter,
		emailService:     emailService,
	}
}

func (uc *AdvanceServiceOrderStatus) Execute(ctx context.Context, input AdvanceServiceOrderStatusInput) (*AdvanceServiceOrderStatusOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.advance_status")
	defer span.End()

	// Buscar ordem existente
	order, err := uc.serviceOrderRepo.FindByID(ctx, input.ServiceOrderID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Validar que ordem não está deletada
	if order.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderDeleted)
		return nil, service_order.ErrServiceOrderDeleted
	}

	// Capturar status antigo antes da mudança
	oldStatus := order.Status()
	statusEnteredAt := order.UpdatedAt()

	// Obter próximo status usando método NextStatus() do OrderStatus
	nextStatus, err := oldStatus.NextStatus()
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Determinar qual operação de inventário executar (lógica de domínio)
	inventoryOp := service_order.DetermineInventoryOperation(oldStatus, nextStatus)

	if inventoryOp.Type != service_order.InventoryOpNone {
		result, err := uc.sagaOrchestrator.StartSaga(ctx, order.ID(), inventoryOp.Type, nextStatus, nil)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		sagaID := result.SagaID
		output := &AdvanceServiceOrderStatusOutput{
			ID:          result.Order.ID(),
			CustomerID:  result.Order.CustomerID(),
			VehicleID:   result.Order.VehicleID(),
			Description: result.Order.Description(),
			Status:      result.Order.Status().String(),
			SagaStatus:  result.Order.SagaStatus(),
			Async:       result.Async,
			ClosedAt:    result.Order.ClosedAt(),
			CreatedAt:   result.Order.CreatedAt(),
			UpdatedAt:   result.Order.UpdatedAt(),
		}
		if sagaID != "" {
			output.SagaID = &sagaID
		}
		return output, nil
	}

	if oldStatus == service_order.StatusCompleted && nextStatus == service_order.StatusAwaitingPayment {
		output, err := uc.startPayment(ctx, order, oldStatus, statusEnteredAt)
		if err != nil {
			span.RecordError(err)
		}
		return output, err
	}

	// Atualizar status da ordem usando UpdateStatus
	if err := order.UpdateStatus(nextStatus); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Create history with status-only metadata (old and new)
	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, order.Status())
	history, err := service_order.NewHistory(
		order.ID(),
		metadata,
		order.Status(),
	)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Save history first
	if err := uc.historyRepo.Save(ctx, history); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Persistir alterações usando repository.Save
	if err := uc.serviceOrderRepo.Save(ctx, order); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Record metrics only after successful persistence
	if observability.ServiceOrderStatusDuration != nil {
		observability.ServiceOrderStatusDuration.Record(ctx, time.Since(statusEnteredAt).Seconds(),
			metricapi.WithAttributes(
				attribute.String("status", oldStatus.String()),
			),
		)
	}

	if observability.ServiceOrderStatusTransition != nil {
		observability.ServiceOrderStatusTransition.Add(ctx, 1,
			metricapi.WithAttributes(
				attribute.String("from_status", oldStatus.String()),
				attribute.String("to_status", order.Status().String()),
				attribute.String("result", "success"),
			),
		)
	}

	// Enviar email de notificação ao cliente
	uc.sendStatusUpdateEmail(ctx, order, oldStatus, order.Status())

	// Retornar output
	return &AdvanceServiceOrderStatusOutput{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		ClosedAt:    order.ClosedAt(),
		CreatedAt:   order.CreatedAt(),
		UpdatedAt:   order.UpdatedAt(),
	}, nil
}

func (uc *AdvanceServiceOrderStatus) startPayment(ctx context.Context, order *service_order.ServiceOrder, oldStatus service_order.OrderStatus, statusEnteredAt time.Time) (*AdvanceServiceOrderStatusOutput, error) {
	preference, err := uc.paymentUseCase.Execute(ctx, order)
	if err != nil {
		return nil, err
	}
	if err := order.AwaitPayment(preference.ID, preference.InitURL); err != nil {
		return nil, err
	}

	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, order.Status())
	metadata["mp_preference_id"] = preference.ID
	metadata["payment_url"] = preference.InitURL
	history, err := service_order.NewHistory(order.ID(), metadata, order.Status())
	if err != nil {
		return nil, err
	}
	if err := uc.historyRepo.Save(ctx, history); err != nil {
		return nil, err
	}
	if err := uc.serviceOrderRepo.Save(ctx, order); err != nil {
		return nil, err
	}

	if observability.ServiceOrderStatusDuration != nil {
		observability.ServiceOrderStatusDuration.Record(ctx, time.Since(statusEnteredAt).Seconds(),
			metricapi.WithAttributes(attribute.String("status", oldStatus.String())),
		)
	}
	if observability.ServiceOrderStatusTransition != nil {
		observability.ServiceOrderStatusTransition.Add(ctx, 1,
			metricapi.WithAttributes(
				attribute.String("from_status", oldStatus.String()),
				attribute.String("to_status", order.Status().String()),
				attribute.String("result", "success"),
			),
		)
	}

	uc.sendStatusUpdateEmail(ctx, order, oldStatus, order.Status())

	return &AdvanceServiceOrderStatusOutput{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		PaymentURL:  order.PaymentURL(),
		ClosedAt:    order.ClosedAt(),
		CreatedAt:   order.CreatedAt(),
		UpdatedAt:   order.UpdatedAt(),
	}, nil
}

// sendStatusUpdateEmail envia email de notificação ao cliente
func (uc *AdvanceServiceOrderStatus) sendStatusUpdateEmail(ctx context.Context, order *service_order.ServiceOrder, oldStatus, newStatus service_order.OrderStatus) {
	// Buscar dados do cliente
	customer, err := uc.customerAdapter.GetCustomerByID(ctx, order.CustomerID())
	if err != nil {
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to load customer for status email",
			"service_order_id", order.ID(),
			"customer_id", order.CustomerID(),
			"error", err,
		)
		return
	}

	if err := uc.emailService.SendStatusUpdateEmail(
		customer.Email,
		customer.Name,
		order.ID(),
		oldStatus.String(),
		newStatus.String(),
	); err != nil {
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to send status email",
			"service_order_id", order.ID(),
			"error", err,
		)
	}
}
