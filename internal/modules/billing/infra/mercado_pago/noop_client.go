package mercado_pago

import (
	"context"
	"fmt"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// NoOpClient is used when MP_ACCESS_TOKEN is not configured.
// It simulates a successful MP response so the payment flow can be exercised
// without a real Mercado Pago account (dev/test environments).
type NoOpClient struct{}

func NewNoOpClient() *NoOpClient {
	return &NoOpClient{}
}

func (c *NoOpClient) CreatePreference(_ context.Context, orderID string, _ []payment.PreferenceItem, _ string) (*payment.Preference, error) {
	mockID := fmt.Sprintf("mock-%s", orderID)
	mockURL := fmt.Sprintf("https://sandbox.mercadopago.com.br/mock/checkout/%s", orderID)
	return &payment.Preference{
		ID:      mockID,
		InitURL: mockURL,
	}, nil
}

func (c *NoOpClient) GetPayment(_ context.Context, paymentID string) (*payment.Payment, error) {
	return &payment.Payment{
		ID:                paymentID,
		Status:            "pending",
		ExternalReference: "",
	}, nil
}
