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
	authMiddlewareChain := func(next http.Handler) http.Handler {
		return authMiddleware.Authenticate(
			rbacMiddleware.RequireRole("USER", "MANAGER", "ADMIN")(next))
	}

	mux.Handle("POST /payments/mp-webhook", http.HandlerFunc(webhookHandler.Handle))
	mux.Handle("GET /service-orders/{id}/payment", authMiddlewareChain(http.HandlerFunc(paymentHandler.GetServiceOrderPayment)))
}
