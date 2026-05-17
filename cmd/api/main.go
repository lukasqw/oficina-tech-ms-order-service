package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oficina-tech/internal/messaging/consumers"
	"oficina-tech/internal/messaging/publishers"
	jwtAuth "oficina-tech/internal/modules/access_control/infra/auth"
	billingModule "oficina-tech/internal/modules/billing"
	billingHttp "oficina-tech/internal/modules/billing/infra/http"
	billingHandlers "oficina-tech/internal/modules/billing/infra/http/handlers"
	appsaga "oficina-tech/internal/modules/service_order/application/saga"
	serviceOrderUsecases "oficina-tech/internal/modules/service_order/application/usecases"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	serviceOrderAdapters "oficina-tech/internal/modules/service_order/infra/adapters/implementations"
	serviceOrderHttp "oficina-tech/internal/modules/service_order/infra/http"
	serviceOrderHandlers "oficina-tech/internal/modules/service_order/infra/http/handlers"
	serviceOrderPersistence "oficina-tech/internal/modules/service_order/infra/persistence"
	"oficina-tech/internal/shared/http_clients"
	"oficina-tech/internal/shared/infra/awsconfig"
	"oficina-tech/internal/shared/infra/database"
	shareddynamo "oficina-tech/internal/shared/infra/dynamodb"
	"oficina-tech/internal/shared/infra/email"
	"oficina-tech/internal/shared/infra/http/middleware"
	"oficina-tech/internal/shared/infra/observability"
	sqsinfra "oficina-tech/internal/shared/infra/sqs"
	"oficina-tech/internal/shared/utils"
)

const (
	queueOrderInventoryRequested = "order-inventory-op-requested"
	queueOrderInventorySucceeded = "order-inventory-op-succeeded"
	queueOrderInventoryFailed    = "order-inventory-op-failed"
	queueCustomerDeleted         = "customer-deleted"
)

type worker interface {
	Start(ctx context.Context) error
}

type serviceOrderRuntime struct {
	orchestrator *appsaga.Orchestrator
	orderRepo    service_order.Repository
	workers      []worker
}

var startTime = time.Now()

func main() {
	observability.NewLogger()

	// Initialize OTel first so metrics use the real exporter.
	// Non-fatal: if OTel is unavailable (e.g. no collector in dev) the service
	// continues with a no-op provider.
	startupCtx := context.Background()
	otelShutdown, err := observability.InitOTel(startupCtx)
	if err != nil {
		slog.Warn("OTel initialization failed, continuing without telemetry", "error", err)
	} else {
		defer func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := otelShutdown(shutdownCtx); err != nil {
				slog.Error("OTel shutdown error", "error", err)
			}
		}()
	}

	if err := observability.InitMetrics(observability.Meter()); err != nil {
		slog.Error("metrics initialization failed", "error", err)
	}

	// Register the health endpoint and start the HTTP server BEFORE the
	// heavyweight infrastructure initialization below. This ensures the
	// Docker / k8s health check can probe the service during startup and
	// avoids a false-unhealthy window caused by DynamoDB / SQS setup time.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler)

	handler := middleware.NewObservabilityMiddleware(middleware.WrapMux(mux))
	server := &http.Server{
		Addr:              ":" + getEnv("PORT", "8082"),
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		slog.Info("MS2 OS Service listening", "addr", server.Addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// ── Infrastructure initialization ────────────────────────────────────────
	database.Connect()

	awsCfg, err := awsconfig.Load(startupCtx)
	if err != nil {
		slog.Error("AWS config initialization failed", "error", err)
		os.Exit(1)
	}

	dynamoClient := shareddynamo.NewClient(awsCfg)
	if err := shareddynamo.EnsureOrderHistoryTable(startupCtx, dynamoClient); err != nil {
		slog.Error("DynamoDB initialization failed", "error", err)
		os.Exit(1)
	}

	sqsClient := sqsinfra.NewClient(awsCfg)
	queueURLs, err := sqsinfra.ResolveQueueURLs(
		startupCtx,
		sqsClient,
		queueOrderInventoryRequested,
		queueOrderInventorySucceeded,
		queueOrderInventoryFailed,
		queueCustomerDeleted,
	)
	if err != nil {
		slog.Error("SQS initialization failed", "error", err)
		os.Exit(1)
	}

	// ── Business logic routes ────────────────────────────────────────────────
	runtime := registerServiceOrderRoutes(mux, dynamoClient, sqsClient, queueURLs)
	if err := runtime.orchestrator.RecoverAwaitingSagas(startupCtx); err != nil {
		slog.Error("saga recovery failed", "error", err)
		os.Exit(1)
	}
	if err := logAwaitingPaymentOrders(startupCtx, runtime.orderRepo); err != nil {
		slog.Error("payment recovery inspection failed", "error", err)
		os.Exit(1)
	}

	// ── SQS workers ──────────────────────────────────────────────────────────
	workerCtx, cancelWorkers := context.WithCancel(context.Background())
	defer cancelWorkers()
	for _, w := range runtime.workers {
		go func(w worker) {
			if err := w.Start(workerCtx); err != nil && err != context.Canceled {
				slog.Error("SQS worker stopped", "error", err)
			}
		}(w)
	}

	slog.Info("MS2 OS Service fully initialized and ready")

	// ── Graceful shutdown ────────────────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	slog.Info("Shutting down MS2 OS Service...")
	cancelWorkers()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}
}

// healthHandler returns service health in the same JSON envelope format used
// by MS1 and MS3, so the test suite can parse it uniformly.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	version := getEnv("APP_VERSION", "dev")

	otelStatus := "inactive"
	if observability.OTelInitialized() {
		otelStatus = "active"
	}

	status := http.StatusOK
	payload := map[string]string{
		"status":   "healthy",
		"database": "connected",
		"version":  version,
		"uptime":   time.Since(startTime).String(),
		"otel":     otelStatus,
	}
	if database.DB == nil {
		status = http.StatusServiceUnavailable
		payload["status"] = "unhealthy"
		payload["database"] = "not initialized"
	}

	utils.RespondSuccess(w, status, payload)
}

func registerServiceOrderRoutes(mux *http.ServeMux, dynamoClient *shareddynamo.Client, sqsClient *sqsinfra.Client, queueURLs map[string]string) *serviceOrderRuntime {
	orderRepo := serviceOrderPersistence.NewServiceOrderRepository(database.DB)
	historyRepo := serviceOrderPersistence.NewDynamoHistoryRepository(dynamoClient)

	ms1Client := http_clients.NewMS1Client()
	ms3Client := http_clients.NewMS3Client()

	customerAdapter := serviceOrderAdapters.NewCustomerAdapter(ms1Client)
	vehicleAdapter := serviceOrderAdapters.NewVehicleAdapter(ms1Client)
	productAdapter := serviceOrderAdapters.NewProductAdapter(ms3Client)
	serviceAdapter := serviceOrderAdapters.NewServiceAdapter(ms3Client)
	emailService := email.NewSMTPEmailService()
	inventoryPublisher := publishers.NewOrderInventoryOperationPublisher(sqsClient, queueURLs[queueOrderInventoryRequested])
	sagaOrchestrator := appsaga.NewOrchestrator(orderRepo, historyRepo, inventoryPublisher, customerAdapter, emailService)
	billing := billingModule.NewModule(orderRepo, historyRepo, customerAdapter, emailService, nil)

	createUseCase := serviceOrderUsecases.NewCreateServiceOrder(orderRepo, customerAdapter, vehicleAdapter, productAdapter, serviceAdapter)
	getUseCase := serviceOrderUsecases.NewGetServiceOrder(orderRepo, productAdapter, serviceAdapter, customerAdapter, vehicleAdapter)
	getAllUseCase := serviceOrderUsecases.NewGetAllServiceOrders(orderRepo, customerAdapter, vehicleAdapter)
	updateUseCase := serviceOrderUsecases.NewUpdateServiceOrder(orderRepo, historyRepo, customerAdapter, vehicleAdapter, productAdapter, serviceAdapter)
	deleteUseCase := serviceOrderUsecases.NewDeleteServiceOrder(orderRepo, sagaOrchestrator, billing.CancelPaymentOrder, billing.RefundPaymentOrder)
	advanceUseCase := serviceOrderUsecases.NewAdvanceServiceOrderStatus(orderRepo, historyRepo, sagaOrchestrator, billing.CreatePaymentOrder, customerAdapter, emailService)
	authorizeUseCase := serviceOrderUsecases.NewRespondToAuthorization(orderRepo, historyRepo, sagaOrchestrator, customerAdapter, emailService)
	historyUseCase := serviceOrderUsecases.NewGetServiceOrderHistory(historyRepo)

	handler := serviceOrderHandlers.NewServiceOrderHandler(
		createUseCase,
		getUseCase,
		getAllUseCase,
		updateUseCase,
		deleteUseCase,
		advanceUseCase,
		authorizeUseCase,
		historyUseCase,
	)

	authMiddleware := middleware.NewAuthMiddleware(jwtAuth.NewJWTService())
	rbacMiddleware := middleware.NewRBACMiddleware()
	serviceOrderHttp.RegisterServiceOrderRoutes(mux, handler, authMiddleware, rbacMiddleware)
	billingHttp.RegisterBillingRoutes(
		mux,
		billingHandlers.NewWebhookHandler(billing.SignatureValidator, billing.HandlePaymentWebhook),
		billingHandlers.NewPaymentHandler(billing.GetPaymentStatus, billing.RetryPayment),
		authMiddleware,
		rbacMiddleware,
	)

	return &serviceOrderRuntime{
		orchestrator: sagaOrchestrator,
		orderRepo:    orderRepo,
		workers: []worker{
			consumers.NewOrderInventoryOperationSucceededConsumer(sqsClient, queueURLs[queueOrderInventorySucceeded], sagaOrchestrator),
			consumers.NewOrderInventoryOperationFailedConsumer(sqsClient, queueURLs[queueOrderInventoryFailed], sagaOrchestrator),
			consumers.NewCustomerDeletedConsumer(sqsClient, queueURLs[queueCustomerDeleted], orderRepo, deleteUseCase),
		},
	}
}

func logAwaitingPaymentOrders(ctx context.Context, orderRepo service_order.Repository) error {
	orders, err := orderRepo.FindBySagaStatus(ctx, service_order.SagaStatusAwaitingPayment)
	if err != nil {
		return err
	}
	for _, order := range orders {
		slog.Info("service order awaiting Mercado Pago webhook; no recovery publish needed", "order_id", order.ID())
	}
	return nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
