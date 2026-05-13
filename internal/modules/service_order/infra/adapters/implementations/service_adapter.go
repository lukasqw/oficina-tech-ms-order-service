package implementations

import (
	"context"

	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/http_clients"
)

type serviceAdapterImpl struct {
	client *http_clients.MS3Client
}

func NewServiceAdapter(client *http_clients.MS3Client) adapters.ServiceAdapter {
	return &serviceAdapterImpl{client: client}
}

func (a *serviceAdapterImpl) GetServiceByID(ctx context.Context, id string) (*dto.ServiceDTO, error) {
	return a.client.GetServiceByID(ctx, id)
}
