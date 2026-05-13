package usecases

import (
	"context"
	"time"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/observability"
)

type GetAllServiceOrdersInput struct {
	CustomerID    *string
	Status        *string
	SortByStatus  bool
	HideCompleted bool
}

type GetAllServiceOrdersOutput struct {
	Orders []ServiceOrderSummary
}

type ServiceOrderSummary struct {
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

type GetAllServiceOrders struct {
	serviceOrderRepo service_order.Repository
	customerAdapter  adapters.CustomerAdapter
	vehicleAdapter   adapters.VehicleAdapter
}

func NewGetAllServiceOrders(
	serviceOrderRepo service_order.Repository,
	customerAdapter adapters.CustomerAdapter,
	vehicleAdapter adapters.VehicleAdapter,
) *GetAllServiceOrders {
	return &GetAllServiceOrders{
		serviceOrderRepo: serviceOrderRepo,
		customerAdapter:  customerAdapter,
		vehicleAdapter:   vehicleAdapter,
	}
}

func (uc *GetAllServiceOrders) Execute(ctx context.Context, input GetAllServiceOrdersInput) (*GetAllServiceOrdersOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.get_all")
	defer span.End()

	// Preparar filtros para o repositório
	filters := service_order.RepositoryFilters{
		CustomerID:    input.CustomerID,
		SortByStatus:  input.SortByStatus,
		HideCompleted: input.HideCompleted,
	}

	// Converter status string para OrderStatus se fornecido
	if input.Status != nil && *input.Status != "" {
		status, err := service_order.NewOrderStatus(*input.Status)
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		filters.Status = &status
	}

	// Buscar ordens com filtros
	orders, err := uc.serviceOrderRepo.FindAllWithFilters(ctx, filters)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	// Converter para output
	summaries := make([]ServiceOrderSummary, 0, len(orders))
	for _, order := range orders {
		// Excluir ordens deletadas
		if !order.IsDeleted() {
			// Converter items
			items := make([]ServiceOrderItemOutput, 0, len(order.Items()))
			for _, item := range order.Items() {
				// Pular items deletados
				if !item.IsDeleted() {
					items = append(items, ServiceOrderItemOutput{
						ID:          item.ID(),
						ItemType:    string(item.ItemType()),
						ReferenceID: item.ReferenceID(),
						Name:        item.Name(),
						Quantity:    item.Quantity(),
						UnitPrice:   item.UnitPrice(),
						Subtotal:    item.Subtotal(),
						CreatedAt:   item.CreatedAt(),
					})
				}
			}

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

			summaries = append(summaries, ServiceOrderSummary{
				ID:          order.ID(),
				CustomerID:  order.CustomerID(),
				VehicleID:   order.VehicleID(),
				Description: order.Description(),
				Customer:    customerOutput,
				Vehicle:     vehicleOutput,
				Status:      order.Status().String(),
				SagaStatus:  order.SagaStatus(),
				Items:       items,
				TotalAmount: order.TotalAmount(),
				ClosedAt:    order.ClosedAt(),
				CreatedAt:   order.CreatedAt(),
				UpdatedAt:   order.UpdatedAt(),
			})
		}
	}

	return &GetAllServiceOrdersOutput{
		Orders: summaries,
	}, nil
}
