package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

type GetPaymentStatusOutput struct {
	PaymentURL string
	OrderID    string // MP Order ID (antes: PreferenceID)
	Status     string
}

type GetPaymentStatus struct {
	repo service_order.Repository
}

func NewGetPaymentStatus(repo service_order.Repository) *GetPaymentStatus {
	return &GetPaymentStatus{repo: repo}
}

func (uc *GetPaymentStatus) Execute(ctx context.Context, serviceOrderID string) (*GetPaymentStatusOutput, error) {
	order, err := uc.repo.FindByID(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status() != service_order.StatusAwaitingPayment || order.SagaStatus() != service_order.SagaStatusAwaitingPayment {
		return nil, payment.ErrPaymentURLNotAvailable
	}
	if order.PaymentURL() == nil || order.MPPreferenceID() == nil {
		return nil, payment.ErrPaymentURLNotAvailable
	}
	return &GetPaymentStatusOutput{
		PaymentURL: *order.PaymentURL(),
		OrderID:    *order.MPPreferenceID(), // MPPreferenceID contém o MP Order ID após a migration 003
		Status:     order.Status().String(),
	}, nil
}
