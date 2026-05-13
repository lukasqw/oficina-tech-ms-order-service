package payment

import "context"

type PaymentInfo struct {
	ServiceOrderID string
	Status         string
	SagaStatus     string
	PreferenceID   string
	PaymentID      string
	PaymentURL     string
}

type Repository interface {
	GetPaymentInfo(ctx context.Context, serviceOrderID string) (*PaymentInfo, error)
}
