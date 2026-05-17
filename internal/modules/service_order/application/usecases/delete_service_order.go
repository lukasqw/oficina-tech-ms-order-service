package usecases

import (
	"context"
	"errors"

	billingUsecases "oficina-tech/internal/modules/billing/application/usecases"
	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/infra/observability"
)

// DeleteServiceOrder use case para deletar uma ordem de serviço (soft delete)
type DeleteServiceOrder struct {
	serviceOrderRepo   service_order.Repository
	sagaOrchestrator   *appsaga.Orchestrator
	cancelPaymentOrder *billingUsecases.CancelPaymentOrder
	refundPaymentOrder *billingUsecases.RefundPaymentOrder
}

// NewDeleteServiceOrder cria uma nova instância do use case
func NewDeleteServiceOrder(
	serviceOrderRepo service_order.Repository,
	sagaOrchestrator *appsaga.Orchestrator,
	cancelPaymentOrder *billingUsecases.CancelPaymentOrder,
	refundPaymentOrder *billingUsecases.RefundPaymentOrder,
) *DeleteServiceOrder {
	return &DeleteServiceOrder{
		serviceOrderRepo:   serviceOrderRepo,
		sagaOrchestrator:   sagaOrchestrator,
		cancelPaymentOrder: cancelPaymentOrder,
		refundPaymentOrder: refundPaymentOrder,
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

// Execute executa o use case de deleção de ordem de serviço.
// Para OS em AWAITING_PAYMENT ou PAYMENT_REJECTED: cancela o Order no MP antes do saga.
// Para OS em PAID: solicita refund total no MP antes do saga.
// Falha no MP bloqueia o cancelamento para evitar inconsistência financeira.
func (uc *DeleteServiceOrder) Execute(ctx context.Context, input DeleteServiceOrderInput) (*DeleteServiceOrderOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.delete")
	defer span.End()

	existingOrder, err := uc.serviceOrderRepo.FindByID(ctx, input.ID)
	if err != nil {
		if errors.Is(err, service_order.ErrServiceOrderNotFound) {
			span.RecordError(service_order.ErrServiceOrderNotFound)
			return nil, service_order.ErrServiceOrderNotFound
		}
		span.RecordError(err)
		return nil, err
	}

	if existingOrder.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderNotFound)
		return nil, service_order.ErrServiceOrderNotFound
	}

	// Operações de pagamento MP antes do saga de inventário.
	// Falha bloqueia o cancelamento para evitar cobrar o cliente sem cancelar o estoque.
	mpOrderID := ""
	if existingOrder.MPPreferenceID() != nil {
		mpOrderID = *existingOrder.MPPreferenceID()
	}

	switch existingOrder.Status() {
	case service_order.StatusAwaitingPayment, service_order.StatusPaymentRejected:
		// Pagamento pendente ou rejeitado: tenta cancelar o Order no MP.
		// Se MP retornar erro (Order já expirado/cancelado), ignora — não bloqueia.
		if uc.cancelPaymentOrder != nil && mpOrderID != "" {
			if err := uc.cancelPaymentOrder.Execute(ctx, mpOrderID); err != nil {
				observability.LoggerFromContext(ctx).WarnContext(ctx,
					"mp cancel order returned error (non-blocking for AWAITING/REJECTED)",
					"mp_order_id", mpOrderID, "error", err)
			}
		}
	case service_order.StatusPaid:
		// OS paga: solicita refund usando o payment ID (não o preference ID).
		// Falha BLOQUEIA o cancelamento.
		mpPaymentID := ""
		if existingOrder.MPPaymentID() != nil {
			mpPaymentID = *existingOrder.MPPaymentID()
		}
		if uc.refundPaymentOrder != nil && mpPaymentID != "" {
			if err := uc.refundPaymentOrder.Execute(ctx, mpPaymentID); err != nil {
				span.RecordError(err)
				return nil, err
			}
		}
	}

	result, err := uc.sagaOrchestrator.CancelOrder(ctx, input.ID, input.Notes)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	output := &DeleteServiceOrderOutput{Success: true, Async: result.Async}
	if result.SagaID != "" {
		output.SagaID = &result.SagaID
	}
	return output, nil
}
