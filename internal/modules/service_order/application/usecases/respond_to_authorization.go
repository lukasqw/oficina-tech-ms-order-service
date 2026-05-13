package usecases

import (
	"context"
	"time"

	"go.opentelemetry.io/otel/attribute"
	metricapi "go.opentelemetry.io/otel/metric"
	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/email"
	"oficina-tech/internal/shared/infra/observability"
)

type RespondToAuthorizationInput struct {
	ServiceOrderID string
	Approved       bool
	Observation    *string
	CallerID       string
	CallerRole     string
}

type RespondToAuthorizationOutput struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Description string
	Status      string
	SagaStatus  string
	SagaID      *string
	Async       bool
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type RespondToAuthorization struct {
	serviceOrderRepo service_order.Repository
	historyRepo      service_order.HistoryRepository
	sagaOrchestrator *appsaga.Orchestrator
	customerAdapter  adapters.CustomerAdapter
	emailService     email.EmailService
}

func NewRespondToAuthorization(
	serviceOrderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	sagaOrchestrator *appsaga.Orchestrator,
	customerAdapter adapters.CustomerAdapter,
	emailService email.EmailService,
) *RespondToAuthorization {
	return &RespondToAuthorization{
		serviceOrderRepo: serviceOrderRepo,
		historyRepo:      historyRepo,
		sagaOrchestrator: sagaOrchestrator,
		customerAdapter:  customerAdapter,
		emailService:     emailService,
	}
}

func (uc *RespondToAuthorization) Execute(ctx context.Context, input RespondToAuthorizationInput) (*RespondToAuthorizationOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.respond_to_authorization")
	defer span.End()

	// 1. Buscar ordem existente
	order, err := uc.serviceOrderRepo.FindByID(ctx, input.ServiceOrderID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// 2. Ownership check: CUSTOMER só pode autorizar a própria OS
	if input.CallerRole == "CUSTOMER" && order.CustomerID() != input.CallerID {
		span.RecordError(service_order.ErrForbiddenAccess)
		return nil, service_order.ErrForbiddenAccess
	}

	// 3. Validar que ordem não está deletada
	if order.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderDeleted)
		return nil, service_order.ErrServiceOrderDeleted
	}

	// 4. Validar que ordem está em PENDING_AUTHORIZATION
	if order.Status() != service_order.StatusPendingAuthorization {
		span.RecordError(service_order.ErrInvalidStatusTransition)
		return nil, service_order.ErrInvalidStatusTransition
	}

	// 4. Capturar status antigo
	oldStatus := order.Status()

	// 5. Determinar novo status baseado na aprovação
	var newStatus service_order.OrderStatus
	if input.Approved {
		newStatus = service_order.StatusAuthorized
	} else {
		newStatus = service_order.StatusAuthorizationDenied
	}

	// 6. Determinar operação de inventário
	inventoryOp := service_order.DetermineInventoryOperation(oldStatus, newStatus)

	if inventoryOp.Type != service_order.InventoryOpNone {
		result, err := uc.sagaOrchestrator.StartSaga(ctx, order.ID(), inventoryOp.Type, newStatus, input.Observation)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		sagaID := result.SagaID
		output := &RespondToAuthorizationOutput{
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

	// Capturar momento em que o status atual foi registrado antes de avançar
	statusEnteredAt := order.UpdatedAt()

	// 8. Atualizar status da ordem
	if err := order.UpdateStatus(newStatus); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// 9. Criar histórico com metadata incluindo observação se fornecida
	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, order.Status())
	if input.Observation != nil && *input.Observation != "" {
		metadata["observation"] = *input.Observation
	}

	history, err := service_order.NewHistory(
		order.ID(),
		metadata,
		order.Status(),
	)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// 10. Salvar histórico
	if err := uc.historyRepo.Save(ctx, history); err != nil {
		span.RecordError(err)
		return nil, err
	}

	// 11. Persistir alterações
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

	// 12. Enviar email de notificação ao cliente
	uc.sendStatusUpdateEmail(ctx, order, oldStatus, order.Status())

	// 13. Retornar output
	return &RespondToAuthorizationOutput{
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

// sendStatusUpdateEmail envia email de notificação ao cliente
func (uc *RespondToAuthorization) sendStatusUpdateEmail(ctx context.Context, order *service_order.ServiceOrder, oldStatus, newStatus service_order.OrderStatus) {
	// Buscar dados do cliente
	customer, err := uc.customerAdapter.GetCustomerByID(ctx, order.CustomerID())
	if err != nil {
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to load customer for authorization email",
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
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to send authorization email",
			"service_order_id", order.ID(),
			"error", err,
		)
	}
}
