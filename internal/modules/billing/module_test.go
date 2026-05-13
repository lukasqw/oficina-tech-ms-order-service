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
		module.CreatePreference == nil ||
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

type moduleMPClient struct{}

func (c *moduleMPClient) CreatePreference(ctx context.Context, orderID string, items []payment.PreferenceItem, externalRef string) (*payment.Preference, error) {
	return nil, nil
}

func (c *moduleMPClient) GetPayment(ctx context.Context, paymentID string) (*payment.Payment, error) {
	return nil, nil
}
