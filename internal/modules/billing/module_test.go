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

func TestNewModuleWiresRetryAndCancelAndRefund(t *testing.T) {
	module := NewModule(nil, nil, nil, nil, &moduleMPClient{})
	if module.RetryPayment == nil ||
		module.CancelPaymentOrder == nil ||
		module.RefundPaymentOrder == nil {
		t.Fatalf("retry/cancel/refund use cases not wired: %+v", module)
	}
}

func TestNewModuleWithAccessTokenCreatesSDKClient(t *testing.T) {
	t.Setenv("MP_ACCESS_TOKEN", "TEST-fake-token-for-testing")
	t.Setenv("MP_NOTIFICATION_URL", "https://example.com/webhook")
	t.Setenv("MP_CALLBACK_BASE_URL", "https://example.com")
	module := NewModule(nil, nil, nil, nil, nil)
	if module.MercadoPagoClient == nil {
		t.Fatalf("expected SDK client when MP_ACCESS_TOKEN is set")
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
