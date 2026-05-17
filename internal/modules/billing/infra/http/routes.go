package http

import (
	"net/http"

	"oficina-tech/internal/modules/billing/infra/http/handlers"
	"oficina-tech/internal/shared/infra/http/middleware"
)

func RegisterBillingRoutes(
	mux *http.ServeMux,
	webhookHandler *handlers.WebhookHandler,
	paymentHandler *handlers.PaymentHandler,
	authMiddleware *middleware.AuthMiddleware,
	rbacMiddleware *middleware.RBACMiddleware,
) {
	resultHandler := handlers.NewResultHandler()

	authMiddlewareChain := func(next http.Handler) http.Handler {
		return authMiddleware.Authenticate(
			rbacMiddleware.RequireRole("USER", "MANAGER", "ADMIN")(next))
	}

	// Webhook server-to-server do Mercado Pago (sem autenticação JWT)
	mux.Handle("POST /payments/mp-webhook", http.HandlerFunc(webhookHandler.Handle))

	// Página de redirect pós-pagamento (sem autenticação — destino público do MP)
	mux.Handle("GET /payments/result", http.HandlerFunc(resultHandler.Handle))

	// Consulta URL de pagamento ativa
	mux.Handle("GET /service-orders/{id}/payment", authMiddlewareChain(http.HandlerFunc(paymentHandler.GetServiceOrderPayment)))

	// Nova tentativa de pagamento após rejeição
	mux.Handle("POST /service-orders/{id}/retry-payment", authMiddlewareChain(http.HandlerFunc(paymentHandler.RetryPayment)))
}
