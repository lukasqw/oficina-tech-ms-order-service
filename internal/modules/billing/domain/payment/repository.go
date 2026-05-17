package payment

import "context"

// PaymentInfo expõe os campos de pagamento de uma OS para os usecases de billing.
type PaymentInfo struct {
	ServiceOrderID        string
	Status                string
	MPOrderID             string // antes: PreferenceID
	MPOrderStatus         string
	MPPaymentID           string
	PaymentURL            string
	PaymentRejectionReason string
	CustomerEmail         string
	CustomerCPF           string
	CustomerName          string
}

type Repository interface {
	GetPaymentInfo(ctx context.Context, serviceOrderID string) (*PaymentInfo, error)
}
