package service_order

import "errors"

var (
	ErrInvalidServiceOrderID          = errors.New("ID da ordem de serviço inválido")
	ErrInvalidCustomerID              = errors.New("ID do cliente inválido")
	ErrInvalidVehicleID               = errors.New("ID do veículo inválido")
	ErrVehicleDoesNotBelongToCustomer = errors.New("veículo não pertence ao cliente da ordem de serviço")
	ErrInvalidStatus                  = errors.New("status inválido")
	ErrInvalidStatusTransition        = errors.New("transição de status inválida")
	ErrServiceOrderNotFound           = errors.New("ordem de serviço não encontrada")
	ErrServiceOrderDeleted            = errors.New("ordem de serviço foi deletada")
	ErrNoNextStatus                   = errors.New("não há próximo status disponível")
	ErrInvalidPaymentPreference       = errors.New("preferência de pagamento inválida")
	ErrInvalidPaymentID               = errors.New("ID de pagamento inválido")
	ErrInvalidPaymentURL              = errors.New("URL de pagamento inválida")

	// Item errors
	ErrInvalidItemID                 = errors.New("ID do item inválido")
	ErrInvalidItemType               = errors.New("tipo de item inválido")
	ErrInvalidReferenceID            = errors.New("ID de referência inválido")
	ErrInvalidItemName               = errors.New("nome do item inválido")
	ErrInvalidQuantity               = errors.New("quantidade deve ser maior que zero")
	ErrInvalidUnitPrice              = errors.New("preço unitário deve ser maior que zero")
	ErrItemNotFound                  = errors.New("item não encontrado na ordem")
	ErrProductNotFound               = errors.New("produto não encontrado")
	ErrServiceNotFound               = errors.New("serviço não encontrado")
	ErrCannotModifyClosedOrder       = errors.New("não é possível modificar ordem fechada")
	ErrCannotModifyItemsAfterPending = errors.New("não é possível modificar itens após status PENDING_AUTHORIZATION")

	// History errors
	ErrInvalidHistoryID      = errors.New("ID de histórico inválido")
	ErrHistoryNotFound       = errors.New("histórico não encontrado")
	ErrInvalidMetadata       = errors.New("metadata inválido")
	ErrHistoryCreationFailed = errors.New("falha ao criar histórico")

	ErrForbiddenAccess = errors.New("acesso não autorizado a esta ordem de serviço")

	// Inventory errors
	ErrProductInventoryNotFound  = errors.New("inventário do produto não encontrado")
	ErrInsufficientReservedStock = errors.New("estoque reservado insuficiente")
	ErrInventoryOperationFailed  = errors.New("operação de estoque falhou")
	ErrSagaNotImplemented        = errors.New("saga de estoque ainda não implementada")
	ErrInvalidSagaID             = errors.New("saga_id inválido")
	ErrSagaAlreadyInProgress     = errors.New("já existe saga em andamento para esta ordem de serviço")
)
