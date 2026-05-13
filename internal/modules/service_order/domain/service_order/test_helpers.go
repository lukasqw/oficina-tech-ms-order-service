package service_order

import (
	"time"

	"oficina-tech/internal/shared/dto"
)

// Test Helper Functions for ServiceOrder Domain

// NewTestServiceOrder creates a ServiceOrder for testing with default values
func NewTestServiceOrder(customerID, vehicleID string) *ServiceOrder {
	order, _ := NewServiceOrder(customerID, vehicleID, "")
	_ = order.SetID("test-order-id")
	return order
}

// NewTestServiceOrderWithID creates a ServiceOrder with a specific ID
func NewTestServiceOrderWithID(id, customerID, vehicleID string) *ServiceOrder {
	order, _ := NewServiceOrder(customerID, vehicleID, "")
	_ = order.SetID(id)
	return order
}

// NewTestServiceOrderWithStatus creates a ServiceOrder with a specific status
func NewTestServiceOrderWithStatus(customerID, vehicleID string, status OrderStatus) *ServiceOrder {
	order := NewTestServiceOrder(customerID, vehicleID)
	if status != StatusReceived {
		// Use ReconstructServiceOrder to bypass transition validation
		now := time.Now()
		var closedAt *time.Time
		var deletedAt *time.Time
		reconstructed, _ := ReconstructServiceOrder(
			order.ID(), customerID, vehicleID, "",
			status, "IDLE", nil, nil, nil, nil, nil, nil, []*ServiceOrderItem{}, closedAt,
			now, now, deletedAt,
		)
		return reconstructed
	}
	return order
}

// NewTestServiceOrderItem creates a ServiceOrderItem for testing with default values
func NewTestServiceOrderItem(itemType ItemType, refID string, quantity int) *ServiceOrderItem {
	item, _ := NewServiceOrderItem("", itemType, refID, "Test Item", quantity, 10000)
	_ = item.SetID("test-item-id")
	return item
}

// NewTestServiceOrderItemWithPrice creates a ServiceOrderItem with a specific price
func NewTestServiceOrderItemWithPrice(itemType ItemType, refID string, quantity int, unitPrice int) *ServiceOrderItem {
	item, _ := NewServiceOrderItem("", itemType, refID, "Test Item", quantity, unitPrice)
	_ = item.SetID("test-item-id")
	return item
}

// NewTestServiceOrderItemWithName creates a ServiceOrderItem with a specific name
func NewTestServiceOrderItemWithName(itemType ItemType, refID, name string, quantity int, unitPrice int) *ServiceOrderItem {
	item, _ := NewServiceOrderItem("", itemType, refID, name, quantity, unitPrice)
	_ = item.SetID("test-item-id")
	return item
}

// NewTestServiceOrderItemFull creates a ServiceOrderItem with all parameters
func NewTestServiceOrderItemFull(id, serviceOrderID string, itemType ItemType, refID, name string, quantity, unitPrice int) *ServiceOrderItem {
	item, _ := NewServiceOrderItem(serviceOrderID, itemType, refID, name, quantity, unitPrice)
	_ = item.SetID(id)
	return item
}

// NewTestHistory creates a History for testing with default values
func NewTestHistory(serviceOrderID string, status OrderStatus) *History {
	metadata := map[string]any{
		"status": map[string]string{
			"old": StatusReceived.String(),
			"new": status.String(),
		},
	}
	history, _ := NewHistory(serviceOrderID, metadata, status)
	_ = history.SetID("test-history-id")
	return history
}

// NewTestHistoryWithMetadata creates a History with specific metadata
func NewTestHistoryWithMetadata(serviceOrderID string, status OrderStatus, metadata map[string]any) *History {
	history, _ := NewHistory(serviceOrderID, metadata, status)
	_ = history.SetID("test-history-id")
	return history
}

// Test Helper Functions for DTOs

// NewTestCustomerDTO creates a CustomerDTO for testing
func NewTestCustomerDTO() *dto.CustomerDTO {
	return &dto.CustomerDTO{
		ID:    "customer-123",
		Name:  "Test Customer",
		Email: "test@example.com",
		Phone: "11999999999",
	}
}

// NewTestCustomerDTOWithID creates a CustomerDTO with a specific ID
func NewTestCustomerDTOWithID(id string) *dto.CustomerDTO {
	return &dto.CustomerDTO{
		ID:    id,
		Name:  "Test Customer",
		Email: "test@example.com",
		Phone: "11999999999",
	}
}

// NewTestVehicleDTO creates a VehicleDTO for testing
func NewTestVehicleDTO(customerID string) *dto.VehicleDTO {
	return &dto.VehicleDTO{
		ID:              "vehicle-456",
		CustomerID:      customerID,
		LicensePlate:    "ABC1234",
		Brand:           "Toyota",
		Model:           "Corolla",
		ModelYear:       2023,
		ManufactureYear: 2022,
	}
}

// NewTestVehicleDTOWithID creates a VehicleDTO with specific IDs
func NewTestVehicleDTOWithID(id, customerID string) *dto.VehicleDTO {
	return &dto.VehicleDTO{
		ID:              id,
		CustomerID:      customerID,
		LicensePlate:    "ABC1234",
		Brand:           "Toyota",
		Model:           "Corolla",
		ModelYear:       2023,
		ManufactureYear: 2022,
	}
}

// NewTestProductDTO creates a ProductDTO for testing
func NewTestProductDTO() *dto.ProductDTO {
	return &dto.ProductDTO{
		ID:          "product-789",
		Name:        "Óleo de Motor",
		Description: "Óleo sintético 5W30",
		Price:       5000, // R$ 50,00
		ProductType: "PRODUCT",
	}
}

// NewTestProductDTOWithID creates a ProductDTO with a specific ID
func NewTestProductDTOWithID(id string) *dto.ProductDTO {
	return &dto.ProductDTO{
		ID:          id,
		Name:        "Óleo de Motor",
		Description: "Óleo sintético 5W30",
		Price:       5000,
		ProductType: "PRODUCT",
	}
}

// NewTestProductDTOWithPrice creates a ProductDTO with a specific price
func NewTestProductDTOWithPrice(id string, price int) *dto.ProductDTO {
	return &dto.ProductDTO{
		ID:          id,
		Name:        "Óleo de Motor",
		Description: "Óleo sintético 5W30",
		Price:       price,
		ProductType: "PRODUCT",
	}
}

// NewTestServiceDTO creates a ServiceDTO for testing
func NewTestServiceDTO() *dto.ServiceDTO {
	return &dto.ServiceDTO{
		ID:          "service-123",
		Name:        "Troca de Óleo",
		Description: "Troca completa de óleo do motor",
		Price:       10000, // R$ 100,00
	}
}

// NewTestServiceDTOWithID creates a ServiceDTO with a specific ID
func NewTestServiceDTOWithID(id string) *dto.ServiceDTO {
	return &dto.ServiceDTO{
		ID:          id,
		Name:        "Troca de Óleo",
		Description: "Troca completa de óleo do motor",
		Price:       10000,
	}
}

// NewTestServiceDTOWithPrice creates a ServiceDTO with a specific price
func NewTestServiceDTOWithPrice(id string, price int) *dto.ServiceDTO {
	return &dto.ServiceDTO{
		ID:          id,
		Name:        "Troca de Óleo",
		Description: "Troca completa de óleo do motor",
		Price:       price,
	}
}

// Time Helper Functions

// FixedTime returns a fixed time for testing
func FixedTime() time.Time {
	return time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
}

// FixedTimePtr returns a pointer to a fixed time for testing
func FixedTimePtr() *time.Time {
	t := FixedTime()
	return &t
}

// TimeAfter returns a time after the fixed time by the given duration
func TimeAfter(d time.Duration) time.Time {
	return FixedTime().Add(d)
}

// TimeAfterPtr returns a pointer to a time after the fixed time
func TimeAfterPtr(d time.Duration) *time.Time {
	t := TimeAfter(d)
	return &t
}

// Assertion Helper Functions

// AssertServiceOrderEqual checks if two service orders have the same key fields
func AssertServiceOrderEqual(expected, actual *ServiceOrder) bool {
	if expected.ID() != actual.ID() {
		return false
	}
	if expected.CustomerID() != actual.CustomerID() {
		return false
	}
	if expected.VehicleID() != actual.VehicleID() {
		return false
	}
	if expected.Status() != actual.Status() {
		return false
	}
	return true
}

// AssertServiceOrderItemEqual checks if two service order items have the same key fields
func AssertServiceOrderItemEqual(expected, actual *ServiceOrderItem) bool {
	if expected.ItemType() != actual.ItemType() {
		return false
	}
	if expected.ReferenceID() != actual.ReferenceID() {
		return false
	}
	if expected.Name() != actual.Name() {
		return false
	}
	if expected.Quantity() != actual.Quantity() {
		return false
	}
	if expected.UnitPrice() != actual.UnitPrice() {
		return false
	}
	return true
}

// AssertHistoryEqual checks if two history records have the same key fields
func AssertHistoryEqual(expected, actual *History) bool {
	if expected.ServiceOrderID() != actual.ServiceOrderID() {
		return false
	}
	if expected.Status() != actual.Status() {
		return false
	}
	return true
}

// Test Data Builders

// ServiceOrderBuilder provides a fluent interface for building test ServiceOrders
type ServiceOrderBuilder struct {
	order *ServiceOrder
}

// NewServiceOrderBuilder creates a new builder with default values
func NewServiceOrderBuilder() *ServiceOrderBuilder {
	order := NewTestServiceOrder("customer-123", "vehicle-456")
	return &ServiceOrderBuilder{order: order}
}

// WithID sets the ID
func (b *ServiceOrderBuilder) WithID(id string) *ServiceOrderBuilder {
	_ = b.order.SetID(id)
	return b
}

// WithCustomerID sets the customer ID
func (b *ServiceOrderBuilder) WithCustomerID(customerID string) *ServiceOrderBuilder {
	_ = b.order.UpdateCustomer(customerID)
	return b
}

// WithVehicleID sets the vehicle ID
func (b *ServiceOrderBuilder) WithVehicleID(vehicleID string) *ServiceOrderBuilder {
	_ = b.order.UpdateVehicle(vehicleID)
	return b
}

// WithStatus sets the status
func (b *ServiceOrderBuilder) WithStatus(status OrderStatus) *ServiceOrderBuilder {
	_ = b.order.UpdateStatus(status)
	return b
}

// WithItem adds an item to the order
func (b *ServiceOrderBuilder) WithItem(item *ServiceOrderItem) *ServiceOrderBuilder {
	_ = b.order.AddItem(item)
	return b
}

// WithProductItem adds a product item with default values
func (b *ServiceOrderBuilder) WithProductItem(refID string, quantity int, unitPrice int) *ServiceOrderBuilder {
	item := NewTestServiceOrderItemWithPrice(ItemTypeProduct, refID, quantity, unitPrice)
	_ = b.order.AddItem(item)
	return b
}

// WithServiceItem adds a service item with default values
func (b *ServiceOrderBuilder) WithServiceItem(refID string, quantity int, unitPrice int) *ServiceOrderBuilder {
	item := NewTestServiceOrderItemWithPrice(ItemTypeService, refID, quantity, unitPrice)
	_ = b.order.AddItem(item)
	return b
}

// Build returns the built ServiceOrder
func (b *ServiceOrderBuilder) Build() *ServiceOrder {
	return b.order
}

// ServiceOrderItemBuilder provides a fluent interface for building test ServiceOrderItems
type ServiceOrderItemBuilder struct {
	item *ServiceOrderItem
}

// NewServiceOrderItemBuilder creates a new builder with default values
func NewServiceOrderItemBuilder() *ServiceOrderItemBuilder {
	item := NewTestServiceOrderItem(ItemTypeProduct, "ref-123", 1)
	return &ServiceOrderItemBuilder{item: item}
}

// WithID sets the ID
func (b *ServiceOrderItemBuilder) WithID(id string) *ServiceOrderItemBuilder {
	_ = b.item.SetID(id)
	return b
}

// WithServiceOrderID sets the service order ID
func (b *ServiceOrderItemBuilder) WithServiceOrderID(serviceOrderID string) *ServiceOrderItemBuilder {
	_ = b.item.SetServiceOrderID(serviceOrderID)
	return b
}

// WithItemType sets the item type
func (b *ServiceOrderItemBuilder) WithItemType(itemType ItemType) *ServiceOrderItemBuilder {
	// Need to recreate item since itemType is immutable
	newItem, _ := NewServiceOrderItem(
		b.item.ServiceOrderID(),
		itemType,
		b.item.ReferenceID(),
		b.item.Name(),
		b.item.Quantity(),
		b.item.UnitPrice(),
	)
	_ = newItem.SetID(b.item.ID())
	b.item = newItem
	return b
}

// WithReferenceID sets the reference ID
func (b *ServiceOrderItemBuilder) WithReferenceID(refID string) *ServiceOrderItemBuilder {
	// Need to recreate item since referenceID is immutable
	newItem, _ := NewServiceOrderItem(
		b.item.ServiceOrderID(),
		b.item.ItemType(),
		refID,
		b.item.Name(),
		b.item.Quantity(),
		b.item.UnitPrice(),
	)
	_ = newItem.SetID(b.item.ID())
	b.item = newItem
	return b
}

// WithName sets the name
func (b *ServiceOrderItemBuilder) WithName(name string) *ServiceOrderItemBuilder {
	// Need to recreate item since name is immutable
	newItem, _ := NewServiceOrderItem(
		b.item.ServiceOrderID(),
		b.item.ItemType(),
		b.item.ReferenceID(),
		name,
		b.item.Quantity(),
		b.item.UnitPrice(),
	)
	_ = newItem.SetID(b.item.ID())
	b.item = newItem
	return b
}

// WithQuantity sets the quantity
func (b *ServiceOrderItemBuilder) WithQuantity(quantity int) *ServiceOrderItemBuilder {
	_ = b.item.UpdateQuantity(quantity)
	return b
}

// WithUnitPrice sets the unit price
func (b *ServiceOrderItemBuilder) WithUnitPrice(unitPrice int) *ServiceOrderItemBuilder {
	// Need to recreate item since unitPrice is immutable
	newItem, _ := NewServiceOrderItem(
		b.item.ServiceOrderID(),
		b.item.ItemType(),
		b.item.ReferenceID(),
		b.item.Name(),
		b.item.Quantity(),
		unitPrice,
	)
	_ = newItem.SetID(b.item.ID())
	b.item = newItem
	return b
}

// Build returns the built ServiceOrderItem
func (b *ServiceOrderItemBuilder) Build() *ServiceOrderItem {
	return b.item
}

// NewTestServiceOrderWithItems creates a ServiceOrder with specific items
func NewTestServiceOrderWithItems(customerID, vehicleID string, status OrderStatus, items []*ServiceOrderItem) *ServiceOrder {
	order := NewTestServiceOrderWithStatus(customerID, vehicleID, status)
	for _, item := range items {
		_ = order.AddItem(item)
	}
	return order
}

// NewTestServiceOrderItemFull2 creates a ServiceOrderItem with all parameters for testing
func NewTestServiceOrderItemFull2(serviceOrderID string, itemType ItemType, refID, name string, quantity int, unitPrice int) *ServiceOrderItem {
	item, _ := NewServiceOrderItem(serviceOrderID, itemType, refID, name, quantity, unitPrice)
	_ = item.SetID("test-item-id-" + refID)
	return item
}

// NewTestServiceOrderItemDeleted creates a deleted ServiceOrderItem for testing
func NewTestServiceOrderItemDeleted(serviceOrderID string, itemType ItemType, refID, name string, quantity int, unitPrice int) *ServiceOrderItem {
	now := time.Now()
	deletedAt := &now
	historyID := ""
	item := ReconstructServiceOrderItem(
		"test-item-id-"+refID,
		serviceOrderID,
		&historyID,
		itemType,
		refID,
		name,
		quantity,
		unitPrice,
		now,
		now,
		deletedAt,
	)
	return item
}
