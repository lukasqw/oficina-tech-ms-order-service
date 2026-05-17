package usecases

import (
	"context"
	"time"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

// RetryPayment cria um novo Order no MP para uma OS em PAYMENT_REJECTED,
// permitindo que o cliente tente pagar novamente.
type RetryPayment struct {
	client      payment.MercadoPagoClient
	orderRepo   service_order.Repository
	historyRepo service_order.HistoryRepository
}

func NewRetryPayment(client payment.MercadoPagoClient, orderRepo service_order.Repository, historyRepo service_order.HistoryRepository) *RetryPayment {
	return &RetryPayment{client: client, orderRepo: orderRepo, historyRepo: historyRepo}
}

type RetryPaymentOutput struct {
	PaymentURL  string
	MPOrderID   string
}

func (uc *RetryPayment) Execute(ctx context.Context, serviceOrderID string) (*RetryPaymentOutput, error) {
	order, err := uc.orderRepo.FindByIDWithItems(ctx, serviceOrderID)
	if err != nil {
		return nil, err
	}
	if order.Status() != service_order.StatusPaymentRejected {
		return nil, service_order.ErrInvalidStatusTransition
	}

	items := BuildOrderItems(order)
	payer := buildPayerInfo(order)

	mpOrder, err := uc.client.CreateOrder(ctx, items, payer, order.ID())
	if err != nil {
		return nil, err
	}

	oldStatus := order.Status()
	if err := order.AwaitPayment(mpOrder.ID, mpOrder.RedirectURL); err != nil {
		return nil, err
	}

	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, order.Status())
	metadata["mp_order_id"] = mpOrder.ID
	metadata["payment_url"] = mpOrder.RedirectURL
	metadata["retry_at"] = time.Now().UTC().Format(time.RFC3339)
	history, err := service_order.NewHistory(order.ID(), metadata, order.Status())
	if err != nil {
		return nil, err
	}
	if err := uc.historyRepo.Save(ctx, history); err != nil {
		return nil, err
	}
	if err := uc.orderRepo.Save(ctx, order); err != nil {
		return nil, err
	}
	return &RetryPaymentOutput{PaymentURL: mpOrder.RedirectURL, MPOrderID: mpOrder.ID}, nil
}
