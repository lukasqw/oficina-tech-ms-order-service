package service_order

type OrderStatus string

const (
	StatusReceived             OrderStatus = "RECEIVED"
	StatusDiagnosing           OrderStatus = "DIAGNOSING"
	StatusPendingAuthorization OrderStatus = "PENDING_AUTHORIZATION"
	StatusAuthorized           OrderStatus = "AUTHORIZED"
	StatusInProgress           OrderStatus = "IN_PROGRESS"
	StatusCompleted            OrderStatus = "COMPLETED"
	StatusAwaitingPayment      OrderStatus = "AWAITING_PAYMENT"
	StatusPaymentRejected      OrderStatus = "PAYMENT_REJECTED"
	StatusPaid                 OrderStatus = "PAID"
	StatusDelivered            OrderStatus = "DELIVERED"
	StatusCanceled             OrderStatus = "CANCELED"
	StatusAuthorizationDenied  OrderStatus = "AUTHORIZATION_DENIED"
)

// NewOrderStatus valida e cria um novo OrderStatus
func NewOrderStatus(value string) (OrderStatus, error) {
	status := OrderStatus(value)
	if !status.IsValid() {
		return "", ErrInvalidStatus
	}
	return status, nil
}

// IsValid verifica se o status é válido
func (s OrderStatus) IsValid() bool {
	switch s {
	case StatusReceived, StatusDiagnosing, StatusPendingAuthorization,
		StatusAuthorized, StatusInProgress, StatusCompleted,
		StatusAwaitingPayment, StatusPaymentRejected, StatusPaid,
		StatusDelivered, StatusCanceled, StatusAuthorizationDenied:
		return true
	}
	return false
}

// String retorna a representação string do status
func (s OrderStatus) String() string {
	return string(s)
}

// CanTransitionTo valida se a transição para o novo status é permitida
func (s OrderStatus) CanTransitionTo(newStatus OrderStatus) bool {
	// Cancelamentos compensatórios são permitidos de qualquer estado não terminal,
	// exceto dos próprios terminais.
	if newStatus == StatusCanceled {
		return s != StatusDelivered && s != StatusCanceled && s != StatusAuthorizationDenied
	}

	// Status finais não podem transicionar para nenhum outro status.
	if s == StatusDelivered || s == StatusCanceled || s == StatusAuthorizationDenied {
		return false
	}

	// AUTHORIZATION_DENIED só pode ser alcançado de PENDING_AUTHORIZATION.
	if newStatus == StatusAuthorizationDenied {
		return s == StatusPendingAuthorization
	}

	// Mapa de transições válidas do fluxo normal.
	validTransitions := map[OrderStatus][]OrderStatus{
		StatusReceived: {
			StatusDiagnosing,
		},
		StatusDiagnosing: {
			StatusPendingAuthorization,
		},
		StatusPendingAuthorization: {
			StatusAuthorized,
		},
		StatusAuthorized: {
			StatusInProgress,
		},
		StatusInProgress: {
			StatusCompleted,
		},
		StatusCompleted: {
			StatusAwaitingPayment,
		},
		StatusAwaitingPayment: {
			StatusPaid,
			StatusPaymentRejected,
		},
		// PAYMENT_REJECTED: retry gera novo order MP e volta para AWAITING_PAYMENT;
		// cancelamento explícito (com refund) vai para CANCELED (tratado acima).
		StatusPaymentRejected: {
			StatusAwaitingPayment,
		},
		StatusPaid: {
			StatusDelivered,
		},
		StatusDelivered: {},
	}

	allowedTransitions, exists := validTransitions[s]
	if !exists {
		return false
	}

	for _, allowed := range allowedTransitions {
		if allowed == newStatus {
			return true
		}
	}

	return false
}

// NextStatus retorna o próximo status na sequência do fluxo
func (s OrderStatus) NextStatus() (OrderStatus, error) {
	switch s {
	case StatusReceived:
		return StatusDiagnosing, nil
	case StatusDiagnosing:
		return StatusPendingAuthorization, nil
	case StatusPendingAuthorization:
		return StatusAuthorized, nil
	case StatusAuthorized:
		return StatusInProgress, nil
	case StatusInProgress:
		return StatusCompleted, nil
	case StatusCompleted:
		return StatusAwaitingPayment, nil
	case StatusAwaitingPayment:
		return StatusPaid, nil
	case StatusPaid:
		return StatusDelivered, nil
	case StatusDelivered, StatusCanceled, StatusAuthorizationDenied, StatusPaymentRejected:
		return "", ErrNoNextStatus
	default:
		return "", ErrInvalidStatus
	}
}
