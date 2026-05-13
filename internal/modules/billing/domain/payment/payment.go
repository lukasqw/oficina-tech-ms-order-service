package payment

import "context"

type PreferenceItem struct {
	Title     string
	Quantity  int
	UnitPrice float64
}

type Preference struct {
	ID      string
	InitURL string
}

type Payment struct {
	ID                string
	Status            string
	ExternalReference string
}

type MercadoPagoClient interface {
	CreatePreference(ctx context.Context, orderID string, items []PreferenceItem, externalRef string) (*Preference, error)
	GetPayment(ctx context.Context, paymentID string) (*Payment, error)
}
