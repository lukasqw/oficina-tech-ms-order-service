package handlers_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"oficina-tech/internal/modules/service_order/application/usecases"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/modules/service_order/infra/http/handlers"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
	"oficina-tech/internal/shared/infra/http/middleware"
	"oficina-tech/internal/shared/infra/observability"
)

func TestMain(m *testing.M) {
	_ = observability.InitMetrics(otel.GetMeterProvider().Meter("test"))
	os.Exit(m.Run())
}

// --- mock implementations ---

type hMockRepo struct {
	orders  []*service_order.ServiceOrder
	findErr error
	saveErr error
}

func (r *hMockRepo) Save(_ context.Context, _ *service_order.ServiceOrder) error { return r.saveErr }
func (r *hMockRepo) SaveWithItems(_ context.Context, _ *service_order.ServiceOrder) error {
	return r.saveErr
}
func (r *hMockRepo) FindByID(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	for _, o := range r.orders {
		if o.ID() == id {
			return o, nil
		}
	}
	return nil, service_order.ErrServiceOrderNotFound
}
func (r *hMockRepo) FindByIDWithItems(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(context.Background(), id)
}
func (r *hMockRepo) FindAll(_ context.Context) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *hMockRepo) FindAllWithFilters(_ context.Context, _ service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *hMockRepo) FindByCustomerID(_ context.Context, customerID string) ([]*service_order.ServiceOrder, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	var result []*service_order.ServiceOrder
	for _, o := range r.orders {
		if o.CustomerID() == customerID {
			result = append(result, o)
		}
	}
	return result, nil
}
func (r *hMockRepo) FindByStatus(_ context.Context, _ service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *hMockRepo) FindBySagaStatus(_ context.Context, _ string) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *hMockRepo) Delete(_ context.Context, _ string) error { return r.saveErr }
func (r *hMockRepo) UpdateItemsHistoryID(_ context.Context, _ []string, _ string) error { return nil }

type hMockHistoryRepo struct {
	histories []*service_order.History
	findErr   error
	saveErr   error
}

func (r *hMockHistoryRepo) Save(_ context.Context, h *service_order.History) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	_ = h.SetID("hist-" + h.ServiceOrderID())
	return nil
}
func (r *hMockHistoryRepo) FindByServiceOrderID(_ context.Context, _ string) ([]*service_order.History, error) {
	return r.histories, r.findErr
}
func (r *hMockHistoryRepo) FindByID(_ context.Context, _ string) (*service_order.History, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if len(r.histories) > 0 {
		return r.histories[0], nil
	}
	return nil, service_order.ErrHistoryNotFound
}

type hMockCustomerAdapter struct {
	customer *dto.CustomerDTO
	err      error
}

func (a *hMockCustomerAdapter) GetCustomerByID(_ context.Context, _ string) (*dto.CustomerDTO, error) {
	return a.customer, a.err
}

type hMockVehicleAdapter struct {
	vehicle        *dto.VehicleDTO
	ownershipValid bool
	vehicleErr     error
	ownershipErr   error
}

func (a *hMockVehicleAdapter) GetVehicleByID(_ context.Context, _ string) (*dto.VehicleDTO, error) {
	return a.vehicle, a.vehicleErr
}
func (a *hMockVehicleAdapter) ValidateVehicleOwnership(_ context.Context, _, _ string) (bool, error) {
	return a.ownershipValid, a.ownershipErr
}

type hMockProductAdapter struct {
	product *dto.ProductDTO
	err     error
}

func (a *hMockProductAdapter) GetProductByID(_ context.Context, _ string) (*dto.ProductDTO, error) {
	return a.product, a.err
}

type hMockServiceAdapter struct {
	svc *dto.ServiceDTO
	err error
}

func (a *hMockServiceAdapter) GetServiceByID(_ context.Context, _ string) (*dto.ServiceDTO, error) {
	return a.svc, a.err
}

type hMockEmailService struct{ err error }

func (s *hMockEmailService) SendStatusUpdateEmail(_, _, _, _, _ string) error { return s.err }

var _ email.EmailService = (*hMockEmailService)(nil)

// helper to build a closed order (with ClosedAt set)
func newHandlerOrderClosed(id, customerID, vehicleID string) *service_order.ServiceOrder {
	now := time.Now()
	closedAt := now
	o, _ := service_order.ReconstructServiceOrder(
		id, customerID, vehicleID, "desc",
		service_order.StatusPaid, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{},
		&closedAt, now, now, nil,
	)
	return o
}

// helper to build a reconstructed order with a known ID
func newHandlerOrder(id, customerID, vehicleID string) *service_order.ServiceOrder {
	now := time.Now()
	o, _ := service_order.ReconstructServiceOrder(
		id, customerID, vehicleID, "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{},
		nil, now, now, nil,
	)
	return o
}

// buildHandler creates a handlers.ServiceOrderHandler with minimal use cases for testing.
// Pass nil for use cases that won't be called in the test.
func buildHandler(
	repo *hMockRepo,
	histRepo *hMockHistoryRepo,
	customerAdapter *hMockCustomerAdapter,
	vehicleAdapter *hMockVehicleAdapter,
) *handlers.ServiceOrderHandler {
	getAllUC := usecases.NewGetAllServiceOrders(repo, customerAdapter, vehicleAdapter)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("not found")},
		&hMockServiceAdapter{err: errors.New("not found")},
		customerAdapter, vehicleAdapter,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)

	return handlers.NewServiceOrderHandler(
		nil, // createUseCase — not used in these tests
		getUC,
		getAllUC,
		nil, // updateUseCase
		nil, // deleteUseCase
		nil, // advanceStatusUseCase
		nil, // authorizeUseCase
		histUC,
	)
}

// --- GetServiceOrder ---

func TestGetServiceOrder_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders/not-a-uuid", nil)
	req.SetPathValue("id", "not-a-uuid")
	rr := httptest.NewRecorder()
	h.GetServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestGetServiceOrder_NotFound(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders/550e8400-e29b-41d4-a716-446655440000", nil)
	req.SetPathValue("id", "550e8400-e29b-41d4-a716-446655440000")
	rr := httptest.NewRecorder()
	h.GetServiceOrder(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d", rr.Code)
	}
}

func TestGetServiceOrder_Found(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440000"
	order := newHandlerOrder(validUUID, "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com", Phone: "11"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{customer: customer},
		&hMockVehicleAdapter{vehicle: vehicle},
	)
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+validUUID, nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.GetServiceOrder(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// --- GetAllServiceOrders ---

func TestGetAllServiceOrders_Success(t *testing.T) {
	order := newHandlerOrder("order-1", "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{err: errors.New("skip")},
		&hMockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetAllServiceOrders_InvalidCustomerIDUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders?customer_id=not-a-uuid", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestGetAllServiceOrders_InvalidStatus(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders?status=BOGUS", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestGetAllServiceOrders_RepoError(t *testing.T) {
	repo := &hMockRepo{findErr: errors.New("db down")}
	h := buildHandler(repo, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("want 500, got %d", rr.Code)
	}
}

func TestGetAllServiceOrders_WithValidCustomerID(t *testing.T) {
	const custUUID = "550e8400-e29b-41d4-a716-446655440050"
	order := newHandlerOrder("order-1", custUUID, "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{err: errors.New("skip")},
		&hMockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	req := httptest.NewRequest(http.MethodGet, "/service-orders?customer_id="+custUUID, nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetAllServiceOrders_WithSortAndHide(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders?sort_by_status=true&hide_completed=true", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d", rr.Code)
	}
}

func TestGetAllServiceOrders_CustomerRole(t *testing.T) {
	const custUUID = "550e8400-e29b-41d4-a716-446655440040"
	order := newHandlerOrder("order-1", custUUID, "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{err: errors.New("skip")},
		&hMockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	ctx := context.WithValue(context.Background(), middleware.UserRoleKey, middleware.RoleCustomer)
	ctx = context.WithValue(ctx, middleware.UserIDKey, custUUID)
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetAllServiceOrders_WithCustomerVehicleAndClosedAt(t *testing.T) {
	closedOrder := newHandlerOrderClosed("order-closed", "cust-1", "veh-1")
	openOrder := newHandlerOrder("order-open", "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{closedOrder, openOrder}}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com", Phone: "11"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{customer: customer},
		&hMockVehicleAdapter{vehicle: vehicle},
	)
	req := httptest.NewRequest(http.MethodGet, "/service-orders", nil)
	rr := httptest.NewRecorder()
	h.GetAllServiceOrders(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetServiceOrder_WithClosedAt(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440020"
	order := newHandlerOrderClosed(validUUID, "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com", Phone: "11"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	h := buildHandler(repo, &hMockHistoryRepo{},
		&hMockCustomerAdapter{customer: customer},
		&hMockVehicleAdapter{vehicle: vehicle},
	)
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+validUUID, nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.GetServiceOrder(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// --- GetServiceOrderHistory ---

func TestGetServiceOrderHistory_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders/bad-id/history", nil)
	req.SetPathValue("id", "bad-id")
	rr := httptest.NewRecorder()
	h.GetServiceOrderHistory(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestGetServiceOrderHistory_Empty(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440001"
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+validUUID+"/history", nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.GetServiceOrderHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestGetServiceOrderHistory_WithEntries(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440002"
	now := time.Now()
	h2 := service_order.ReconstructHistory("hist-1", validUUID, map[string]any{"status": "changed"}, service_order.StatusDiagnosing, now)
	histRepo := &hMockHistoryRepo{histories: []*service_order.History{h2}}
	h := buildHandler(&hMockRepo{}, histRepo, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodGet, "/service-orders/"+validUUID+"/history", nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.GetServiceOrderHistory(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// --- CreateServiceOrder ---

func TestCreateServiceOrder_Success(t *testing.T) {
	const custUUID = "550e8400-e29b-41d4-a716-446655440010"
	const vehUUID = "550e8400-e29b-41d4-a716-446655440011"

	customer := &dto.CustomerDTO{ID: custUUID, Name: "Test", Email: "t@x.com", Phone: "11"}
	vehicle := &dto.VehicleDTO{ID: vehUUID, CustomerID: custUUID, LicensePlate: "ABC"}
	repo := &hMockRepo{}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{customer: customer}
	vehA := &hMockVehicleAdapter{vehicle: vehicle, ownershipValid: true}
	prodA := &hMockProductAdapter{err: errors.New("not found")}
	svcA := &hMockServiceAdapter{err: errors.New("not found")}

	createUC := usecases.NewCreateServiceOrder(repo, custA, vehA, prodA, svcA)
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo, prodA, svcA, custA, vehA)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(createUC, getUC, getAllUC, nil, nil, nil, nil, histUC)

	body := `{"customer_id":"` + custUUID + `","vehicle_id":"` + vehUUID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/service-orders", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.CreateServiceOrder(rr, req)
	if rr.Code != http.StatusCreated {
		t.Errorf("want 201, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateServiceOrder_ExecuteError(t *testing.T) {
	const custUUID = "550e8400-e29b-41d4-a716-446655440030"
	const vehUUID = "550e8400-e29b-41d4-a716-446655440031"
	repo := &hMockRepo{}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{err: errors.New("customer not found")}
	vehA := &hMockVehicleAdapter{}
	prodA := &hMockProductAdapter{}
	svcA := &hMockServiceAdapter{}

	createUC := usecases.NewCreateServiceOrder(repo, custA, vehA, prodA, svcA)
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo, prodA, svcA, custA, vehA)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(createUC, getUC, getAllUC, nil, nil, nil, nil, histUC)

	body := `{"customer_id":"` + custUUID + `","vehicle_id":"` + vehUUID + `"}`
	req := httptest.NewRequest(http.MethodPost, "/service-orders", strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.CreateServiceOrder(rr, req)
	if rr.Code < 400 {
		t.Errorf("expected error response, got %d", rr.Code)
	}
}

func TestCreateServiceOrder_BadJSON(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPost, "/service-orders", strings.NewReader("{invalid json}"))
	rr := httptest.NewRecorder()
	h.CreateServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestCreateServiceOrder_ValidationFail(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	// Missing required customer_id and vehicle_id
	req := httptest.NewRequest(http.MethodPost, "/service-orders", strings.NewReader(`{}`))
	rr := httptest.NewRecorder()
	h.CreateServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400 for validation failure, got %d", rr.Code)
	}
}

// --- UpdateServiceOrder ---

func TestUpdateServiceOrder_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPut, "/service-orders/bad-id", strings.NewReader(`{}`))
	req.SetPathValue("id", "bad-id")
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateServiceOrder_BadJSON(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440003"
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPut, "/service-orders/"+validUUID, strings.NewReader("{bad json}"))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateServiceOrder_Success(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440012"
	order := newHandlerOrder(validUUID, "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{ownershipValid: true}
	prodA := &hMockProductAdapter{err: errors.New("not found")}
	svcA := &hMockServiceAdapter{err: errors.New("not found")}

	updateUC := usecases.NewUpdateServiceOrder(repo, histRepo, custA, vehA, prodA, svcA)
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo, prodA, svcA, custA, vehA)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, updateUC, nil, nil, nil, histUC)

	req := httptest.NewRequest(http.MethodPut, "/service-orders/"+validUUID, strings.NewReader(`{}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateServiceOrder_WithEmptyItems(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440015"
	order := newHandlerOrder(validUUID, "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{ownershipValid: true}
	prodA := &hMockProductAdapter{err: errors.New("not found")}
	svcA := &hMockServiceAdapter{err: errors.New("not found")}

	updateUC := usecases.NewUpdateServiceOrder(repo, histRepo, custA, vehA, prodA, svcA)
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo, prodA, svcA, custA, vehA)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, updateUC, nil, nil, nil, histUC)

	req := httptest.NewRequest(http.MethodPut, "/service-orders/"+validUUID, strings.NewReader(`{"items":[]}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateServiceOrder_ValidationFail(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440016"
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	// customer_id set but not a valid UUID → validation fails
	req := httptest.NewRequest(http.MethodPut, "/service-orders/"+validUUID, strings.NewReader(`{"customer_id":"not-a-uuid"}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestUpdateServiceOrder_NotFound(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440013"
	repo := &hMockRepo{}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{ownershipValid: true}
	prodA := &hMockProductAdapter{err: errors.New("not found")}
	svcA := &hMockServiceAdapter{err: errors.New("not found")}

	updateUC := usecases.NewUpdateServiceOrder(repo, histRepo, custA, vehA, prodA, svcA)
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo, prodA, svcA, custA, vehA)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, updateUC, nil, nil, nil, histUC)

	req := httptest.NewRequest(http.MethodPut, "/service-orders/"+validUUID, strings.NewReader(`{}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.UpdateServiceOrder(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

// --- DeleteServiceOrder ---

func TestDeleteServiceOrder_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodDelete, "/service-orders/bad-id", nil)
	req.SetPathValue("id", "bad-id")
	rr := httptest.NewRecorder()
	h.DeleteServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- AdvanceServiceOrderStatus ---

func TestAdvanceServiceOrderStatus_Success_NoSaga(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440070"
	// RECEIVED → DIAGNOSING: DetermineInventoryOperation returns InventoryOpNone → no saga call
	order := newHandlerOrder(validUUID, "cust-1", "veh-1")
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{customer: &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com", Phone: "11"}}
	vehA := &hMockVehicleAdapter{}

	advanceUC := usecases.NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil, custA, &hMockEmailService{})
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("skip")},
		&hMockServiceAdapter{err: errors.New("skip")},
		custA, vehA,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, nil, nil, advanceUC, nil, histUC)

	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/advance", nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AdvanceServiceOrderStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestAdvanceServiceOrderStatus_NotFound(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440071"
	repo := &hMockRepo{}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{}

	advanceUC := usecases.NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil, custA, &hMockEmailService{})
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("skip")},
		&hMockServiceAdapter{err: errors.New("skip")},
		custA, vehA,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, nil, nil, advanceUC, nil, histUC)

	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/advance", nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AdvanceServiceOrderStatus(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestAdvanceServiceOrderStatus_FinalStatus(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440073"
	now := time.Now()
	// DELIVERED is a final status — NextStatus() returns error
	order, _ := service_order.ReconstructServiceOrder(
		validUUID, "cust-1", "veh-1", "desc",
		service_order.StatusDelivered, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{}, nil, now, now, nil,
	)
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{}

	advanceUC := usecases.NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil, custA, &hMockEmailService{})
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("skip")},
		&hMockServiceAdapter{err: errors.New("skip")},
		custA, vehA,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, nil, nil, advanceUC, nil, histUC)

	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/advance", nil)
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AdvanceServiceOrderStatus(rr, req)
	if rr.Code < 400 {
		t.Errorf("expected error response, got %d", rr.Code)
	}
}

func TestAdvanceServiceOrderStatus_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPost, "/service-orders/bad-id/advance", nil)
	req.SetPathValue("id", "bad-id")
	rr := httptest.NewRecorder()
	h.AdvanceServiceOrderStatus(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

// --- AuthorizeServiceOrder ---

func TestAuthorizeServiceOrder_InvalidUUID(t *testing.T) {
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPost, "/service-orders/bad-id/authorize", strings.NewReader(`{"approved":true}`))
	req.SetPathValue("id", "bad-id")
	rr := httptest.NewRecorder()
	h.AuthorizeServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestAuthorizeServiceOrder_Success_Approved(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440060"
	now := time.Now()
	order, _ := service_order.ReconstructServiceOrder(
		validUUID, "cust-1", "veh-1", "desc",
		service_order.StatusPendingAuthorization, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{}, nil, now, now, nil,
	)
	repo := &hMockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{customer: &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com", Phone: "11"}}
	vehA := &hMockVehicleAdapter{}

	authorizeUC := usecases.NewRespondToAuthorization(repo, histRepo, nil, custA, &hMockEmailService{})
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("skip")},
		&hMockServiceAdapter{err: errors.New("skip")},
		custA, vehA,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, nil, nil, nil, authorizeUC, histUC)

	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/authorize",
		strings.NewReader(`{"approved":true}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AuthorizeServiceOrder(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthorizeServiceOrder_NotFound(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440061"
	repo := &hMockRepo{}
	histRepo := &hMockHistoryRepo{}
	custA := &hMockCustomerAdapter{}
	vehA := &hMockVehicleAdapter{}

	authorizeUC := usecases.NewRespondToAuthorization(repo, histRepo, nil, custA, &hMockEmailService{})
	getAllUC := usecases.NewGetAllServiceOrders(repo, custA, vehA)
	getUC := usecases.NewGetServiceOrder(repo,
		&hMockProductAdapter{err: errors.New("skip")},
		&hMockServiceAdapter{err: errors.New("skip")},
		custA, vehA,
	)
	histUC := usecases.NewGetServiceOrderHistory(histRepo)
	h := handlers.NewServiceOrderHandler(nil, getUC, getAllUC, nil, nil, nil, authorizeUC, histUC)

	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/authorize",
		strings.NewReader(`{"approved":true}`))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AuthorizeServiceOrder(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d — body: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthorizeServiceOrder_BadJSON(t *testing.T) {
	const validUUID = "550e8400-e29b-41d4-a716-446655440004"
	h := buildHandler(&hMockRepo{}, &hMockHistoryRepo{}, &hMockCustomerAdapter{}, &hMockVehicleAdapter{})
	req := httptest.NewRequest(http.MethodPost, "/service-orders/"+validUUID+"/authorize", strings.NewReader("{bad json}"))
	req.SetPathValue("id", validUUID)
	rr := httptest.NewRecorder()
	h.AuthorizeServiceOrder(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}
