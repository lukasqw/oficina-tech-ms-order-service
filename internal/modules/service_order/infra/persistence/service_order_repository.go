package persistence

import (
	"context"
	"errors"
	"fmt"
	"oficina-tech/internal/modules/service_order/domain/service_order"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type ServiceOrderRepositoryImpl struct {
	db *gorm.DB
}

func NewServiceOrderRepository(db *gorm.DB) service_order.Repository {
	return &ServiceOrderRepositoryImpl{db: db}
}

func (r *ServiceOrderRepositoryImpl) Save(ctx context.Context, order *service_order.ServiceOrder) error {
	return r.SaveWithItems(ctx, order)
}

func (r *ServiceOrderRepositoryImpl) SaveWithItems(ctx context.Context, order *service_order.ServiceOrder) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		model, err := FromDomain(order)
		if err != nil {
			return err
		}

		// If ID is empty, create new record; otherwise update
		if order.ID() == "" {
			// Generate UUID v7
			model.ID = uuid.Must(uuid.NewV7())
			if err := tx.Create(model).Error; err != nil {
				return err
			}
			if err := order.SetID(model.ID.String()); err != nil {
				return err
			}

			// Update serviceOrderID for all items in the domain entity
			for _, item := range order.Items() {
				_ = item.SetServiceOrderID(model.ID.String())
			}
		} else {
			if err := tx.Save(model).Error; err != nil {
				return err
			}
		}

		// Handle items
		// Load existing items to identify which ones to soft delete
		var existingItems []ServiceOrderItemModel
		if err := tx.Unscoped().Where("service_order_id = ?", model.ID).Find(&existingItems).Error; err != nil {
			return err
		}

		// Create a map of current item IDs for quick lookup
		currentItemIDs := make(map[string]bool)
		for _, item := range order.Items() {
			if item.ID() != "" {
				currentItemIDs[item.ID()] = true
			}
		}

		// Soft delete items that are no longer in the order
		// Preserve history_id for items that already have it
		for _, existingItem := range existingItems {
			itemID := existingItem.ID.String()
			if !currentItemIDs[itemID] && !existingItem.DeletedAt.Valid {
				// This item needs to be soft deleted
				// Use Delete which will set deleted_at while preserving history_id
				if err := tx.Delete(&existingItem).Error; err != nil {
					return err
				}
			}
		}

		// Then create/update all current items
		for _, item := range order.Items() {
			itemModel, err := FromDomainItem(item)
			if err != nil {
				return err
			}

			// If item has no ID, generate one
			if item.ID() == "" {
				itemModel.ID = uuid.Must(uuid.NewV7())
				if err := item.SetID(itemModel.ID.String()); err != nil {
					return err
				}
			}

			// Set the service order ID
			itemModel.ServiceOrderID = model.ID

			// Create or update the item
			if err := tx.Save(itemModel).Error; err != nil {
				return err
			}
		}

		return nil
	})
}

func (r *ServiceOrderRepositoryImpl) FindByID(ctx context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByIDWithItems(ctx, id)
}

func (r *ServiceOrderRepositoryImpl) FindByIDWithItems(ctx context.Context, id string) (*service_order.ServiceOrder, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("invalid UUID: %w", err)
	}

	var model ServiceOrderModel
	if err := r.db.WithContext(ctx).Preload("Items").First(&model, "id = ?", uid).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, service_order.ErrServiceOrderNotFound
		}
		return nil, err
	}

	return model.ToDomainWithItems()
}

func (r *ServiceOrderRepositoryImpl) FindAll(ctx context.Context) ([]*service_order.ServiceOrder, error) {
	var models []ServiceOrderModel
	if err := r.db.WithContext(ctx).Preload("Items").Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*service_order.ServiceOrder, 0, len(models))
	for _, model := range models {
		order, err := model.ToDomainWithItems()
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (r *ServiceOrderRepositoryImpl) FindAllWithFilters(ctx context.Context, filters service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	query := r.db.WithContext(ctx).Preload("Items")

	// Aplicar filtro de CustomerID
	if filters.CustomerID != nil && *filters.CustomerID != "" {
		uid, err := uuid.Parse(*filters.CustomerID)
		if err != nil {
			return nil, fmt.Errorf("invalid customer UUID: %w", err)
		}
		query = query.Where("customer_id = ?", uid)
	}

	// Aplicar filtro de Status
	if filters.Status != nil {
		query = query.Where("status = ?", filters.Status.String())
	}

	// Aplicar filtro para ocultar status finalizados
	if filters.HideCompleted {
		query = query.Where("status NOT IN (?)", []string{"COMPLETED", "AWAITING_PAYMENT", "PAID", "DELIVERED", "AUTHORIZATION_DENIED"})
	}

	// Aplicar ordenação
	if filters.SortByStatus {
		// Ordenação customizada por status seguindo o fluxo do workflow
		// RECEIVED → DIAGNOSING → PENDING_AUTHORIZATION → AUTHORIZED → IN_PROGRESS → COMPLETED → AWAITING_PAYMENT → PAID → DELIVERED
		query = query.Order(`
			CASE status
				WHEN 'RECEIVED' THEN 1
				WHEN 'DIAGNOSING' THEN 2
				WHEN 'PENDING_AUTHORIZATION' THEN 3
				WHEN 'AUTHORIZED' THEN 4
				WHEN 'IN_PROGRESS' THEN 5
				WHEN 'COMPLETED' THEN 6
				WHEN 'AWAITING_PAYMENT' THEN 7
				WHEN 'PAID' THEN 8
				WHEN 'DELIVERED' THEN 9
				WHEN 'CANCELED' THEN 10
				WHEN 'AUTHORIZATION_DENIED' THEN 11
				ELSE 12
			END ASC
		`).Order("created_at ASC") // Mais antigas primeiro
	} else {
		query = query.Order("created_at DESC")
	}

	var models []ServiceOrderModel
	if err := query.Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*service_order.ServiceOrder, 0, len(models))
	for _, model := range models {
		order, err := model.ToDomainWithItems()
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (r *ServiceOrderRepositoryImpl) FindByCustomerID(ctx context.Context, customerID string) ([]*service_order.ServiceOrder, error) {
	uid, err := uuid.Parse(customerID)
	if err != nil {
		return nil, fmt.Errorf("invalid customer UUID: %w", err)
	}

	var models []ServiceOrderModel
	if err := r.db.WithContext(ctx).Preload("Items").Where("customer_id = ?", uid).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*service_order.ServiceOrder, 0, len(models))
	for _, model := range models {
		order, err := model.ToDomainWithItems()
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (r *ServiceOrderRepositoryImpl) FindByStatus(ctx context.Context, status service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	var models []ServiceOrderModel
	if err := r.db.WithContext(ctx).Preload("Items").Where("status = ?", status.String()).Order("created_at DESC").Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*service_order.ServiceOrder, 0, len(models))
	for _, model := range models {
		order, err := model.ToDomainWithItems()
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (r *ServiceOrderRepositoryImpl) FindBySagaStatus(ctx context.Context, sagaStatus string) ([]*service_order.ServiceOrder, error) {
	var models []ServiceOrderModel
	if err := r.db.WithContext(ctx).Preload("Items").Where("saga_status = ?", sagaStatus).Order("updated_at ASC").Find(&models).Error; err != nil {
		return nil, err
	}

	orders := make([]*service_order.ServiceOrder, 0, len(models))
	for _, model := range models {
		order, err := model.ToDomainWithItems()
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

func (r *ServiceOrderRepositoryImpl) Delete(ctx context.Context, id string) error {
	uid, err := uuid.Parse(id)
	if err != nil {
		return fmt.Errorf("invalid UUID: %w", err)
	}

	var model ServiceOrderModel

	// First check if the record exists (GORM automatically filters deleted_at IS NULL)
	result := r.db.WithContext(ctx).First(&model, "id = ?", uid)
	if result.Error != nil {
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return service_order.ErrServiceOrderNotFound
		}
		return result.Error
	}

	// Perform soft delete (GORM sets deleted_at automatically)
	if err := r.db.WithContext(ctx).Delete(&model).Error; err != nil {
		return err
	}

	return nil
}

// UpdateItemsHistoryID updates the history_id for specific items
// This is used to associate deleted items with a history record
func (r *ServiceOrderRepositoryImpl) UpdateItemsHistoryID(ctx context.Context, itemIDs []string, historyID string) error {
	if len(itemIDs) == 0 {
		return nil
	}

	historyUUID, err := uuid.Parse(historyID)
	if err != nil {
		return fmt.Errorf("invalid history UUID: %w", err)
	}

	// Convert item IDs to UUIDs
	itemUUIDs := make([]uuid.UUID, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		itemUUID, err := uuid.Parse(itemID)
		if err != nil {
			return fmt.Errorf("invalid item UUID %s: %w", itemID, err)
		}
		itemUUIDs = append(itemUUIDs, itemUUID)
	}

	// Update history_id for the specified items
	// Use Unscoped to include soft-deleted items
	result := r.db.WithContext(ctx).Unscoped().
		Model(&ServiceOrderItemModel{}).
		Where("id IN ?", itemUUIDs).
		Update("history_id", historyUUID)

	if result.Error != nil {
		return result.Error
	}

	return nil
}
