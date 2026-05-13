package dto

import "time"

// ServiceOrderDTO is a shared data transfer object for service order information
// Used for inter-module communication
type ServiceOrderDTO struct {
	ID          string
	CustomerID  string
	VehicleID   string
	Status      string
	TotalAmount int
	ClosedAt    *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// HistoryDTO is a shared data transfer object for service order history
// Used for inter-module communication
type HistoryDTO struct {
	ID             string
	ServiceOrderID string
	Status         string
	Metadata       map[string]any
	CreatedAt      time.Time
}

// ServiceOrderItemDTO is a shared data transfer object for service order item information
// Used for inter-module communication
type ServiceOrderItemDTO struct {
	ID          string
	ItemType    string // "PRODUCT" or "SERVICE"
	ReferenceID string
	Name        string
	Quantity    int
	UnitPrice   int // in cents
	Subtotal    int // calculated: quantity * unitPrice
}
