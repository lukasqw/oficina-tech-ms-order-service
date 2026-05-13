package payment

import "errors"

var (
	ErrMissingAccessToken      = errors.New("MP_ACCESS_TOKEN não configurado")
	ErrMissingWebhookSecret    = errors.New("MP_WEBHOOK_SECRET não configurado")
	ErrInvalidWebhookSignature = errors.New("assinatura do Mercado Pago inválida")
	ErrMalformedWebhook        = errors.New("webhook do Mercado Pago malformado")
	ErrPaymentURLNotAvailable  = errors.New("URL de pagamento não encontrada")
)
