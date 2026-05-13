package dto

import (
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/utils"
)

// ItemInput represents an item to be added to a service order
type ItemInput struct {
	// Type of item (PRODUCT or SERVICE)
	ItemType string `json:"item_type" validate:"required,oneof=PRODUCT SERVICE" example:"SERVICE" enums:"PRODUCT,SERVICE"`
	// UUID of the product or service
	ReferenceID string `json:"reference_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Quantity of the item
	Quantity int `json:"quantity" validate:"required,min=1" example:"2"`
}

// CreateServiceOrderRequest represents the HTTP request for creating a service order
type CreateServiceOrderRequest struct {
	// Customer UUID who owns the service order
	CustomerID string `json:"customer_id" validate:"required,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Vehicle UUID being serviced
	VehicleID string `json:"vehicle_id" validate:"required,uuid" example:"660e8400-e29b-41d4-a716-446655440001"`
	// Description of the service order
	Description string `json:"description,omitempty" example:"Cliente relatou barulho no motor"`
	// Array of items (products and/or services) to include in the order
	Items []ItemInput `json:"items,omitempty" validate:"dive"`
}

// UpdateServiceOrderRequest represents the HTTP request for updating a service order
// All fields are optional to allow partial updates
type UpdateServiceOrderRequest struct {
	// Customer UUID (optional for update)
	CustomerID *string `json:"customer_id,omitempty" validate:"omitempty,uuid" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Vehicle UUID (optional for update)
	VehicleID *string `json:"vehicle_id,omitempty" validate:"omitempty,uuid" example:"660e8400-e29b-41d4-a716-446655440001"`
	// Description of the service order (optional for update)
	Description *string `json:"description,omitempty" example:"Cliente relatou barulho no motor"`
	// Order status (optional for update)
	Status *string `json:"status,omitempty" example:"IN_PROGRESS" enums:"RECEIVED,DIAGNOSING,PENDING_AUTHORIZATION,AUTHORIZED,IN_PROGRESS,COMPLETED,AWAITING_PAYMENT,PAID,DELIVERED,CANCELED,AUTHORIZATION_DENIED"`
	// Array of items to replace existing items (optional)
	Items []ItemInput `json:"items,omitempty" validate:"omitempty,dive"`
}

type AuthorizeServiceOrderRequest struct {
	Approved bool    `json:"approved"`
	Notes    *string `json:"notes,omitempty"`
}

// ServiceOrderItemResponse represents the HTTP response for a service order item
type ServiceOrderItemResponse struct {
	// Item unique identifier
	ID string `json:"id" example:"770e8400-e29b-41d4-a716-446655440002"`
	// Type of item (PRODUCT or SERVICE)
	ItemType string `json:"item_type" example:"SERVICE" enums:"PRODUCT,SERVICE"`
	// UUID of the referenced product or service
	ReferenceID string `json:"reference_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Name of the product or service
	Name string `json:"name" example:"Troca de óleo"`
	// Description of the product or service
	Description string `json:"description,omitempty" example:"Troca de óleo do motor com filtro"`
	// Quantity of the item
	Quantity int `json:"quantity" example:"1"`
	// Unit price in cents (BRL)
	UnitPrice int `json:"unit_price" example:"15000"`
	// Subtotal in cents (quantity * unit_price)
	Subtotal int `json:"subtotal" example:"15000"`
	// Creation timestamp in ISO 8601 format
	CreatedAt string `json:"created_at" example:"2024-01-15T10:30:00Z"`
}

// CustomerResponse represents customer data in the service order response
type CustomerResponse struct {
	// Customer unique identifier
	ID string `json:"id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Customer full name
	Name string `json:"name" example:"João Silva"`
	// Customer email address
	Email string `json:"email" example:"joao.silva@example.com"`
	// Customer phone number in Brazilian format
	Phone string `json:"phone" example:"11987654321"`
}

// VehicleResponse represents vehicle data in the service order response
type VehicleResponse struct {
	// Vehicle unique identifier
	ID string `json:"id" example:"660e8400-e29b-41d4-a716-446655440001"`
	// Customer UUID who owns the vehicle
	CustomerID string `json:"customer_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Vehicle license plate in Brazilian format
	LicensePlate string `json:"license_plate" example:"ABC1D23"`
	// Vehicle brand
	Brand string `json:"brand" example:"Volkswagen"`
	// Vehicle model
	Model string `json:"model" example:"Gol"`
	// Model year
	ModelYear int `json:"model_year" example:"2023"`
	// Manufacture year
	ManufactureYear int `json:"manufacture_year" example:"2022"`
}

// ServiceOrderResponse represents the HTTP response for a service order
type ServiceOrderResponse struct {
	// Service order unique identifier
	ID string `json:"id" example:"880e8400-e29b-41d4-a716-446655440003"`
	// Customer UUID who owns the service order
	CustomerID string `json:"customer_id" example:"550e8400-e29b-41d4-a716-446655440000"`
	// Vehicle UUID being serviced
	VehicleID string `json:"vehicle_id" example:"660e8400-e29b-41d4-a716-446655440001"`
	// Description of the service order
	Description string `json:"description,omitempty" example:"Cliente relatou barulho no motor"`
	// Customer details (included when fetching individual order)
	Customer *CustomerResponse `json:"customer,omitempty"`
	// Vehicle details (included when fetching individual order)
	Vehicle *VehicleResponse `json:"vehicle,omitempty"`
	// Current order status
	Status string `json:"status" example:"IN_PROGRESS" enums:"RECEIVED,DIAGNOSING,PENDING_AUTHORIZATION,AUTHORIZED,IN_PROGRESS,COMPLETED,AWAITING_PAYMENT,PAID,DELIVERED,CANCELED,AUTHORIZATION_DENIED"`
	// Current saga status
	SagaStatus string `json:"saga_status" example:"IDLE" enums:"IDLE,AWAITING_INVENTORY,AWAITING_PAYMENT,FAILED"`
	// Array of items (products and services) in the order
	Items []ServiceOrderItemResponse `json:"items"`
	// Total amount in cents (BRL) - sum of all item subtotals
	TotalAmount int `json:"total_amount" example:"35000"`
	// Timestamp when order was closed (only for final statuses)
	ClosedAt *string `json:"closed_at,omitempty" example:"2024-01-20T15:45:00Z"`
	// Creation timestamp in ISO 8601 format
	CreatedAt string `json:"created_at" example:"2024-01-15T10:30:00Z"`
	// Last update timestamp in ISO 8601 format
	UpdatedAt string `json:"updated_at" example:"2024-01-15T14:20:00Z"`
}

// ToServiceOrderResponse converts a domain ServiceOrder entity to ServiceOrderResponse DTO
func ToServiceOrderResponse(order *service_order.ServiceOrder) ServiceOrderResponse {
	// Converter items
	items := make([]ServiceOrderItemResponse, 0, len(order.Items()))
	for _, item := range order.Items() {
		// Pular items deletados
		if !item.IsDeleted() {
			items = append(items, ServiceOrderItemResponse{
				ID:          item.ID(),
				ItemType:    string(item.ItemType()),
				ReferenceID: item.ReferenceID(),
				Name:        item.Name(),
				Quantity:    item.Quantity(),
				UnitPrice:   item.UnitPrice(),
				Subtotal:    item.Subtotal(),
				CreatedAt:   utils.FormatTimeRFC3339(item.CreatedAt()),
			})
		}
	}

	response := ServiceOrderResponse{
		ID:          order.ID(),
		CustomerID:  order.CustomerID(),
		VehicleID:   order.VehicleID(),
		Description: order.Description(),
		Status:      order.Status().String(),
		SagaStatus:  order.SagaStatus(),
		Items:       items,
		TotalAmount: order.TotalAmount(),
		CreatedAt:   utils.FormatTimeRFC3339(order.CreatedAt()),
		UpdatedAt:   utils.FormatTimeRFC3339(order.UpdatedAt()),
	}

	// Format ClosedAt if it exists
	if order.ClosedAt() != nil {
		closedAt := utils.FormatTimeRFC3339(*order.ClosedAt())
		response.ClosedAt = &closedAt
	}

	return response
}

// ToServiceOrderResponseList converts a slice of domain ServiceOrder entities to a slice of ServiceOrderResponse DTOs
func ToServiceOrderResponseList(orders []*service_order.ServiceOrder) []ServiceOrderResponse {
	responses := make([]ServiceOrderResponse, len(orders))
	for i, order := range orders {
		responses[i] = ToServiceOrderResponse(order)
	}
	return responses
}

// HistoryResponse represents the HTTP response for a service order history record
type HistoryResponse struct {
	// History record unique identifier
	ID string `json:"id" example:"990e8400-e29b-41d4-a716-446655440004"`
	// Service order UUID this history belongs to
	ServiceOrderID string `json:"service_order_id" example:"880e8400-e29b-41d4-a716-446655440003"`
	// Additional metadata about the status change (key-value pairs)
	Metadata map[string]interface{} `json:"metadata"`
	// Status at the time of this history record
	Status string `json:"status" example:"AUTHORIZED" enums:"RECEIVED,DIAGNOSING,PENDING_AUTHORIZATION,AUTHORIZED,IN_PROGRESS,COMPLETED,AWAITING_PAYMENT,PAID,DELIVERED,CANCELED,AUTHORIZATION_DENIED"`
	// Timestamp when this status change occurred in ISO 8601 format
	CreatedAt string `json:"created_at" example:"2024-01-15T11:00:00Z"`
}

// ToHistoryResponse converts a History entity to HistoryResponse DTO
func ToHistoryResponse(history *service_order.History) HistoryResponse {
	return HistoryResponse{
		ID:             history.ID(),
		ServiceOrderID: history.ServiceOrderID(),
		Metadata:       history.Metadata(),
		Status:         history.Status().String(),
		CreatedAt:      utils.FormatTimeRFC3339(history.CreatedAt()),
	}
}

// ToHistoryResponseList converts a slice of History entities to a slice of HistoryResponse DTOs
func ToHistoryResponseList(histories []*service_order.History) []HistoryResponse {
	responses := make([]HistoryResponse, len(histories))
	for i, history := range histories {
		responses[i] = ToHistoryResponse(history)
	}
	return responses
}
