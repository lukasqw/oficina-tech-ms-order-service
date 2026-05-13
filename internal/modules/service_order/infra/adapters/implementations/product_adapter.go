package implementations

import (
	"context"

	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/http_clients"
)

type productAdapterImpl struct {
	client *http_clients.MS3Client
}

func NewProductAdapter(client *http_clients.MS3Client) adapters.ProductAdapter {
	return &productAdapterImpl{client: client}
}

func (a *productAdapterImpl) GetProductByID(ctx context.Context, id string) (*dto.ProductDTO, error) {
	return a.client.GetProductByID(ctx, id)
}
