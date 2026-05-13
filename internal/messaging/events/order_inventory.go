package events

type InventoryItem struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

type OrderInventoryOperationRequested struct {
	Event      string          `json:"event"`
	SagaID     string          `json:"saga_id"`
	OrderID    string          `json:"order_id"`
	Operation  string          `json:"operation"`
	Items      []InventoryItem `json:"items"`
	OccurredAt string          `json:"occurred_at"`
}

type OrderInventoryOperationSucceeded struct {
	Event      string `json:"event"`
	SagaID     string `json:"saga_id"`
	OrderID    string `json:"order_id"`
	Operation  string `json:"operation"`
	OccurredAt string `json:"occurred_at"`
}

type OrderInventoryOperationFailed struct {
	Event      string `json:"event"`
	SagaID     string `json:"saga_id"`
	OrderID    string `json:"order_id"`
	Operation  string `json:"operation"`
	Reason     string `json:"reason"`
	OccurredAt string `json:"occurred_at"`
}

type CustomerDeleted struct {
	Event      string `json:"event"`
	CustomerID string `json:"customer_id"`
	Email      string `json:"email,omitempty"`
	OccurredAt string `json:"occurred_at"`
}

const (
	EventOrderInventoryOperationRequested = "OrderInventoryOperationRequested"
	EventOrderInventoryOperationSucceeded = "OrderInventoryOperationSucceeded"
	EventOrderInventoryOperationFailed    = "OrderInventoryOperationFailed"
	EventCustomerDeleted                  = "CustomerDeleted"
)
