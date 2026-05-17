package http

import (
	"net/http"
	"testing"

	jwtAuth "oficina-tech/internal/modules/access_control/infra/auth"
	"oficina-tech/internal/modules/billing/infra/http/handlers"
	"oficina-tech/internal/shared/infra/http/middleware"
)

func TestRegisterBillingRoutes(t *testing.T) {
	mux := http.NewServeMux()
	RegisterBillingRoutes(
		mux,
		handlers.NewWebhookHandler(nil, nil),
		handlers.NewPaymentHandler(nil, nil),
		middleware.NewAuthMiddleware(jwtAuth.NewJWTService()),
		middleware.NewRBACMiddleware(),
	)
}
