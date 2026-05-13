package service_order

import (
	"time"
)

// History representa um registro de histórico de mudanças em uma ordem de serviço
type History struct {
	id             string
	serviceOrderID string
	metadata       map[string]any
	status         OrderStatus
	createdAt      time.Time
}

// NewHistory cria um novo registro de histórico
// O ID será gerado pela camada de persistência
func NewHistory(serviceOrderID string, metadata map[string]any, status OrderStatus) (*History, error) {
	if serviceOrderID == "" {
		return nil, ErrInvalidServiceOrderID
	}

	return &History{
		serviceOrderID: serviceOrderID,
		metadata:       metadata,
		status:         status,
		createdAt:      time.Now(),
	}, nil
}

// ReconstructHistory reconstrói um histórico a partir de dados persistidos
func ReconstructHistory(
	id string,
	serviceOrderID string,
	metadata map[string]any,
	status OrderStatus,
	createdAt time.Time,
) *History {
	return &History{
		id:             id,
		serviceOrderID: serviceOrderID,
		metadata:       metadata,
		status:         status,
		createdAt:      createdAt,
	}
}

// Getters
func (h *History) ID() string               { return h.id }
func (h *History) ServiceOrderID() string   { return h.serviceOrderID }
func (h *History) Metadata() map[string]any { return h.metadata }
func (h *History) Status() OrderStatus      { return h.status }
func (h *History) CreatedAt() time.Time     { return h.createdAt }

// Setters
func (h *History) SetID(id string) error {
	if id == "" {
		return ErrInvalidHistoryID
	}
	h.id = id
	return nil
}

// BuildHistoryMetadata compares old and new service order states and builds simplified metadata
// Only stores the changes made, excluding items
func BuildHistoryMetadata(oldOrder, newOrder *ServiceOrder) map[string]any {
	changes := make(map[string]any)

	// Compare customer ID
	if oldOrder.CustomerID() != newOrder.CustomerID() {
		changes["customer_id"] = map[string]string{
			"old": oldOrder.CustomerID(),
			"new": newOrder.CustomerID(),
		}
	}

	// Compare vehicle ID
	if oldOrder.VehicleID() != newOrder.VehicleID() {
		changes["vehicle_id"] = map[string]string{
			"old": oldOrder.VehicleID(),
			"new": newOrder.VehicleID(),
		}
	}

	// Compare status
	if oldOrder.Status() != newOrder.Status() {
		changes["status"] = map[string]string{
			"old": oldOrder.Status().String(),
			"new": newOrder.Status().String(),
		}
	}

	return changes
}

// BuildHistoryMetadataWithItems compares old and new service order states
// Marks if items were changed, but doesn't calculate quantities
func BuildHistoryMetadataWithItems(oldOrder, newOrder *ServiceOrder) map[string]any {
	changes := BuildHistoryMetadata(oldOrder, newOrder)

	// Check if items were modified
	if hasItemsChanged(oldOrder.Items(), newOrder.Items()) {
		changes["items_changed"] = true
	}

	return changes
}

// hasItemsChanged checks if there were any changes to items by comparing content
// Since items are deleted and recreated, we compare by content (type, reference, quantity, price)
func hasItemsChanged(oldItems, newItems []*ServiceOrderItem) bool {
	// Filter only active items for comparison
	oldActiveItems := filterActiveItems(oldItems)
	newActiveItems := filterActiveItems(newItems)

	// Different number of active items
	if len(oldActiveItems) != len(newActiveItems) {
		return true
	}

	// Build content signatures for old items
	oldSignatures := make(map[string]bool)
	for _, item := range oldActiveItems {
		signature := buildItemSignature(item)
		oldSignatures[signature] = true
	}

	// Check if all new items have matching signatures
	for _, item := range newActiveItems {
		signature := buildItemSignature(item)
		if !oldSignatures[signature] {
			return true // Item content changed
		}
	}

	return false
}

// filterActiveItems returns only non-deleted items
func filterActiveItems(items []*ServiceOrderItem) []*ServiceOrderItem {
	active := make([]*ServiceOrderItem, 0)
	for _, item := range items {
		if !item.IsDeleted() {
			active = append(active, item)
		}
	}
	return active
}

// buildItemSignature creates a unique signature based on item content
func buildItemSignature(item *ServiceOrderItem) string {
	return string(item.ItemType()) + "|" +
		item.ReferenceID() + "|" +
		item.Name() + "|" +
		string(rune(item.Quantity())) + "|" +
		string(rune(item.UnitPrice()))
}

// BuildStatusOnlyMetadata creates metadata for status-only changes with old and new values
func BuildStatusOnlyMetadata(oldStatus, newStatus OrderStatus) map[string]any {
	return map[string]any{
		"status": map[string]string{
			"old": oldStatus.String(),
			"new": newStatus.String(),
		},
	}
}

// CaptureOrderState creates a snapshot of the current order state for comparison
// Captures order-level fields and items for detecting changes
func CaptureOrderState(order *ServiceOrder) *ServiceOrder {
	// Create a copy of items to preserve their state
	itemsCopy := make([]*ServiceOrderItem, len(order.Items()))
	for i, item := range order.Items() {
		itemsCopy[i] = ReconstructServiceOrderItem(
			item.ID(),
			item.ServiceOrderID(),
			item.HistoryID(),
			item.ItemType(),
			item.ReferenceID(),
			item.Name(),
			item.Quantity(),
			item.UnitPrice(),
			item.CreatedAt(),
			item.UpdatedAt(),
			item.DeletedAt(),
		)
	}

	// Reconstruct order with copied items
	orderCopy, _ := ReconstructServiceOrder(
		order.ID(),
		order.CustomerID(),
		order.VehicleID(),
		order.Description(),
		order.Status(),
		order.SagaStatus(),
		order.CurrentSagaID(),
		order.SagaTargetStatus(),
		order.SagaNotes(),
		order.MPPreferenceID(),
		order.MPPaymentID(),
		order.PaymentURL(),
		itemsCopy,
		order.ClosedAt(),
		order.CreatedAt(),
		order.UpdatedAt(),
		order.DeletedAt(),
	)

	return orderCopy
}
