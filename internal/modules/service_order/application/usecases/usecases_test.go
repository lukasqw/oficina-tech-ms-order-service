package usecases

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/observability"
)

func TestMain(m *testing.M) {
	_ = observability.InitMetrics(otel.GetMeterProvider().Meter("test"))
	os.Exit(m.Run())
}

// --- Mock implementations ---

type mockRepo struct {
	orders  []*service_order.ServiceOrder
	findErr error
	saveErr error
}

func (r *mockRepo) Save(_ context.Context, _ *service_order.ServiceOrder) error { return r.saveErr }
func (r *mockRepo) SaveWithItems(_ context.Context, _ *service_order.ServiceOrder) error {
	return r.saveErr
}
func (r *mockRepo) FindByID(_ context.Context, id string) (*service_order.ServiceOrder, error) {
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
func (r *mockRepo) FindByIDWithItems(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(context.Background(), id)
}
func (r *mockRepo) FindAll(_ context.Context) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *mockRepo) FindAllWithFilters(_ context.Context, _ service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *mockRepo) FindByCustomerID(_ context.Context, customerID string) ([]*service_order.ServiceOrder, error) {
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
func (r *mockRepo) FindByStatus(_ context.Context, _ service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *mockRepo) FindBySagaStatus(_ context.Context, _ string) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *mockRepo) Delete(_ context.Context, _ string) error { return r.saveErr }
func (r *mockRepo) UpdateItemsHistoryID(_ context.Context, _ []string, _ string) error { return nil }

type mockHistoryRepo struct {
	histories []*service_order.History
	findErr   error
	saveErr   error
}

func (r *mockHistoryRepo) Save(_ context.Context, h *service_order.History) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	_ = h.SetID("hist-" + h.ServiceOrderID())
	return nil
}
func (r *mockHistoryRepo) FindByServiceOrderID(_ context.Context, _ string) ([]*service_order.History, error) {
	return r.histories, r.findErr
}
func (r *mockHistoryRepo) FindByID(_ context.Context, _ string) (*service_order.History, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if len(r.histories) > 0 {
		return r.histories[0], nil
	}
	return nil, service_order.ErrHistoryNotFound
}

type mockCustomerAdapter struct {
	customer *dto.CustomerDTO
	err      error
}

func (a *mockCustomerAdapter) GetCustomerByID(_ context.Context, _ string) (*dto.CustomerDTO, error) {
	return a.customer, a.err
}

type mockVehicleAdapter struct {
	vehicle        *dto.VehicleDTO
	ownershipValid bool
	vehicleErr     error
	ownershipErr   error
}

func (a *mockVehicleAdapter) GetVehicleByID(_ context.Context, _ string) (*dto.VehicleDTO, error) {
	return a.vehicle, a.vehicleErr
}
func (a *mockVehicleAdapter) ValidateVehicleOwnership(_ context.Context, _, _ string) (bool, error) {
	return a.ownershipValid, a.ownershipErr
}

type mockProductAdapter struct {
	product *dto.ProductDTO
	err     error
}

func (a *mockProductAdapter) GetProductByID(_ context.Context, _ string) (*dto.ProductDTO, error) {
	return a.product, a.err
}

type mockServiceAdapter struct {
	svc *dto.ServiceDTO
	err error
}

func (a *mockServiceAdapter) GetServiceByID(_ context.Context, _ string) (*dto.ServiceDTO, error) {
	return a.svc, a.err
}

// helper to build a reconstructed order with a known ID
func newOrderWithID(id, customerID, vehicleID string) *service_order.ServiceOrder {
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

// --- GetAllServiceOrders ---

func TestGetAllServiceOrders_EmptyList(t *testing.T) {
	repo := &mockRepo{}
	uc := NewGetAllServiceOrders(repo,
		&mockCustomerAdapter{err: errors.New("not found")},
		&mockVehicleAdapter{vehicleErr: errors.New("not found")},
	)

	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Orders) != 0 {
		t.Errorf("expected 0 orders, got %d", len(out.Orders))
	}
}

func TestGetAllServiceOrders_WithOrders(t *testing.T) {
	order := newOrderWithID("id-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "João", Email: "j@x.com", Phone: "11"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC-1234", Brand: "Toyota", Model: "Corolla"}

	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{customer: customer}, &mockVehicleAdapter{vehicle: vehicle, ownershipValid: true})

	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Orders) != 1 {
		t.Fatalf("expected 1 order, got %d", len(out.Orders))
	}
	if out.Orders[0].Customer == nil {
		t.Error("expected customer data populated")
	}
}

func TestGetAllServiceOrders_RepoError(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("db down")}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	if _, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

func TestGetAllServiceOrders_WithCustomerIDFilter(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	custID := "cust-1"
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{CustomerID: &custID})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestGetAllServiceOrders_WithSortAndHide(t *testing.T) {
	repo := &mockRepo{}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{
		SortByStatus:  true,
		HideCompleted: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

func TestGetAllServiceOrders_InvalidStatusFilter(t *testing.T) {
	repo := &mockRepo{}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	status := "INVALID_STATUS"
	if _, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{Status: &status}); err == nil {
		t.Fatal("expected error for invalid status filter")
	}
}

func TestGetAllServiceOrders_ValidStatusFilter(t *testing.T) {
	repo := &mockRepo{}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	status := "RECEIVED"
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{Status: &status})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
}

// --- GetServiceOrderHistory ---

func TestGetServiceOrderHistory_Empty(t *testing.T) {
	histRepo := &mockHistoryRepo{}
	uc := NewGetServiceOrderHistory(histRepo)

	out, err := uc.Execute(context.Background(), GetServiceOrderHistoryInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil || len(out.History) != 0 {
		t.Errorf("expected empty history, got %v", out)
	}
}

func TestGetServiceOrderHistory_WithEntries(t *testing.T) {
	now := time.Now()
	h := service_order.ReconstructHistory("hist-1", "so-1", map[string]any{"status": "changed"}, service_order.StatusDiagnosing, now)
	histRepo := &mockHistoryRepo{histories: []*service_order.History{h}}
	uc := NewGetServiceOrderHistory(histRepo)

	out, err := uc.Execute(context.Background(), GetServiceOrderHistoryInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.History) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(out.History))
	}
}

func TestGetServiceOrderHistory_RepoError(t *testing.T) {
	histRepo := &mockHistoryRepo{findErr: errors.New("db down")}
	uc := NewGetServiceOrderHistory(histRepo)
	if _, err := uc.Execute(context.Background(), GetServiceOrderHistoryInput{ServiceOrderID: "so-1"}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

// --- GetServiceOrder ---

func TestGetServiceOrder_NotFound(t *testing.T) {
	repo := &mockRepo{}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("not found")},
		&mockServiceAdapter{err: errors.New("not found")},
		&mockCustomerAdapter{err: errors.New("not found")},
		&mockVehicleAdapter{vehicleErr: errors.New("not found")},
	)
	if _, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "missing"}); err == nil {
		t.Fatal("expected not-found error")
	}
}

func TestGetServiceOrder_Found(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Maria", Email: "m@x.com", Phone: "21"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "XYZ-9999", Brand: "Honda", Model: "Civic"}

	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("not found")},
		&mockServiceAdapter{err: errors.New("not found")},
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle},
	)

	out, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.ID != "so-1" {
		t.Errorf("want ID 'so-1', got %s", out.ID)
	}
	if out.Customer == nil {
		t.Error("expected customer populated")
	}
}

// --- CreateServiceOrder ---

func TestCreateServiceOrder_CustomerNotFound(t *testing.T) {
	repo := &mockRepo{}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{err: errors.New("customer not found")},
		&mockVehicleAdapter{vehicleErr: errors.New("vehicle not found")},
		&mockProductAdapter{},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{CustomerID: "cust-1", VehicleID: "veh-1", Items: []ItemInput{}}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when customer not found")
	}
}

func TestCreateServiceOrder_VehicleNotFound(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicleErr: errors.New("vehicle not found")},
		&mockProductAdapter{},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{CustomerID: "cust-1", VehicleID: "veh-1", Items: []ItemInput{}}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when vehicle not found")
	}
}

func TestCreateServiceOrder_VehicleOwnershipFails(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "other-cust"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: false},
		&mockProductAdapter{},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{CustomerID: "cust-1", VehicleID: "veh-1", Items: []ItemInput{}}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when vehicle ownership check fails")
	}
}

func TestCreateServiceOrder_Success_NoItems(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{CustomerID: "cust-1", VehicleID: "veh-1", Description: "Revisão", Items: []ItemInput{}}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.CustomerID != "cust-1" {
		t.Errorf("want CustomerID 'cust-1', got %s", out.CustomerID)
	}
}

// --- mockEmailService ---

type mockEmailService struct{ err error }

func (s *mockEmailService) SendStatusUpdateEmail(_, _, _, _, _ string) error { return s.err }

// --- UpdateServiceOrder ---

func TestUpdateServiceOrder_NotFound(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("not found")}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{}, &mockVehicleAdapter{},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "missing"}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when order not found")
	}
}

func TestUpdateServiceOrder_Deleted(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{}, &mockVehicleAdapter{},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1"}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for deleted order")
	}
}

func TestUpdateServiceOrder_DescriptionOnly(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	// Ownership check always runs for existing vehicle — must return true
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	desc := "Nova descrição"
	input := UpdateServiceOrderInput{ID: "so-1", Description: &desc}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Description != "Nova descrição" {
		t.Errorf("want description updated, got %s", out.Description)
	}
}

func TestUpdateServiceOrder_CustomerNotFound(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	newCustomer := "cust-2"
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{err: errors.New("customer not found")},
		&mockVehicleAdapter{vehicleErr: errors.New("skip")},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", CustomerID: &newCustomer}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when new customer not found")
	}
}

func TestUpdateServiceOrder_OwnershipFails(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	newVehicle := "veh-2"
	vehicle := &dto.VehicleDTO{ID: "veh-2", CustomerID: "other-cust"}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: false},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", VehicleID: &newVehicle}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when ownership check fails")
	}
}

// --- RespondToAuthorization ---

func newOrderWithStatus(id, customerID, vehicleID string, status service_order.OrderStatus) *service_order.ServiceOrder {
	now := time.Now()
	o, _ := service_order.ReconstructServiceOrder(
		id, customerID, vehicleID, "desc",
		status, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{},
		nil, now, now, nil,
	)
	return o
}

func TestRespondToAuthorization_NotFound(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("not found")}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "missing",
	}); err == nil {
		t.Fatal("expected error when order not found")
	}
}

func TestRespondToAuthorization_WrongCustomer(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1",
		CallerRole:     "CUSTOMER",
		CallerID:       "other-cust",
	}); err == nil {
		t.Fatal("expected forbidden error when wrong customer tries to authorize")
	}
}

func TestRespondToAuthorization_Deleted(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1",
	}); err == nil {
		t.Fatal("expected error for deleted order")
	}
}

func TestRespondToAuthorization_WrongStatus(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusReceived)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1",
		Approved:       true,
	}); err == nil {
		t.Fatal("expected error when order is not in PENDING_AUTHORIZATION")
	}
}

func TestRespondToAuthorization_Approved_NoSaga(t *testing.T) {
	// PENDING_AUTHORIZATION → AUTHORIZED has InventoryOpNone, so no saga needed
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{err: errors.New("email skip")},
		&mockEmailService{},
	)
	out, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1",
		Approved:       true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("expected non-nil output")
	}
	if out.Status != "AUTHORIZED" {
		t.Errorf("want status AUTHORIZED, got %s", out.Status)
	}
}

func TestRespondToAuthorization_Approved_WithObservation(t *testing.T) {
	order := newOrderWithStatus("so-obs", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{customer: customer},
		&mockEmailService{},
	)
	obs := "Approved with observation"
	out, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-obs",
		Approved:       true,
		Observation:    &obs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "AUTHORIZED" {
		t.Errorf("want AUTHORIZED, got %s", out.Status)
	}
}

func TestRespondToAuthorization_HistoryRepoError(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{saveErr: errors.New("db error")}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1", Approved: true,
	}); err == nil {
		t.Fatal("expected error when historyRepo.Save fails")
	}
}

func TestRespondToAuthorization_SaveError(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}, saveErr: errors.New("db error")}
	histRepo := &mockHistoryRepo{}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1", Approved: true,
	}); err == nil {
		t.Fatal("expected error when repo.Save fails")
	}
}

// --- AdvanceServiceOrderStatus ---

func TestAdvanceServiceOrderStatus_NotFound(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("not found")}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "missing"}); err == nil {
		t.Fatal("expected error when order not found")
	}
}

func TestAdvanceServiceOrderStatus_Deleted(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"}); err == nil {
		t.Fatal("expected error for deleted order")
	}
}

func TestAdvanceServiceOrderStatus_FinalStatus(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusDelivered)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"}); err == nil {
		t.Fatal("expected error for final status DELIVERED")
	}
}

func TestAdvanceServiceOrderStatus_Success_NoSaga(t *testing.T) {
	// RECEIVED → DIAGNOSING: DetermineInventoryOperation returns InventoryOpNone → no saga
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{err: errors.New("email skip")},
		&mockEmailService{},
	)
	out, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "DIAGNOSING" {
		t.Errorf("want DIAGNOSING, got %s", out.Status)
	}
	if out.Async {
		t.Error("expected Async=false for InventoryOpNone path")
	}
}

func TestAdvanceServiceOrderStatus_HistoryRepoError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{saveErr: errors.New("db error")}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"}); err == nil {
		t.Fatal("expected error when historyRepo.Save fails")
	}
}

func TestAdvanceServiceOrderStatus_SaveError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}, saveErr: errors.New("db error")}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{}, &mockEmailService{},
	)
	if _, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"}); err == nil {
		t.Fatal("expected error when repo.Save fails")
	}
}

func TestAdvanceServiceOrderStatus_Success_WithEmail(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{customer: customer},
		&mockEmailService{},
	)
	out, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "DIAGNOSING" {
		t.Errorf("want DIAGNOSING, got %s", out.Status)
	}
}

// --- DeleteServiceOrder ---

func TestDeleteServiceOrder_NotFound(t *testing.T) {
	repo := &mockRepo{}
	uc := NewDeleteServiceOrder(repo, nil, nil, nil)
	if _, err := uc.Execute(context.Background(), DeleteServiceOrderInput{ID: "missing"}); err == nil {
		t.Fatal("expected error when order not found")
	}
}

func TestDeleteServiceOrder_AlreadyDeleted(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewDeleteServiceOrder(repo, nil, nil, nil)
	if _, err := uc.Execute(context.Background(), DeleteServiceOrderInput{ID: "so-1"}); err == nil {
		t.Fatal("expected error for already deleted order")
	}
}

func TestDeleteServiceOrder_RepoError(t *testing.T) {
	repo := &mockRepo{findErr: errors.New("db down")}
	uc := NewDeleteServiceOrder(repo, nil, nil, nil)
	if _, err := uc.Execute(context.Background(), DeleteServiceOrderInput{ID: "so-1"}); err == nil {
		t.Fatal("expected repo error to propagate")
	}
}

// --- UpdateServiceOrder with items ---

func TestUpdateServiceOrder_WithProductItem(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	product := &dto.ProductDTO{ID: "ref-1", Name: "Product A", Price: 5000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 2}},
	}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 item in output, got %d", len(out.Items))
	}
	if out.Items[0].Name != "Product A" {
		t.Errorf("want 'Product A', got %s", out.Items[0].Name)
	}
}

func TestUpdateServiceOrder_WithServiceItem(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	svc := &dto.ServiceDTO{ID: "ref-2", Name: "Service B", Price: 10000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{err: errors.New("not a product")},
		&mockServiceAdapter{svc: svc},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "SERVICE", ReferenceID: "ref-2", Quantity: 1}},
	}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(out.Items))
	}
}

func TestUpdateServiceOrder_ItemInvalidType(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "INVALID", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for invalid item type")
	}
}

func TestUpdateServiceOrder_ItemProductNotFound(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{err: errors.New("product not found")},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when product not found")
	}
}

func TestUpdateServiceOrder_CannotModifyItems(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when order cannot modify items (PENDING_AUTHORIZATION)")
	}
}

func TestUpdateServiceOrder_ReplacesExistingItems(t *testing.T) {
	// Order with an existing item — updateItems should remove it and add the new one
	// This covers the saveHistory SetHistoryID loop and buildOutput deleted-item filter
	now := time.Now()
	existingItem, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "old-ref", "Old Product", 1, 1000)
	_ = existingItem.SetID("existing-item-id")
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{existingItem},
		nil, now, now, nil,
	)

	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	product := &dto.ProductDTO{ID: "new-ref", Name: "New Product", Price: 5000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "new-ref", Quantity: 2}},
	}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 active item in output, got %d", len(out.Items))
	}
	if out.Items[0].Name != "New Product" {
		t.Errorf("want 'New Product', got %s", out.Items[0].Name)
	}
}

func TestUpdateServiceOrder_SaveWithItemsError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}, saveErr: errors.New("db error")}
	histRepo := &mockHistoryRepo{}
	product := &dto.ProductDTO{ID: "ref-1", Name: "Product A", Price: 5000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when SaveWithItems fails")
	}
}

func TestUpdateServiceOrder_WithNewCustomer(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	newCust := "cust-2"
	customer := &dto.CustomerDTO{ID: "cust-2", Name: "New Customer", Email: "new@x.com"}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", CustomerID: &newCust}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.CustomerID != "cust-2" {
		t.Errorf("want CustomerID 'cust-2', got %s", out.CustomerID)
	}
}

// --- RespondToAuthorization email paths ---

func TestRespondToAuthorization_EmailFails(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{customer: customer},
		&mockEmailService{err: errors.New("smtp down")},
	)
	out, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1", Approved: true,
	})
	if err != nil {
		t.Fatalf("expected success despite email failure: %v", err)
	}
	if out.Status != "AUTHORIZED" {
		t.Errorf("want AUTHORIZED, got %s", out.Status)
	}
}

// --- AdvanceServiceOrderStatus additional paths ---

func TestAdvanceServiceOrderStatus_EmailFails(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{customer: customer},
		&mockEmailService{err: errors.New("smtp down")},
	)
	// Email failure is just logged — should still succeed
	out, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("expected success despite email failure: %v", err)
	}
	if out.Status != "DIAGNOSING" {
		t.Errorf("want DIAGNOSING, got %s", out.Status)
	}
}

func TestUpdateServiceOrder_HistoryRepoError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{saveErr: errors.New("db error")}
	desc := "desc"
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", Description: &desc}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when historyRepo.Save fails")
	}
}

func TestUpdateServiceOrder_SaveError_NoItems(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}, saveErr: errors.New("db error")}
	histRepo := &mockHistoryRepo{}
	desc := "desc"
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", Description: &desc}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when repo.Save fails (no items)")
	}
}

func TestUpdateServiceOrder_WithNewVehicle(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	newVeh := "veh-2"
	vehicle := &dto.VehicleDTO{ID: "veh-2", CustomerID: "cust-1"}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", VehicleID: &newVeh}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.VehicleID != "veh-2" {
		t.Errorf("want VehicleID 'veh-2', got %s", out.VehicleID)
	}
}

// --- CreateServiceOrder with items ---

func TestCreateServiceOrder_Success_WithProductItem(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	product := &dto.ProductDTO{ID: "ref-p", Name: "Óleo Motor", Price: 5000}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{
		CustomerID: "cust-1",
		VehicleID:  "veh-1",
		Items:      []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-p", Quantity: 2}},
	}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(out.Items))
	}
}

func TestCreateServiceOrder_Success_WithServiceItem(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	svc := &dto.ServiceDTO{ID: "ref-s", Name: "Troca de Óleo", Price: 10000}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{err: errors.New("not a product")},
		&mockServiceAdapter{svc: svc},
	)
	input := CreateServiceOrderInput{
		CustomerID: "cust-1",
		VehicleID:  "veh-1",
		Items:      []ItemInput{{ItemType: "SERVICE", ReferenceID: "ref-s", Quantity: 1}},
	}
	out, err := uc.Execute(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 item, got %d", len(out.Items))
	}
}

func TestCreateServiceOrder_InvalidItemType(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{
		CustomerID: "cust-1",
		VehicleID:  "veh-1",
		Items:      []ItemInput{{ItemType: "INVALID", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for invalid item type")
	}
}

func TestCreateServiceOrder_ServiceNotFound(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{err: errors.New("not a product")},
		&mockServiceAdapter{err: errors.New("service not found")},
	)
	input := CreateServiceOrderInput{
		CustomerID: "cust-1",
		VehicleID:  "veh-1",
		Items:      []ItemInput{{ItemType: "SERVICE", ReferenceID: "ref-s", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when service not found")
	}
}

func TestCreateServiceOrder_SaveError(t *testing.T) {
	repo := &mockRepo{saveErr: errors.New("db error")}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1", LicensePlate: "ABC"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{
		CustomerID:  "cust-1",
		VehicleID:   "veh-1",
		Description: "Revisão",
		Items:       []ItemInput{},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when SaveWithItems fails")
	}
}

func TestCreateServiceOrder_ProductNotFound(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	vehicle := &dto.VehicleDTO{ID: "veh-1", CustomerID: "cust-1"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{vehicle: vehicle, ownershipValid: true},
		&mockProductAdapter{err: errors.New("product not found")},
		&mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{
		CustomerID: "cust-1",
		VehicleID:  "veh-1",
		Items:      []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when product not found")
	}
}

func TestAdvanceServiceOrderStatus_AuthorizedToInProgress(t *testing.T) {
	// AUTHORIZED → IN_PROGRESS: InventoryOpNone → no saga
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusAuthorized)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewAdvanceServiceOrderStatus(repo, histRepo, nil, nil,
		&mockCustomerAdapter{err: errors.New("email skip")},
		&mockEmailService{},
	)
	out, err := uc.Execute(context.Background(), AdvanceServiceOrderStatusInput{ServiceOrderID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "IN_PROGRESS" {
		t.Errorf("want IN_PROGRESS, got %s", out.Status)
	}
}

// mockHistoryRepoNoID implements HistoryRepository but does NOT set the history ID on Save.
// This causes history.ID() to remain "" so that item.SetHistoryID("") fails.
type mockHistoryRepoNoID struct{}

func (r *mockHistoryRepoNoID) Save(_ context.Context, _ *service_order.History) error { return nil }
func (r *mockHistoryRepoNoID) FindByServiceOrderID(_ context.Context, _ string) ([]*service_order.History, error) {
	return nil, nil
}
func (r *mockHistoryRepoNoID) FindByID(_ context.Context, _ string) (*service_order.History, error) {
	return nil, nil
}

// --- UpdateServiceOrder: RemoveItem error (item with empty ID) ---

func TestUpdateServiceOrder_RemoveItemError(t *testing.T) {
	// Create item WITHOUT calling SetID → ID stays ""
	// RemoveItem("") returns ErrInvalidItemID → updateItems returns error
	noIDItem, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "old-ref", "Old Item", 1, 100)
	now := time.Now()
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{noIDItem},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	product := &dto.ProductDTO{ID: "new-ref", Name: "New Product", Price: 5000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "new-ref", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when RemoveItem fails (item has empty ID)")
	}
}

// --- UpdateServiceOrder: SetHistoryID error (history ID not set by repo) ---

func TestUpdateServiceOrder_SetHistoryIDError(t *testing.T) {
	// historyRepo.Save does NOT set history ID → history.ID() = "" →
	// item.SetHistoryID("") fails with ErrInvalidHistoryID
	now := time.Now()
	existingItem, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "old-ref", "Old Item", 1, 100)
	_ = existingItem.SetID("existing-item-1")
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{existingItem},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	product := &dto.ProductDTO{ID: "new-ref", Name: "New Product", Price: 5000}
	uc := NewUpdateServiceOrder(repo, &mockHistoryRepoNoID{},
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "new-ref", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when SetHistoryID fails (history ID is empty)")
	}
}

// --- GetServiceOrderHistory extra paths ---

func TestGetServiceOrderHistory_EmptyID(t *testing.T) {
	histRepo := &mockHistoryRepo{}
	uc := NewGetServiceOrderHistory(histRepo)
	if _, err := uc.Execute(context.Background(), GetServiceOrderHistoryInput{ServiceOrderID: ""}); err == nil {
		t.Fatal("expected error for empty ServiceOrderID")
	}
}

// --- GetServiceOrder extra paths ---

func TestGetServiceOrder_Deleted(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("skip")},
		&mockServiceAdapter{err: errors.New("skip")},
		&mockCustomerAdapter{err: errors.New("skip")},
		&mockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	if _, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"}); err == nil {
		t.Fatal("expected error for deleted order")
	}
}

func TestGetServiceOrder_WithActiveItem(t *testing.T) {
	now := time.Now()
	item, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "ref-1", "Item", 2, 1000)
	_ = item.SetID("item-1")
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{item},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("skip")},
		&mockServiceAdapter{err: errors.New("skip")},
		&mockCustomerAdapter{err: errors.New("skip")},
		&mockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	out, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 1 {
		t.Errorf("expected 1 item in output, got %d", len(out.Items))
	}
}

func TestGetServiceOrder_WithDeletedItem(t *testing.T) {
	now := time.Now()
	deletedAt := now
	deletedItem := service_order.ReconstructServiceOrderItem(
		"item-del", "so-1", nil, service_order.ItemTypeProduct, "ref-1", "Item", 1, 100,
		now, now, &deletedAt,
	)
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{deletedItem},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("skip")},
		&mockServiceAdapter{err: errors.New("skip")},
		&mockCustomerAdapter{err: errors.New("skip")},
		&mockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	out, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 0 {
		t.Errorf("expected 0 active items (deleted filtered), got %d", len(out.Items))
	}
}

func TestGetServiceOrder_CustomerVehicleNotFound(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("skip")},
		&mockServiceAdapter{err: errors.New("skip")},
		&mockCustomerAdapter{err: errors.New("customer not found")},
		&mockVehicleAdapter{vehicleErr: errors.New("vehicle not found")},
	)
	out, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Customer != nil {
		t.Error("expected nil customer output when customer not found")
	}
	if out.Vehicle != nil {
		t.Error("expected nil vehicle output when vehicle not found")
	}
}

// --- GetAllServiceOrders extra paths ---

func TestGetAllServiceOrders_WithDeletedOrder(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	order.MarkAsDeleted()
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Orders) != 0 {
		t.Errorf("expected deleted order filtered, got %d orders", len(out.Orders))
	}
}

func TestGetAllServiceOrders_WithOrderHavingItems(t *testing.T) {
	now := time.Now()
	item, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "ref-1", "Item", 1, 500)
	_ = item.SetID("item-1")
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{item},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Orders) != 1 || len(out.Orders[0].Items) != 1 {
		t.Errorf("expected 1 order with 1 item")
	}
}

func TestGetAllServiceOrders_WithDeletedItemInOrder(t *testing.T) {
	now := time.Now()
	deletedAt := now
	deletedItem := service_order.ReconstructServiceOrderItem(
		"item-del", "so-1", nil, service_order.ItemTypeProduct, "ref-1", "Item", 1, 100,
		now, now, &deletedAt,
	)
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{deletedItem},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetAllServiceOrders(repo, &mockCustomerAdapter{}, &mockVehicleAdapter{})
	out, err := uc.Execute(context.Background(), GetAllServiceOrdersInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Orders) != 1 || len(out.Orders[0].Items) != 0 {
		t.Errorf("expected 1 order with 0 active items (deleted filtered)")
	}
}

// --- UpdateServiceOrder extra paths ---

func TestUpdateServiceOrder_VehicleOwnershipError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipErr: errors.New("ownership check failed")},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	if _, err := uc.Execute(context.Background(), UpdateServiceOrderInput{ID: "so-1"}); err == nil {
		t.Fatal("expected error when ownership check returns error")
	}
}

func TestUpdateServiceOrder_EmptyVehicleIDPointer(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	emptyVeh := ""
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", VehicleID: &emptyVeh}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for empty vehicleID (UpdateVehicle fails with empty string)")
	}
}

func TestUpdateServiceOrder_ItemZeroQuantity(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	product := &dto.ProductDTO{ID: "ref-1", Name: "Product A", Price: 5000}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{product: product},
		&mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "PRODUCT", ReferenceID: "ref-1", Quantity: 0}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for zero quantity → NewServiceOrderItem fails")
	}
}

func TestUpdateServiceOrder_ItemServiceNotFound(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{err: errors.New("not a product")},
		&mockServiceAdapter{err: errors.New("service not found")},
	)
	input := UpdateServiceOrderInput{
		ID:    "so-1",
		Items: []ItemInput{{ItemType: "SERVICE", ReferenceID: "ref-s", Quantity: 1}},
	}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when service not found via fetchItemDetails")
	}
}

// --- CreateServiceOrder extra paths ---

func TestCreateServiceOrder_EmptyVehicleID(t *testing.T) {
	repo := &mockRepo{}
	customer := &dto.CustomerDTO{ID: "cust-1", Name: "Test", Email: "t@x.com"}
	uc := NewCreateServiceOrder(repo,
		&mockCustomerAdapter{customer: customer},
		&mockVehicleAdapter{},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := CreateServiceOrderInput{CustomerID: "cust-1", VehicleID: "", Items: []ItemInput{}}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error for empty vehicleID (NewServiceOrder fails)")
	}
}

// --- GetServiceOrder: 2 items → sort.Slice lambda triggered ---

func TestGetServiceOrder_WithMultipleItems(t *testing.T) {
	now := time.Now()
	later := now.Add(time.Second)
	item1, _ := service_order.NewServiceOrderItem("so-1", service_order.ItemTypeProduct, "ref-1", "Item A", 1, 500)
	_ = item1.SetID("item-1")
	item2 := service_order.ReconstructServiceOrderItem(
		"item-2", "so-1", nil, service_order.ItemTypeService, "ref-2", "Item B", 2, 1000,
		later, later, nil,
	)
	order, _ := service_order.ReconstructServiceOrder(
		"so-1", "cust-1", "veh-1", "desc",
		service_order.StatusReceived, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		[]*service_order.ServiceOrderItem{item2, item1},
		nil, now, now, nil,
	)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	uc := NewGetServiceOrder(repo,
		&mockProductAdapter{err: errors.New("skip")},
		&mockServiceAdapter{err: errors.New("skip")},
		&mockCustomerAdapter{err: errors.New("skip")},
		&mockVehicleAdapter{vehicleErr: errors.New("skip")},
	)
	out, err := uc.Execute(context.Background(), GetServiceOrderInput{ID: "so-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(out.Items))
	}
}

// --- UpdateServiceOrder: GetVehicleByID error + empty customerID ---

func TestUpdateServiceOrder_GetVehicleError(t *testing.T) {
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	newVeh := "veh-2"
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{vehicleErr: errors.New("vehicle not found")},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", VehicleID: &newVeh}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when GetVehicleByID fails")
	}
}

func TestUpdateServiceOrder_EmptyCustomerIDPointer(t *testing.T) {
	// Customer pointer set to "" → GetCustomerByID returns nil,nil → UpdateCustomer("") fails
	order := newOrderWithID("so-1", "cust-1", "veh-1")
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	emptyCustomer := ""
	uc := NewUpdateServiceOrder(repo, histRepo,
		&mockCustomerAdapter{},
		&mockVehicleAdapter{ownershipValid: true},
		&mockProductAdapter{}, &mockServiceAdapter{},
	)
	input := UpdateServiceOrderInput{ID: "so-1", CustomerID: &emptyCustomer}
	if _, err := uc.Execute(context.Background(), input); err == nil {
		t.Fatal("expected error when UpdateCustomer is called with empty string")
	}
}

// --- RespondToAuthorization extra paths ---

func TestRespondToAuthorization_EmptyObservation(t *testing.T) {
	order := newOrderWithStatus("so-1", "cust-1", "veh-1", service_order.StatusPendingAuthorization)
	repo := &mockRepo{orders: []*service_order.ServiceOrder{order}}
	histRepo := &mockHistoryRepo{}
	obs := ""
	uc := NewRespondToAuthorization(repo, histRepo, nil,
		&mockCustomerAdapter{err: errors.New("email skip")},
		&mockEmailService{},
	)
	out, err := uc.Execute(context.Background(), RespondToAuthorizationInput{
		ServiceOrderID: "so-1",
		Approved:       true,
		Observation:    &obs,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Status != "AUTHORIZED" {
		t.Errorf("want AUTHORIZED, got %s", out.Status)
	}
}
