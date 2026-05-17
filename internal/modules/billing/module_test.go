package billing

import (
	"context"
	"testing"

	"oficina-tech/internal/modules/billing/domain/payment"
)

func TestNewModuleWiresUseCases(t *testing.T) {
	module := NewModule(nil, nil, nil, nil, &moduleMPClient{})
	if module.MercadoPagoClient == nil ||
		module.SignatureValidator == nil ||
		module.CreatePaymentOrder == nil ||
		module.HandlePaymentWebhook == nil ||
		module.GetPaymentStatus == nil {
		t.Fatalf("module dependencies were not wired: %+v", module)
	}
}

func TestNewModuleCreatesDefaultClientWhenNil(t *testing.T) {
	module := NewModule(nil, nil, nil, nil, nil)
	if module.MercadoPagoClient == nil {
		t.Fatalf("expected default Mercado Pago client")
	}
}

// moduleMPClient é um stub mínimo que satisfaz a interface payment.MercadoPagoClient.
type moduleMPClient struct{}

func (c *moduleMPClient) CreateOrder(_ context.Context, _ []payment.OrderItem, _ payment.PayerInfo, _ string) (*payment.Order, error) {
	return nil, nil
}
func (c *moduleMPClient) GetOrder(_ context.Context, _ string) (*payment.Order, error) {
	return nil, nil
}
func (c *moduleMPClient) CancelOrder(_ context.Context, _ string) (*payment.Order, error) {
	return nil, nil
}
func (c *moduleMPClient) RefundOrder(_ context.Context, _ string, _ *string) (*payment.Order, error) {
	return nil, nil
}
func (c *moduleMPClient) GetPayment(_ context.Context, _ string) (*payment.Payment, error) {
	return nil, nil
}
