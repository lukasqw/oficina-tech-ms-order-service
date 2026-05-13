package usecases

import (
	"context"
	"errors"

	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/infra/observability"
)

// DeleteServiceOrder use case para deletar uma ordem de serviço (soft delete)
type DeleteServiceOrder struct {
	serviceOrderRepo service_order.Repository
	sagaOrchestrator *appsaga.Orchestrator
}

// NewDeleteServiceOrder cria uma nova instância do use case
func NewDeleteServiceOrder(serviceOrderRepo service_order.Repository, sagaOrchestrator *appsaga.Orchestrator) *DeleteServiceOrder {
	return &DeleteServiceOrder{
		serviceOrderRepo: serviceOrderRepo,
		sagaOrchestrator: sagaOrchestrator,
	}
}

// Input representa os dados de entrada para deletar uma ordem de serviço
type DeleteServiceOrderInput struct {
	ID    string
	Notes *string
}

// Output representa o resultado da operação de deleção
type DeleteServiceOrderOutput struct {
	Success bool
	Async   bool
	SagaID  *string
}

// Execute executa o use case de deleção de ordem de serviço
func (uc *DeleteServiceOrder) Execute(ctx context.Context, input DeleteServiceOrderInput) (*DeleteServiceOrderOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.delete")
	defer span.End()

	// Validar que a ordem existe antes de deletar
	existingOrder, err := uc.serviceOrderRepo.FindByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			span.RecordError(service_order.ErrServiceOrderNotFound)
			return nil, service_order.ErrServiceOrderNotFound
		}
		span.RecordError(err)
		return nil, err
	}

	// Verificar se a ordem já está deletada
	if existingOrder.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderNotFound)
		return nil, service_order.ErrServiceOrderNotFound
	}

	result, err := uc.sagaOrchestrator.CancelOrder(ctx, input.ID, input.Notes)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	output := &DeleteServiceOrderOutput{
		Success: true,
		Async:   result.Async,
	}
	if result.SagaID != "" {
		output.SagaID = &result.SagaID
	}

	return output, nil
}
