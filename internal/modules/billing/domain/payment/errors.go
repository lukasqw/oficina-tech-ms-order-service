package payment

import "errors"

var (
	ErrMissingAccessToken      = errors.New("MP_ACCESS_TOKEN não configurado")
	ErrMissingWebhookSecret    = errors.New("MP_WEBHOOK_SECRET não configurado")
	ErrInvalidWebhookSignature = errors.New("assinatura do Mercado Pago inválida")
	ErrMalformedWebhook        = errors.New("webhook do Mercado Pago malformado")
	ErrPaymentURLNotAvailable  = errors.New("URL de pagamento não encontrada")
	ErrOrderNotFound           = errors.New("order não encontrado no Mercado Pago")
	ErrOrderCreationFailed     = errors.New("falha ao criar order no Mercado Pago")
	ErrOrderNotCancellable     = errors.New("order não pode ser cancelado: já está em estado final")
	ErrOrderNotRefundable      = errors.New("order não pode ser estornado: pagamento não confirmado")
)
