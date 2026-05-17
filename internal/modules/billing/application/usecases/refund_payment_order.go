package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
)

// RefundPaymentOrder solicita estorno total de um Order já pago no Mercado Pago.
// Chamado internamente antes de mover a OS para CANCELED quando já está em PAID.
// Falha no MP bloqueia o cancelamento da OS para evitar divergência financeira.
type RefundPaymentOrder struct {
	client payment.MercadoPagoClient
}

func NewRefundPaymentOrder(client payment.MercadoPagoClient) *RefundPaymentOrder {
	return &RefundPaymentOrder{client: client}
}

// Execute solicita estorno total (amount nil) do Order. Retorna ErrOrderNotRefundable
// se o Order não estiver em estado pagável.
func (uc *RefundPaymentOrder) Execute(ctx context.Context, mpOrderID string) error {
	if mpOrderID == "" {
		return nil
	}
	_, err := uc.client.RefundOrder(ctx, mpOrderID, nil)
	return err
}
