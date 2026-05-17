package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// CancelPaymentOrder cancela um Order no Mercado Pago que ainda não foi pago.
// Chamado internamente antes de mover a OS para CANCELED em AWAITING_PAYMENT ou PAYMENT_REJECTED.
// Se mpOrderID estiver vazio (OS nunca teve um Order criado), retorna sem erro.
type CancelPaymentOrder struct {
	client payment.MercadoPagoClient
}

func NewCancelPaymentOrder(client payment.MercadoPagoClient) *CancelPaymentOrder {
	return &CancelPaymentOrder{client: client}
}

// Execute cancela o Order no MP. Retorna ErrOrderNotCancellable se o Order já estiver em estado final.
// Falha no MP bloqueia o cancelamento da OS para evitar cobranças indevidas.
func (uc *CancelPaymentOrder) Execute(ctx context.Context, mpOrderID string) error {
	if mpOrderID == "" {
		return nil
	}
	_, err := uc.client.CancelOrder(ctx, mpOrderID)
	return err
}
