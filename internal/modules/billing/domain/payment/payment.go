package payment

import "context"

// OrderItem representa um item da OS enviado ao Mercado Pago.
type OrderItem struct {
	Title       string
	Description string
	Quantity    int
	UnitPrice   float64
}

// PayerInfo contém os dados do pagador usados na criação do Order no MP.
// Populados via snapshot na criação da OS — sem chamada adicional ao MS1 no momento do pagamento.
type PayerInfo struct {
	Email string
	CPF   string
	Name  string
}

// Order representa um Order criado na Orders API do Mercado Pago.
type Order struct {
	ID          string // mp_order_id (ex: "order-abc123")
	Status      string // created, processed, action_required, in_process, completed, cancelled
	RedirectURL string // URL de checkout para o cliente pagar (equivalente ao init_point)
	PaymentID   string // ID do primeiro payment interno ao order (se disponível)
}

// Payment representa o pagamento interno a um Order (transactions.payments[0]).
type Payment struct {
	ID            string
	Status        string // pending, approved, rejected, cancelled, refunded
	StatusDetail  string // motivo de rejeição (ex: cc_rejected_insufficient_amount)
	ExternalReference string
}

// MercadoPagoClient é o port para o adaptador do Mercado Pago (Orders API).
type MercadoPagoClient interface {
	// CreateOrder cria um novo Order na Orders API usando tipo "online" (Checkout Pro redirect).
	CreateOrder(ctx context.Context, items []OrderItem, payer PayerInfo, externalRef string) (*Order, error)

	// GetOrder busca o estado atual de um Order pelo ID do MP.
	GetOrder(ctx context.Context, mpOrderID string) (*Order, error)

	// CancelOrder cancela um Order que ainda não foi pago.
	// Retorna ErrOrderNotCancellable se o Order já estiver em estado final.
	CancelOrder(ctx context.Context, mpOrderID string) (*Order, error)

	// RefundOrder solicita estorno total (amount == nil) ou parcial de um Order já pago.
	// Retorna ErrOrderNotRefundable se o Order não estiver em estado pagável.
	RefundOrder(ctx context.Context, mpOrderID string, amount *string) (*Order, error)

	// GetPayment busca os dados de um pagamento interno pelo ID do payment.
	GetPayment(ctx context.Context, paymentID string) (*Payment, error)
}
