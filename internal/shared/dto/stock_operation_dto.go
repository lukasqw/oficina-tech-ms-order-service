package dto

// StockOperationType represents the type of stock operation to be performed
type StockOperationType string

const (
	StockOpReserve           StockOperationType = "RESERVE"
	StockOpReservedDecrease  StockOperationType = "RESERVED_DECREASE"
	StockOpCancelReserved    StockOperationType = "CANCEL_RESERVED"
	StockOpCancelConfirmed   StockOperationType = "CANCEL_CONFIRMED"
	StockOpAvailableDecrease StockOperationType = "AVAILABLE_DECREASE"
	StockOpIncrease          StockOperationType = "INCREASE"
)

// StockOperationDTO represents a stock operation request
type StockOperationDTO struct {
	OperationType StockOperationType
	ProductID     string
	Quantity      int
	OrderID       string // Optional, for audit trail
}

// StockOperationResultDTO represents the result of a stock operation
type StockOperationResultDTO struct {
	Success bool
	Error   error
}
