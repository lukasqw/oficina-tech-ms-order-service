package http

import (
	"net/http"
	"oficina-tech/internal/modules/service_order/infra/http/handlers"
	"oficina-tech/internal/shared/infra/http/middleware"
)

// RegisterServiceOrderRoutes registers all service order-related routes with authentication and authorization
func RegisterServiceOrderRoutes(mux *http.ServeMux, handler *handlers.ServiceOrderHandler, authMiddleware *middleware.AuthMiddleware, rbacMiddleware *middleware.RBACMiddleware) {
	// Create a middleware chain for most service order routes
	// Most endpoints require authentication and USER, MANAGER, or ADMIN role
	authMiddlewareChain := func(next http.Handler) http.Handler {
		return authMiddleware.Authenticate(
			rbacMiddleware.RequireRole("USER", "MANAGER", "ADMIN")(next))
	}

	// Create a middleware chain for DELETE endpoint
	// DELETE requires MANAGER or ADMIN role only
	deleteMiddlewareChain := func(next http.Handler) http.Handler {
		return authMiddleware.Authenticate(
			rbacMiddleware.RequireRole("MANAGER", "ADMIN")(next))
	}

	// Customer and internal users can list service orders and authorize budgets
	customerOrInternalChain := func(next http.Handler) http.Handler {
		return authMiddleware.Authenticate(
			rbacMiddleware.RequireRole("CUSTOMER", "USER", "MANAGER", "ADMIN")(next))
	}

	// Register all service order routes
	mux.Handle("POST /service-orders", authMiddlewareChain(http.HandlerFunc(handler.CreateServiceOrder)))
	mux.Handle("GET /service-orders", customerOrInternalChain(http.HandlerFunc(handler.GetAllServiceOrders)))
	mux.Handle("GET /service-orders/{id}", authMiddlewareChain(http.HandlerFunc(handler.GetServiceOrder)))
	mux.Handle("GET /service-orders/{id}/history", authMiddlewareChain(http.HandlerFunc(handler.GetServiceOrderHistory)))
	mux.Handle("PUT /service-orders/{id}", authMiddlewareChain(http.HandlerFunc(handler.UpdateServiceOrder)))
	mux.Handle("POST /service-orders/{id}/advance", authMiddlewareChain(http.HandlerFunc(handler.AdvanceServiceOrderStatus)))
	mux.Handle("POST /service-orders/{id}/authorize", customerOrInternalChain(http.HandlerFunc(handler.AuthorizeServiceOrder)))
	mux.Handle("DELETE /service-orders/{id}", deleteMiddlewareChain(http.HandlerFunc(handler.DeleteServiceOrder)))
}
