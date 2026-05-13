package billing

import (
	"os"

	"oficina-tech/internal/modules/billing/application/usecases"
	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/billing/infra/mercado_pago"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/adapters"
	"oficina-tech/internal/shared/infra/email"
)

type Module struct {
	MercadoPagoClient    payment.MercadoPagoClient
	SignatureValidator   *mercado_pago.SignatureValidator
	CreatePreference     *usecases.CreatePaymentPreference
	HandlePaymentWebhook *usecases.HandlePaymentWebhook
	GetPaymentStatus     *usecases.GetPaymentStatus
}

func NewModule(
	orderRepo service_order.Repository,
	historyRepo service_order.HistoryRepository,
	customerAdapter adapters.CustomerAdapter,
	emailService email.EmailService,
	client payment.MercadoPagoClient,
) *Module {
	if client == nil {
		client = mercado_pago.NewClientFromEnv()
	}
	return &Module{
		MercadoPagoClient:    client,
		SignatureValidator:   mercado_pago.NewSignatureValidator(os.Getenv("MP_WEBHOOK_SECRET")),
		CreatePreference:     usecases.NewCreatePaymentPreference(client),
		HandlePaymentWebhook: usecases.NewHandlePaymentWebhook(client, orderRepo, historyRepo, customerAdapter, emailService),
		GetPaymentStatus:     usecases.NewGetPaymentStatus(orderRepo),
	}
}
