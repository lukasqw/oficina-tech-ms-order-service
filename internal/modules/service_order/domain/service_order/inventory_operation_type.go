package service_order

import "oficina-tech/internal/shared/dto"

const (
	// InventoryOpNone represents no inventory operation needed
	InventoryOpNone dto.StockOperationType = "NONE"
)

// InventoryOperation representa uma operação de inventário a ser executada
type InventoryOperation struct {
	Type dto.StockOperationType
}

// DetermineInventoryOperation determina qual operação de inventário deve ser executada
// baseada na transição de status. Esta é a lógica de negócio do domínio.
func DetermineInventoryOperation(oldStatus, newStatus OrderStatus) InventoryOperation {
	// Transição para PENDING_AUTHORIZATION: Reservar estoque
	if newStatus == StatusPendingAuthorization {
		return InventoryOperation{
			Type: dto.StockOpReserve,
		}
	}

	// Transição para COMPLETED: Baixar estoque reservado
	if newStatus == StatusCompleted {
		return InventoryOperation{
			Type: dto.StockOpReservedDecrease,
		}
	}

	// Transição para CANCELED: Cancelar reserva ou baixa confirmada
	if newStatus == StatusCanceled {
		// Se estava em PENDING_AUTHORIZATION ou IN_PROGRESS, cancela reserva
		switch oldStatus {
		case StatusPendingAuthorization,
			StatusInProgress,
			StatusAuthorized,
			StatusDiagnosing,
			StatusReceived:
			return InventoryOperation{
				Type: dto.StockOpCancelReserved,
			}
		case StatusCompleted, StatusAwaitingPayment, StatusPaid:
			// Se estava após COMPLETED, cancela baixa confirmada
			return InventoryOperation{
				Type: dto.StockOpCancelConfirmed,
			}
		}
	}

	// Transição para AUTHORIZATION_DENIED: Cancelar reserva
	if newStatus == StatusAuthorizationDenied {
		return InventoryOperation{
			Type: dto.StockOpCancelReserved,
		}
	}

	// Nenhuma operação necessária
	return InventoryOperation{
		Type: InventoryOpNone,
	}
}
