package usecases

import "time"

// ServiceOrderItemOutput represents the enriched item output with product/service details
type ServiceOrderItemOutput struct {
	ID          string
	ItemType    string
	ReferenceID string
	Name        string
	Description string
	Quantity    int
	UnitPrice   int
	Subtotal    int
	CreatedAt   time.Time
}
