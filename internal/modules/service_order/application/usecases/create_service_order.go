package usecases

import (
	"context"
	"time"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/observability"
)

type ItemInput struct {
	ItemType    string
	ReferenceID string
	Quantity    int
}

type CreateServiceOrderInput struct {
	CustomerID  string
	VehicleID   string
	Description string
	Items       []ItemInput
}

type CreateServiceOrderItemOutput struct {
	ID          string
	ItemType    string
	ReferenceID string
	Name        string
	Quantity    int
	UnitPrice   int
	Subtotal    int
	CreatedAt   time.Time
}

type CreateServiceOrderOutput struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Description string
	Status      string
	SagaStatus  string
	Items       []CreateServiceOrderItemOutput
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateServiceOrder struct {
	serviceOrderRepo service_order.Repository
	customerAdapter  adapters.CustomerAdapter
	vehicleAdapter   adapters.VehicleAdapter
	productAdapter   adapters.ProductAdapter
	serviceAdapter   adapters.ServiceAdapter
}

func NewCreateServiceOrder(
	serviceOrderRepo service_order.Repository,
	customerAdapter adapters.CustomerAdapter,
	vehicleAdapter adapters.VehicleAdapter,
	productAdapter adapters.ProductAdapter,
	serviceAdapter adapters.ServiceAdapter,
) *CreateServiceOrder {
	return &CreateServiceOrder{
		serviceOrderRepo: serviceOrderRepo,
		customerAdapter:  customerAdapter,
		vehicleAdapter:   vehicleAdapter,
		productAdapter:   productAdapter,
		serviceAdapter:   serviceAdapter,
	}
}

func (uc *CreateServiceOrder) Execute(ctx context.Context, input CreateServiceOrderInput) (*CreateServiceOrderOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.create")
	defer span.End()

	logger := observability.LoggerFromContext(ctx)

	logger.InfoContext(ctx, "creating service order",
		"customer_id", input.CustomerID,
		"vehicle_id", input.VehicleID,
		"item_count", len(input.Items),
	)

	// Validar existência do customer através do adapter
	_, err := uc.customerAdapter.GetCustomerByID(ctx, input.CustomerID)
	if err != nil {
		logger.WarnContext(ctx, "customer not found for service order creation",
			"customer_id", input.CustomerID,
			"error", err,
		)
		span.RecordError(err)
		return nil, err
	}

	// Validar existência e propriedade do vehicle através do adapter
	if input.VehicleID != "" {
		vehicleDTO, err := uc.vehicleAdapter.GetVehicleByID(ctx, input.VehicleID)
		if err != nil {
			logger.WarnContext(ctx, "vehicle not found for service order creation",
				"vehicle_id", input.VehicleID,
				"error", err,
			)
			span.RecordError(err)
			return nil, err
		}

		// Validar que o veículo pertence ao cliente da ordem de serviço
		if vehicleDTO.CustomerID != input.CustomerID {
			logger.WarnContext(ctx, "vehicle does not belong to customer",
				"vehicle_id", input.VehicleID,
				"vehicle_customer_id", vehicleDTO.CustomerID,
				"requested_customer_id", input.CustomerID,
			)
			span.RecordError(service_order.ErrVehicleDoesNotBelongToCustomer)
			return nil, service_order.ErrVehicleDoesNotBelongToCustomer
		}
	}

	// Validar todos os itens antes de criar a ordem (fail fast)
	var itemsToAdd []*service_order.ServiceOrderItem
	for _, itemInput := range input.Items {
		// Validar tipo do item
		itemType := service_order.ItemType(itemInput.ItemType)
		if itemType != service_order.ItemTypeProduct && itemType != service_order.ItemTypeService {
			logger.WarnContext(ctx, "invalid item type on service order creation",
				"item_type", itemInput.ItemType,
				"reference_id", itemInput.ReferenceID,
			)
			span.RecordError(service_order.ErrInvalidItemType)
			return nil, service_order.ErrInvalidItemType
		}

		var name string
		var price int

		// Validar e capturar informações do produto ou serviço
		if itemType == service_order.ItemTypeProduct {
			product, err := uc.productAdapter.GetProductByID(ctx, itemInput.ReferenceID)
			if err != nil {
				logger.WarnContext(ctx, "product not found for service order item",
					"reference_id", itemInput.ReferenceID,
					"error", err,
				)
				span.RecordError(err)
				return nil, err
			}
			name = product.Name
			price = product.Price
		} else {
			service, err := uc.serviceAdapter.GetServiceByID(ctx, itemInput.ReferenceID)
			if err != nil {
				logger.WarnContext(ctx, "service not found for service order item",
					"reference_id", itemInput.ReferenceID,
					"error", err,
				)
				span.RecordError(err)
				return nil, err
			}
			name = service.Name
			price = service.Price
		}

		// Criar ServiceOrderItem (será associado à ordem após sua criação)
		item, err := service_order.NewServiceOrderItem(
			"", // serviceOrderID será definido após criar a ordem
			itemType,
			itemInput.ReferenceID,
			name,
			itemInput.Quantity,
			price,
		)
		if err != nil {
			logger.WarnContext(ctx, "failed to build service order item",
				"reference_id", itemInput.ReferenceID,
				"item_type", itemInput.ItemType,
				"quantity", itemInput.Quantity,
				"error", err,
			)
			span.RecordError(err)
			return nil, err
		}

		itemsToAdd = append(itemsToAdd, item)
	}

	// Criar ServiceOrder com status RECEIVED
	order, err := service_order.NewServiceOrder(input.CustomerID, input.VehicleID, input.Description)
	if err != nil {
		logger.ErrorContext(ctx, "failed to build service order domain entity",
			"customer_id", input.CustomerID,
			"vehicle_id", input.VehicleID,
			"error", err,
		)
		span.RecordError(err)
		return nil, err
	}

	// Adicionar todos os itens à ordem (sem serviceOrderID ainda)
	for _, item := range itemsToAdd {
		if err := order.AddItem(item); err != nil {
			span.RecordError(err)
			return nil, err
		}
	}

	// Persistir ordem com itens em uma única transação
	// O repositório irá gerar o ID da ordem e atualizar os items
	if err := uc.serviceOrderRepo.SaveWithItems(ctx, order); err != nil {
		logger.ErrorContext(ctx, "failed to persist service order",
			"customer_id", input.CustomerID,
			"vehicle_id", input.VehicleID,
			"error", err,
		)
		span.RecordError(err)
		return nil, err
	}

	logger.InfoContext(ctx, "service order created",
		"service_order_id", order.ID(),
		"customer_id", order.CustomerID(),
		"vehicle_id", order.VehicleID(),
		"status", order.Status().String(),
		"item_count", len(order.Items()),
	)

	// Construir output com itens
	itemOutputs := make([]CreateServiceOrderItemOutput, 0, len(order.Items()))
	for _, item := range order.Items() {
		itemOutputs = append(itemOutputs, CreateServiceOrderItemOutput{
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

	// Record service order created metric
	if observability.ServiceOrderCreated != nil {
		observability.ServiceOrderCreated.Add(ctx, 1)
	}

	// Retornar output
	return &CreateServiceOrderOutput{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		Items:       itemOutputs,
		CreatedAt:   order.CreatedAt(),
		UpdatedAt:   order.UpdatedAt(),
	}, nil
}
