package implementations

import (
	"context"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
)

type inventoryAdapterStub struct{}

func NewInventoryAdapter() adapters.InventoryAdapter {
	return &inventoryAdapterStub{}
}

func (a *inventoryAdapterStub) ChangeStock(ctx context.Context, operationType dto.StockOperationType, productID string, quantity int, orderID string) error {
	return service_order.ErrSagaNotImplemented
}
