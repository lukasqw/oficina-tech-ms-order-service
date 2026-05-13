package usecases

import (
	"context"
	"time"

	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/observability"
)

type UpdateServiceOrderInput struct {
	ID          string
	CustomerID  *string
	VehicleID   *string
	Description *string
	Status      *string
	Items       []ItemInput
}

type UpdateServiceOrderItemOutput struct {
	ID          string
	ItemType    string
	ReferenceID string
	Name        string
	Quantity    int
	UnitPrice   int
	Subtotal    int
	CreatedAt   time.Time
}

type UpdateServiceOrderOutput struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Description string
	Status      string
	SagaStatus  string
	Items       []UpdateServiceOrderItemOutput
	TotalAmount int
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type UpdateServiceOrder struct {
	serviceOrderRepo service_order.Repository
	historyRepo      service_order.HistoryRepository
	customerAdapter  adapters.CustomerAdapter
	vehicleAdapter   adapters.VehicleAdapter
	productAdapter   adapters.ProductAdapter
	serviceAdapter   adapters.ServiceAdapter
}

func NewUpdateServiceOrder(
	serviceOrderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	customerAdapter adapters.CustomerAdapter,
	vehicleAdapter adapters.VehicleAdapter,
	productAdapter adapters.ProductAdapter,
	serviceAdapter adapters.ServiceAdapter,
) *UpdateServiceOrder {
	return &UpdateServiceOrder{
		serviceOrderRepo: serviceOrderRepo,
		historyRepo:      historyRepo,
		customerAdapter:  customerAdapter,
		vehicleAdapter:   vehicleAdapter,
		productAdapter:   productAdapter,
		serviceAdapter:   serviceAdapter,
	}
}

func (uc *UpdateServiceOrder) Execute(ctx context.Context, input UpdateServiceOrderInput) (*UpdateServiceOrderOutput, error) {
	ctx, span := observability.SpanUseCase(ctx, "service_order.update")
	defer span.End()

	order, err := uc.serviceOrderRepo.FindByID(ctx, input.ID)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if order.IsDeleted() {
		span.RecordError(service_order.ErrServiceOrderDeleted)
		return nil, service_order.ErrServiceOrderDeleted
	}

	oldOrder := service_order.CaptureOrderState(order)

	if err := uc.validateAndUpdateCustomerVehicle(ctx, order, input); err != nil {
		span.RecordError(err)
		return nil, err
	}

	itemsModified, err := uc.updateItems(ctx, order, input.Items)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	if err := uc.saveHistory(ctx, order, oldOrder, itemsModified); err != nil {
		span.RecordError(err)
		return nil, err
	}

	if err := uc.saveOrder(ctx, order, itemsModified); err != nil {
		span.RecordError(err)
		return nil, err
	}

	return uc.buildOutput(order), nil
}

func (uc *UpdateServiceOrder) validateAndUpdateCustomerVehicle(ctx context.Context, order *service_order.ServiceOrder, input UpdateServiceOrderInput) error {
	finalCustomerID := order.CustomerID()
	finalVehicleID := order.VehicleID()

	if input.CustomerID != nil {
		finalCustomerID = *input.CustomerID
		if _, err := uc.customerAdapter.GetCustomerByID(ctx, *input.CustomerID); err != nil {
			return err
		}
	}

	if input.VehicleID != nil {
		finalVehicleID = *input.VehicleID
		if *input.VehicleID != "" {
			if _, err := uc.vehicleAdapter.GetVehicleByID(ctx, *input.VehicleID); err != nil {
				return err
			}
		}
	}

	if finalVehicleID != "" {
		isOwner, err := uc.vehicleAdapter.ValidateVehicleOwnership(ctx, finalVehicleID, finalCustomerID)
		if err != nil {
			return err
		}
		if !isOwner {
			return service_order.ErrVehicleDoesNotBelongToCustomer
		}
	}

	if input.CustomerID != nil {
		if err := order.UpdateCustomer(*input.CustomerID); err != nil {
			return err
		}
	}

	if input.VehicleID != nil {
		if err := order.UpdateVehicle(*input.VehicleID); err != nil {
			return err
		}
	}

	if input.Description != nil {
		order.UpdateDescription(*input.Description)
	}

	return nil
}

func (uc *UpdateServiceOrder) updateItems(ctx context.Context, order *service_order.ServiceOrder, items []ItemInput) (bool, error) {
	if items == nil {
		return false, nil
	}

	// Valida se pode modificar itens antes de processar
	if !order.CanModifyItems() {
		return false, service_order.ErrCannotModifyItemsAfterPending
	}

	itemsToAdd, err := uc.validateAndBuildItems(ctx, order.ID(), items)
	if err != nil {
		return false, err
	}

	for _, existingItem := range order.Items() {
		if !existingItem.IsDeleted() {
			if err := order.RemoveItem(existingItem.ID()); err != nil {
				return false, err
			}
		}
	}

	for _, item := range itemsToAdd {
		if err := order.AddItem(item); err != nil {
			return false, err
		}
	}

	return true, nil
}

func (uc *UpdateServiceOrder) validateAndBuildItems(ctx context.Context, orderID string, items []ItemInput) ([]*service_order.ServiceOrderItem, error) {
	var itemsToAdd []*service_order.ServiceOrderItem

	for _, itemInput := range items {
		itemType := service_order.ItemType(itemInput.ItemType)
		if itemType != service_order.ItemTypeProduct && itemType != service_order.ItemTypeService {
			return nil, service_order.ErrInvalidItemType
		}

		name, price, err := uc.fetchItemDetails(ctx, itemType, itemInput.ReferenceID)
		if err != nil {
			return nil, err
		}

		item, err := service_order.NewServiceOrderItem(
			orderID,
			itemType,
			itemInput.ReferenceID,
			name,
			itemInput.Quantity,
			price,
		)
		if err != nil {
			return nil, err
		}

		itemsToAdd = append(itemsToAdd, item)
	}

	return itemsToAdd, nil
}

func (uc *UpdateServiceOrder) fetchItemDetails(ctx context.Context, itemType service_order.ItemType, referenceID string) (string, int, error) {
	if itemType == service_order.ItemTypeProduct {
		product, err := uc.productAdapter.GetProductByID(ctx, referenceID)
		if err != nil {
			return "", 0, err
		}
		return product.Name, product.Price, nil
	}

	service, err := uc.serviceAdapter.GetServiceByID(ctx, referenceID)
	if err != nil {
		return "", 0, err
	}
	return service.Name, service.Price, nil
}

func (uc *UpdateServiceOrder) saveHistory(ctx context.Context, order *service_order.ServiceOrder, oldOrder *service_order.ServiceOrder, itemsModified bool) error {
	var metadata map[string]interface{}
	if itemsModified {
		metadata = service_order.BuildHistoryMetadataWithItems(oldOrder, order)
	} else {
		metadata = service_order.BuildHistoryMetadata(oldOrder, order)
	}

	history, err := service_order.NewHistory(order.ID(), metadata, order.Status())
	if err != nil {
		return err
	}

	if err := uc.historyRepo.Save(ctx, history); err != nil {
		return err
	}

	if itemsModified {
		for _, item := range order.Items() {
			if item.IsDeleted() && item.HistoryID() == nil {
				if err := item.SetHistoryID(history.ID()); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func (uc *UpdateServiceOrder) saveOrder(ctx context.Context, order *service_order.ServiceOrder, itemsModified bool) error {
	if itemsModified {
		return uc.serviceOrderRepo.SaveWithItems(ctx, order)
	}
	return uc.serviceOrderRepo.Save(ctx, order)
}

func (uc *UpdateServiceOrder) buildOutput(order *service_order.ServiceOrder) *UpdateServiceOrderOutput {
	itemOutputs := make([]UpdateServiceOrderItemOutput, 0, len(order.Items()))
	for _, item := range order.Items() {
		if !item.IsDeleted() {
			itemOutputs = append(itemOutputs, UpdateServiceOrderItemOutput{
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

	return &UpdateServiceOrderOutput{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		Items:       itemOutputs,
		TotalAmount: order.TotalAmount(),
		ClosedAt:    order.ClosedAt(),
		CreatedAt:   order.CreatedAt(),
		UpdatedAt:   order.UpdatedAt(),
	}
}
