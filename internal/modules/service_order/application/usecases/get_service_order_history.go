package usecases

import (
	"context"
	"time"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/infra/observability"
)

type GetServiceOrderHistoryInput struct {
	ServiceOrderID string
}

type HistoryOutput struct {
	ID             string
	ServiceOrderID string
	Metadata       map[string]interface{}
	Status         string
	CreatedAt      time.Time
}

type GetServiceOrderHistoryOutput struct {
	History []HistoryOutput
}

type GetServiceOrderHistory struct {
	historyRepo service_order.HistoryRepository
}

func NewGetServiceOrderHistory(historyRepo service_order.HistoryRepository) *GetServiceOrderHistory {
	return &GetServiceOrderHistory{
		historyRepo: historyRepo,
	}
}

func (uc *GetServiceOrderHistory) Execute(ctx context.Context, input GetServiceOrderHistoryInput) (*GetServiceOrderHistoryOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.get_history")
	defer span.End()

	// Validar input
	if input.ServiceOrderID == "" {
		span.RecordError(service_order.ErrInvalidServiceOrderID)
		return nil, service_order.ErrInvalidServiceOrderID
	}

	// Buscar histórico da ordem de serviço
	histories, err := uc.historyRepo.FindByServiceOrderID(ctx, input.ServiceOrderID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Mapear domain entities para output
	historyOutputs := make([]HistoryOutput, 0, len(histories))
	for _, history := range histories {
		historyOutputs = append(historyOutputs, HistoryOutput{
			ID:             history.ID(),
			ServiceOrderID: history.ServiceOrderID(),
			Metadata:       history.Metadata(),
			Status:         history.Status().String(),
			CreatedAt:      history.CreatedAt(),
		})
	}

	return &GetServiceOrderHistoryOutput{
		History: historyOutputs,
	}, nil
}
