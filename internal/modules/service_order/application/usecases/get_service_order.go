package usecases

import (
	"context"
	"sort"
	"time"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/observability"
)

type GetServiceOrderInput struct {
	ID string
}

type GetServiceOrderOutput struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Description string
	Customer    *CustomerOutput
	Vehicle     *VehicleOutput
	Status      string
	SagaStatus  string
	Items       []ServiceOrderItemOutput
	TotalAmount int
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CustomerOutput struct {
	ID    string
	Name  string
	Email string
	Phone string
}

type VehicleOutput struct {
	ID              string
	CustomerID      string
	LicensePlate    string
	Brand           string
	Model           string
	ModelYear       int
	ManufactureYear int
}

type GetServiceOrder struct {
	serviceOrderRepo service_order.Repository
	productAdapter   adapters.ProductAdapter
	serviceAdapter   adapters.ServiceAdapter
	customerAdapter  adapters.CustomerAdapter
	vehicleAdapter   adapters.VehicleAdapter
}

func NewGetServiceOrder(
	serviceOrderRepo service_order.Repository,
	productAdapter adapters.ProductAdapter,
	serviceAdapter adapters.ServiceAdapter,
	customerAdapter adapters.CustomerAdapter,
	vehicleAdapter adapters.VehicleAdapter,
) *GetServiceOrder {
	return &GetServiceOrder{
		serviceOrderRepo: serviceOrderRepo,
		productAdapter:   productAdapter,
		serviceAdapter:   serviceAdapter,
		customerAdapter:  customerAdapter,
		vehicleAdapter:   vehicleAdapter,
	}
}

func (uc *GetServiceOrder) Execute(ctx context.Context, input GetServiceOrderInput) (*GetServiceOrderOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.get")
	defer span.End()

	// Buscar ordem por ID com items
	order, err := uc.serviceOrderRepo.FindByIDWithItems(ctx, input.ID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Validar que ordem não está deletada
	if order.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderDeleted)
		return nil, service_order.ErrServiceOrderDeleted
	}

	// Enriquecer items com detalhes de produtos/serviços
	items := order.Items()
	itemOutputs := make([]ServiceOrderItemOutput, 0, len(items))

	// Ordenar items por data de criação (mais antigos primeiro)
	sortedItems := make([]*service_order.ServiceOrderItem, len(items))
	copy(sortedItems, items)
	sort.Slice(sortedItems, func(i, j int) bool {
		return sortedItems[i].CreatedAt().Before(sortedItems[j].CreatedAt())
	})

	for _, item := range sortedItems {
		// Pular items deletados
		if item.IsDeleted() {
			continue
		}

		itemOutput := ServiceOrderItemOutput{
			ID:          item.ID(),
			ItemType:    string(item.ItemType()),
			ReferenceID: item.ReferenceID(),
			Name:        item.Name(),
			Quantity:    item.Quantity(),
			UnitPrice:   item.UnitPrice(),
			Subtotal:    item.Subtotal(),
			CreatedAt:   item.CreatedAt(),
		}

		itemOutputs = append(itemOutputs, itemOutput)
	}

	// Calcular valor total
	totalAmount := order.TotalAmount()

	// Buscar dados do cliente
	var customerOutput *CustomerOutput
	customer, err := uc.customerAdapter.GetCustomerByID(ctx, order.CustomerID())
	if err == nil && customer != nil {
		customerOutput = &CustomerOutput{
			ID:    customer.ID,
			Name:  customer.Name,
			Email: customer.Email,
			Phone: customer.Phone,
		}
	}

	// Buscar dados do veículo
	var vehicleOutput *VehicleOutput
	vehicle, err := uc.vehicleAdapter.GetVehicleByID(ctx, order.VehicleID())
	if err == nil && vehicle != nil {
		vehicleOutput = &VehicleOutput{
			ID:              vehicle.ID,
			CustomerID:      vehicle.CustomerID,
			LicensePlate:    vehicle.LicensePlate,
			Brand:           vehicle.Brand,
			Model:           vehicle.Model,
			ModelYear:       vehicle.ModelYear,
			ManufactureYear: vehicle.ManufactureYear,
		}
	}

	// Retornar output
	return &GetServiceOrderOutput{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Customer:    customerOutput,
		Vehicle:     vehicleOutput,
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		Items:       itemOutputs,
		TotalAmount: totalAmount,
		ClosedAt:    order.ClosedAt(),
		CreatedAt:   order.CreatedAt(),
		UpdatedAt:   order.UpdatedAt(),
	}, nil
}
