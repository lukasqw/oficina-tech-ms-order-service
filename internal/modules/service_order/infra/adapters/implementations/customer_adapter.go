package implementations

import (
	"context"

	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/http_clients"
)

type customerAdapterImpl struct {
	client *http_clients.MS1Client
}

func NewCustomerAdapter(client *http_clients.MS1Client) adapters.CustomerAdapter {
	return &customerAdapterImpl{client: client}
}

func (a *customerAdapterImpl) GetCustomerByID(ctx context.Context, id string) (*dto.CustomerDTO, error) {
	return a.client.GetCustomerByID(ctx, id)
}
