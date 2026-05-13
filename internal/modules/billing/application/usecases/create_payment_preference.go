package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

type CreatePaymentPreference struct {
	client payment.MercadoPagoClient
}

func NewCreatePaymentPreference(client payment.MercadoPagoClient) *CreatePaymentPreference {
	return &CreatePaymentPreference{client: client}
}

func (uc *CreatePaymentPreference) Execute(ctx context.Context, order *service_order.ServiceOrder) (*payment.Preference, error) {
	return uc.client.CreatePreference(ctx, order.ID(), BuildPreferenceItems(order), order.ID())
}

func BuildPreferenceItems(order *service_order.ServiceOrder) []payment.PreferenceItem {
	items := make([]payment.PreferenceItem, 0, len(order.Items()))
	for _, item := range order.Items() {
		if item.IsDeleted() {
			continue
		}
		items = append(items, payment.PreferenceItem{
			Title:     item.Name(),
			Quantity:  item.Quantity(),
			UnitPrice: float64(item.UnitPrice()) / 100,
		})
	}
	if len(items) == 0 {
		items = append(items, payment.PreferenceItem{
			Title:     "Ordem de Serviço " + order.ID(),
			Quantity:  1,
			UnitPrice: float64(order.TotalAmount()) / 100,
		})
	}
	return items
}
