package implementations

import (
	"context"

	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/http_clients"
)

type vehicleAdapterImpl struct {
	client *http_clients.MS1Client
}

func NewVehicleAdapter(client *http_clients.MS1Client) adapters.VehicleAdapter {
	return &vehicleAdapterImpl{client: client}
}

func (a *vehicleAdapterImpl) GetVehicleByID(ctx context.Context, id string) (*dto.VehicleDTO, error) {
	return a.client.GetVehicleByID(ctx, id)
}

func (a *vehicleAdapterImpl) ValidateVehicleOwnership(ctx context.Context, vehicleID, customerID string) (bool, error) {
	vehicle, err := a.client.GetVehicleByID(ctx, vehicleID)
	if err != nil {
		return false, err
	}
	return vehicle.CustomerID == customerID, nil
}
