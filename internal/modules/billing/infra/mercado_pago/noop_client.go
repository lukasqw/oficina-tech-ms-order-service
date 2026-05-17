package mercado_pago

import (
	"context"
	"fmt"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// NoOpClient é usado quando MP_ACCESS_TOKEN não está configurado.
// Simula respostas bem-sucedidas do Mercado Pago para que o fluxo
// possa ser exercitado sem uma conta real (dev/teste local).
type NoOpClient struct{}

func NewNoOpClient() *NoOpClient {
	return &NoOpClient{}
}

func (c *NoOpClient) CreateOrder(_ context.Context, _ []payment.OrderItem, _ payment.PayerInfo, externalRef string) (*payment.Order, error) {
	mockID := fmt.Sprintf("mock-order-%s", externalRef)
	return &payment.Order{
		ID:          mockID,
		Status:      "created",
		RedirectURL: fmt.Sprintf("https://sandbox.mercadopago.com.br/mock/checkout/%s", mockID),
		PaymentID:   fmt.Sprintf("mock-pay-%s", externalRef),
	}, nil
}

func (c *NoOpClient) GetOrder(_ context.Context, mpOrderID string) (*payment.Order, error) {
	return &payment.Order{
		ID:        mpOrderID,
		Status:    "processed",
		PaymentID: fmt.Sprintf("mock-pay-%s", mpOrderID),
	}, nil
}

func (c *NoOpClient) CancelOrder(_ context.Context, mpOrderID string) (*payment.Order, error) {
	return &payment.Order{
		ID:     mpOrderID,
		Status: "cancelled",
	}, nil
}

func (c *NoOpClient) RefundOrder(_ context.Context, mpOrderID string, _ *string) (*payment.Order, error) {
	return &payment.Order{
		ID:     mpOrderID,
		Status: "refunded",
	}, nil
}

func (c *NoOpClient) GetPayment(_ context.Context, paymentID string) (*payment.Payment, error) {
	return &payment.Payment{
		ID:     paymentID,
		Status: "approved",
	}, nil
}
