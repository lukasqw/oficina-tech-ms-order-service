package adapters

import (
	"context"
	"oficina-tech/internal/shared/dto"
)

type CustomerAdapter interface {
	GetCustomerByID(ctx context.Context, id string) (*dto.CustomerDTO, error)
}

type InventoryAdapter interface {
	ChangeStock(ctx context.Context, operationType dto.StockOperationType, productID string, quantity int, orderID string) error
}

type ProductAdapter interface {
	GetProductByID(ctx context.Context, id string) (*dto.ProductDTO, error)
}

type ServiceAdapter interface {
	GetServiceByID(ctx context.Context, id string) (*dto.ServiceDTO, error)
}

type VehicleAdapter interface {
	GetVehicleByID(ctx context.Context, id string) (*dto.VehicleDTO, error)
	ValidateVehicleOwnership(ctx context.Context, vehicleID, customerID string) (bool, error)
}
