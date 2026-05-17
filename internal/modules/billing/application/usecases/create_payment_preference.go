package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

// CreatePaymentPreference cria um Order na Orders API do Mercado Pago para
// uma OS que está avançando para AWAITING_PAYMENT.
// O nome do struct é mantido por compatibilidade de wiring; o campo no Module
// foi renomeado para CreatePaymentOrder.
type CreatePaymentPreference struct {
	client payment.MercadoPagoClient
}

func NewCreatePaymentPreference(client payment.MercadoPagoClient) *CreatePaymentPreference {
	return &CreatePaymentPreference{client: client}
}

// Execute cria o Order no MP e retorna o Order com RedirectURL para o cliente.
// PayerInfo é preenchido com o snapshot do customer persistido na OS (fase 5).
// Enquanto os métodos de snapshot não existem no domínio, PayerInfo usa campos vazios.
func (uc *CreatePaymentPreference) Execute(ctx context.Context, order *service_order.ServiceOrder) (*payment.Order, error) {
	items := BuildOrderItems(order)
	payer := buildPayerInfo(order)
	return uc.client.CreateOrder(ctx, items, payer, order.ID())
}

func BuildOrderItems(order *service_order.ServiceOrder) []payment.OrderItem {
	items := make([]payment.OrderItem, 0, len(order.Items()))
	for _, item := range order.Items() {
		if item.IsDeleted() {
			continue
		}
		items = append(items, payment.OrderItem{
			Title:     item.Name(),
			Quantity:  item.Quantity(),
			UnitPrice: float64(item.UnitPrice()) / 100,
		})
	}
	if len(items) == 0 {
		items = append(items, payment.OrderItem{
			Title:     "Ordem de Serviço " + order.ID(),
			Quantity:  1,
			UnitPrice: float64(order.TotalAmount()) / 100,
		})
	}
	return items
}

// buildPayerInfo constrói o PayerInfo a partir do snapshot do customer persistido na OS.
// CPF não está disponível no CustomerDTO atual — omitido (opcional no Checkout Pro com redirect).
func buildPayerInfo(order *service_order.ServiceOrder) payment.PayerInfo {
	return payment.PayerInfo{
		Email: order.CustomerEmail(),
		Name:  order.CustomerName(),
	}
}
