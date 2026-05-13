package usecases

import (
	"context"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/email"
	"oficina-tech/internal/shared/infra/observability"
)

type HandlePaymentWebhookInput struct {
	PaymentID         string
	ExternalReference string
}

type HandlePaymentWebhookOutput struct {
	Processed bool
	Status    string
	OrderID   string
}

type HandlePaymentWebhook struct {
	client          payment.MercadoPagoClient
	orderRepo       service_order.Repository
	historyRepo     service_order.HistoryRepository
	customerAdapter adapters.CustomerAdapter
	emailService    email.EmailService
}

func NewHandlePaymentWebhook(
	client payment.MercadoPagoClient,
	orderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	customerAdapter adapters.CustomerAdapter,
	emailService email.EmailService,
) *HandlePaymentWebhook {
	return &HandlePaymentWebhook{
		client:          client,
		orderRepo:       orderRepo,
		historyRepo:     historyRepo,
		customerAdapter: customerAdapter,
		emailService:    emailService,
	}
}

func (uc *HandlePaymentWebhook) Execute(ctx context.Context, input HandlePaymentWebhookInput) (*HandlePaymentWebhookOutput, error) {
	mpPayment, err := uc.client.GetPayment(ctx, input.PaymentID)
	if err != nil {
		return nil, err
	}

	orderID := mpPayment.ExternalReference
	if orderID == "" {
		orderID = input.ExternalReference
	}
	if orderID == "" {
		return nil, payment.ErrMalformedWebhook
	}

	switch mpPayment.Status {
	case "approved":
		return uc.markApproved(ctx, orderID, mpPayment.ID)
	case "rejected", "cancelled", "canceled":
		return uc.markRejected(ctx, orderID, mpPayment.ID, mpPayment.Status)
	case "pending", "in_process":
		return &HandlePaymentWebhookOutput{Processed: false, Status: mpPayment.Status, OrderID: orderID}, nil
	default:
		return &HandlePaymentWebhookOutput{Processed: false, Status: mpPayment.Status, OrderID: orderID}, nil
	}
}

func (uc *HandlePaymentWebhook) markApproved(ctx context.Context, orderID, paymentID string) (*HandlePaymentWebhookOutput, error) {
	order, err := uc.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status() == service_order.StatusPaid {
		return &HandlePaymentWebhookOutput{Processed: false, Status: order.Status().String(), OrderID: order.ID()}, nil
	}
	if order.Status() != service_order.StatusAwaitingPayment {
		return &HandlePaymentWebhookOutput{Processed: false, Status: order.Status().String(), OrderID: order.ID()}, nil
	}

	oldStatus := order.Status()
	if err := order.ConfirmPayment(paymentID); err != nil {
		return nil, err
	}
	if err := uc.saveHistory(ctx, order, oldStatus, order.Status(), paymentID, "approved"); err != nil {
		return nil, err
	}
	if err := uc.orderRepo.Save(ctx, order); err != nil {
		return nil, err
	}
	uc.sendStatusUpdateEmail(ctx, order, oldStatus, order.Status())
	return &HandlePaymentWebhookOutput{Processed: true, Status: order.Status().String(), OrderID: order.ID()}, nil
}

func (uc *HandlePaymentWebhook) markRejected(ctx context.Context, orderID, paymentID, paymentStatus string) (*HandlePaymentWebhookOutput, error) {
	order, err := uc.orderRepo.FindByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status() == service_order.StatusCompleted {
		return &HandlePaymentWebhookOutput{Processed: false, Status: order.Status().String(), OrderID: order.ID()}, nil
	}
	if order.Status() != service_order.StatusAwaitingPayment {
		return &HandlePaymentWebhookOutput{Processed: false, Status: order.Status().String(), OrderID: order.ID()}, nil
	}

	oldStatus := order.Status()
	if err := order.RejectPayment(); err != nil {
		return nil, err
	}
	if err := uc.saveHistory(ctx, order, oldStatus, order.Status(), paymentID, paymentStatus); err != nil {
		return nil, err
	}
	if err := uc.orderRepo.Save(ctx, order); err != nil {
		return nil, err
	}
	uc.sendStatusUpdateEmail(ctx, order, oldStatus, order.Status())
	return &HandlePaymentWebhookOutput{Processed: true, Status: order.Status().String(), OrderID: order.ID()}, nil
}

func (uc *HandlePaymentWebhook) saveHistory(ctx context.Context, order *service_order.ServiceOrder, oldStatus, newStatus service_order.OrderStatus, paymentID, paymentStatus string) error {
	metadata := service_order.BuildStatusOnlyMetadata(oldStatus, newStatus)
	metadata["mp_payment_id"] = paymentID
	metadata["mp_payment_status"] = paymentStatus
	history, err := service_order.NewHistory(order.ID(), metadata, newStatus)
	if err != nil {
		return err
	}
	return uc.historyRepo.Save(ctx, history)
}

func (uc *HandlePaymentWebhook) sendStatusUpdateEmail(ctx context.Context, order *service_order.ServiceOrder, oldStatus, newStatus service_order.OrderStatus) {
	customer, err := uc.customerAdapter.GetCustomerByID(ctx, order.CustomerID())
	if err != nil {
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to load customer for payment email",
			"service_order_id", order.ID(),
			"customer_id", order.CustomerID(),
			"error", err,
		)
		return
	}

	if err := uc.emailService.SendStatusUpdateEmail(customer.Email, customer.Name, order.ID(), oldStatus.String(), newStatus.String()); err != nil {
		observability.LoggerFromContext(ctx).WarnContext(ctx, "failed to send payment email",
			"service_order_id", order.ID(),
			"error", err,
		)
	}
}
