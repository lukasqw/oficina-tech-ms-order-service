package usecases

import (
	"context"
	"errors"
	"testing"
	"time"

	"oficina-tech/internal/modules/billing/domain/payment"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/email"
)

const (
	testOrderID    = "11111111-1111-4111-8111-111111111111"
	testCustomerID = "22222222-2222-4222-8222-222222222222"
	testVehicleID  = "33333333-3333-4333-8333-333333333333"
)

func TestHandlePaymentWebhookApprovedTransitionsToPaid(t *testing.T) {
	repo := newMemoryOrderRepo(awaitingPaymentOrder(t))
	historyRepo := &memoryHistoryRepo{}
	emailService := email.NewMockEmailService()
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: testOrderID}},
		repo,
		historyRepo,
		&fakeCustomerAdapter{},
		emailService,
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !output.Processed || output.Status != service_order.StatusPaid.String() {
		t.Fatalf("unexpected output: %+v", output)
	}
	order := repo.orders[testOrderID]
	if order.Status() != service_order.StatusPaid || order.SagaStatus() != service_order.SagaStatusIdle {
		t.Fatalf("order was not paid: status=%s saga=%s", order.Status(), order.SagaStatus())
	}
	if order.MPPaymentID() == nil || *order.MPPaymentID() != "pay-1" {
		t.Fatalf("mp_payment_id was not persisted")
	}
	if len(historyRepo.saved) != 1 {
		t.Fatalf("expected history save")
	}
	if len(emailService.SentEmails) != 1 {
		t.Fatalf("expected payment email")
	}
}

func TestHandlePaymentWebhookRejectedReturnsToCompleted(t *testing.T) {
	repo := newMemoryOrderRepo(awaitingPaymentOrder(t))
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-2", Status: "rejected", ExternalReference: testOrderID}},
		repo,
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-2"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !output.Processed || output.Status != service_order.StatusCompleted.String() {
		t.Fatalf("unexpected output: %+v", output)
	}
	order := repo.orders[testOrderID]
	if order.Status() != service_order.StatusCompleted || order.SagaStatus() != service_order.SagaStatusIdle {
		t.Fatalf("order did not return to completed: status=%s saga=%s", order.Status(), order.SagaStatus())
	}
}

func TestHandlePaymentWebhookRejectedIsIdempotentWhenAlreadyCompleted(t *testing.T) {
	historyRepo := &memoryHistoryRepo{}
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-2", Status: "cancelled", ExternalReference: testOrderID}},
		newMemoryOrderRepo(completedOrder(t)),
		historyRepo,
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-2"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Processed || len(historyRepo.saved) != 0 {
		t.Fatalf("completed rejected webhook should be idempotent: %+v", output)
	}
}

func TestHandlePaymentWebhookPendingIsIgnored(t *testing.T) {
	order := awaitingPaymentOrder(t)
	repo := newMemoryOrderRepo(order)
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-3", Status: "pending", ExternalReference: testOrderID}},
		repo,
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-3"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Processed {
		t.Fatalf("pending webhook should be ignored")
	}
	if repo.orders[testOrderID].Status() != service_order.StatusAwaitingPayment {
		t.Fatalf("status changed unexpectedly")
	}
}

func TestHandlePaymentWebhookApprovedIsIdempotentWhenAlreadyPaid(t *testing.T) {
	order := awaitingPaymentOrder(t)
	if err := order.ConfirmPayment("pay-1"); err != nil {
		t.Fatalf("ConfirmPayment() error = %v", err)
	}
	repo := newMemoryOrderRepo(order)
	historyRepo := &memoryHistoryRepo{}
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: testOrderID}},
		repo,
		historyRepo,
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Processed || len(historyRepo.saved) != 0 {
		t.Fatalf("already paid webhook should be idempotent: %+v", output)
	}
}

func TestHandlePaymentWebhookApprovedIgnoredWhenOrderIsNotAwaiting(t *testing.T) {
	historyRepo := &memoryHistoryRepo{}
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: testOrderID}},
		newMemoryOrderRepo(completedOrder(t)),
		historyRepo,
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-1"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Processed || len(historyRepo.saved) != 0 {
		t.Fatalf("non-awaiting order should be ignored: %+v", output)
	}
}

func TestHandlePaymentWebhookApprovedPropagatesHistoryError(t *testing.T) {
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: testOrderID}},
		newMemoryOrderRepo(awaitingPaymentOrder(t)),
		&memoryHistoryRepo{err: errors.New("history failed")},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	if _, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-1"}); err == nil {
		t.Fatalf("expected history error")
	}
}

func TestHandlePaymentWebhookApprovedSkipsEmailWhenCustomerLookupFails(t *testing.T) {
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-1", Status: "approved", ExternalReference: testOrderID}},
		newMemoryOrderRepo(awaitingPaymentOrder(t)),
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{err: errors.New("customer failed")},
		email.NewMockEmailService(),
	)

	if _, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-1"}); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestHandlePaymentWebhookUsesPayloadExternalReferenceFallback(t *testing.T) {
	repo := newMemoryOrderRepo(awaitingPaymentOrder(t))
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-4", Status: "approved"}},
		repo,
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-4", ExternalReference: testOrderID})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !output.Processed || output.OrderID != testOrderID {
		t.Fatalf("unexpected fallback output: %+v", output)
	}
}

func TestHandlePaymentWebhookErrorsWithoutExternalReference(t *testing.T) {
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-5", Status: "approved"}},
		newMemoryOrderRepo(awaitingPaymentOrder(t)),
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	if _, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-5"}); err != payment.ErrMalformedWebhook {
		t.Fatalf("expected ErrMalformedWebhook, got %v", err)
	}
}

func TestHandlePaymentWebhookUnknownStatusIsIgnored(t *testing.T) {
	uc := NewHandlePaymentWebhook(
		&fakeMPClient{payment: &payment.Payment{ID: "pay-6", Status: "refunded", ExternalReference: testOrderID}},
		newMemoryOrderRepo(awaitingPaymentOrder(t)),
		&memoryHistoryRepo{},
		&fakeCustomerAdapter{},
		email.NewMockEmailService(),
	)

	output, err := uc.Execute(context.Background(), HandlePaymentWebhookInput{PaymentID: "pay-6"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Processed || output.Status != "refunded" {
		t.Fatalf("unexpected unknown status output: %+v", output)
	}
}

func TestGetPaymentStatus(t *testing.T) {
	uc := NewGetPaymentStatus(newMemoryOrderRepo(awaitingPaymentOrder(t)))
	output, err := uc.Execute(context.Background(), testOrderID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.PaymentURL != "https://pay/pref-1" || output.PreferenceID != "pref-1" || output.Status != service_order.StatusAwaitingPayment.String() {
		t.Fatalf("unexpected output: %+v", output)
	}
}

func TestGetPaymentStatusReturnsNotAvailableWhenOrderIsNotAwaiting(t *testing.T) {
	uc := NewGetPaymentStatus(newMemoryOrderRepo(completedOrder(t)))
	if _, err := uc.Execute(context.Background(), testOrderID); err != payment.ErrPaymentURLNotAvailable {
		t.Fatalf("expected ErrPaymentURLNotAvailable, got %v", err)
	}
}

func TestGetPaymentStatusReturnsNotFound(t *testing.T) {
	uc := NewGetPaymentStatus(newMemoryOrderRepo())
	if _, err := uc.Execute(context.Background(), testOrderID); err != service_order.ErrServiceOrderNotFound {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestBuildPreferenceItemsFallback(t *testing.T) {
	order := completedOrder(t)
	items := BuildPreferenceItems(order)
	if len(items) != 1 || items[0].Title == "" || items[0].Quantity != 1 {
		t.Fatalf("unexpected fallback items: %+v", items)
	}
}

func TestCreatePaymentPreferenceBuildsItemsInBRL(t *testing.T) {
	order := completedOrder(t) //nolint:ineffassign,staticcheck // value is overwritten by ReconstructServiceOrder; call needed for store side-effect
	item, err := service_order.NewServiceOrderItem(testOrderID, service_order.ItemTypeService, "44444444-4444-4444-8444-444444444444", "Troca de óleo", 2, 12345)
	if err != nil {
		t.Fatalf("NewServiceOrderItem() error = %v", err)
	}
	if err := item.SetID("55555555-5555-4555-8555-555555555555"); err != nil {
		t.Fatalf("SetID() error = %v", err)
	}
	order, err = service_order.ReconstructServiceOrder(
		testOrderID, testCustomerID, testVehicleID, "test", service_order.StatusCompleted, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil, []*service_order.ServiceOrderItem{item}, nil, time.Now(), time.Now(), nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	client := &fakeMPClient{preference: &payment.Preference{ID: "pref", InitURL: "https://pay"}}
	uc := NewCreatePaymentPreference(client)

	if _, err := uc.Execute(context.Background(), order); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(client.items) != 1 || client.items[0].UnitPrice != 123.45 || client.externalRef != testOrderID {
		t.Fatalf("unexpected preference items: %+v external=%s", client.items, client.externalRef)
	}
}

type fakeMPClient struct {
	preference  *payment.Preference
	payment     *payment.Payment
	items       []payment.PreferenceItem
	externalRef string
}

func (c *fakeMPClient) CreatePreference(_ context.Context, _ string, items []payment.PreferenceItem, externalRef string) (*payment.Preference, error) {
	c.items = items
	c.externalRef = externalRef
	return c.preference, nil
}

func (c *fakeMPClient) GetPayment(context.Context, string) (*payment.Payment, error) {
	return c.payment, nil
}

type fakeCustomerAdapter struct {
	err error
}

func (a *fakeCustomerAdapter) GetCustomerByID(context.Context, string) (*dto.CustomerDTO, error) {
	if a.err != nil {
		return nil, a.err
	}
	return &dto.CustomerDTO{ID: testCustomerID, Name: "Maria", Email: "maria@example.com"}, nil
}

type memoryOrderRepo struct {
	orders map[string]*service_order.ServiceOrder
}

func newMemoryOrderRepo(orders ...*service_order.ServiceOrder) *memoryOrderRepo {
	repo := &memoryOrderRepo{orders: map[string]*service_order.ServiceOrder{}}
	for _, order := range orders {
		repo.orders[order.ID()] = order
	}
	return repo
}

func (r *memoryOrderRepo) Save(_ context.Context, order *service_order.ServiceOrder) error {
	r.orders[order.ID()] = order
	return nil
}

func (r *memoryOrderRepo) SaveWithItems(ctx context.Context, order *service_order.ServiceOrder) error {
	return r.Save(ctx, order)
}

func (r *memoryOrderRepo) FindByID(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	order, ok := r.orders[id]
	if !ok {
		return nil, service_order.ErrServiceOrderNotFound
	}
	return order, nil
}

func (r *memoryOrderRepo) FindByIDWithItems(ctx context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(ctx, id)
}

func (r *memoryOrderRepo) FindAll(context.Context) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *memoryOrderRepo) FindAllWithFilters(context.Context, service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *memoryOrderRepo) FindByCustomerID(context.Context, string) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *memoryOrderRepo) FindByStatus(context.Context, service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return nil, nil
}

func (r *memoryOrderRepo) FindBySagaStatus(_ context.Context, sagaStatus string) ([]*service_order.ServiceOrder, error) {
	orders := []*service_order.ServiceOrder{}
	for _, order := range r.orders {
		if order.SagaStatus() == sagaStatus {
			orders = append(orders, order)
		}
	}
	return orders, nil
}

func (r *memoryOrderRepo) Delete(context.Context, string) error {
	return nil
}

func (r *memoryOrderRepo) UpdateItemsHistoryID(context.Context, []string, string) error {
	return nil
}

type memoryHistoryRepo struct {
	saved []*service_order.History
	err   error
}

func (r *memoryHistoryRepo) Save(_ context.Context, history *service_order.History) error {
	if r.err != nil {
		return r.err
	}
	r.saved = append(r.saved, history)
	return nil
}

func (r *memoryHistoryRepo) FindByServiceOrderID(context.Context, string) ([]*service_order.History, error) {
	return r.saved, nil
}

func (r *memoryHistoryRepo) FindByID(context.Context, string) (*service_order.History, error) {
	return nil, errors.New("not implemented")
}

func awaitingPaymentOrder(t *testing.T) *service_order.ServiceOrder {
	t.Helper()
	prefID := "pref-1"
	paymentURL := "https://pay/pref-1"
	order, err := service_order.ReconstructServiceOrder(
		testOrderID,
		testCustomerID,
		testVehicleID,
		"test",
		service_order.StatusAwaitingPayment,
		service_order.SagaStatusAwaitingPayment,
		nil,
		nil,
		nil,
		&prefID,
		nil,
		&paymentURL,
		nil,
		nil,
		time.Now(),
		time.Now(),
		nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	return order
}

func completedOrder(t *testing.T) *service_order.ServiceOrder {
	t.Helper()
	order, err := service_order.ReconstructServiceOrder(
		testOrderID,
		testCustomerID,
		testVehicleID,
		"test",
		service_order.StatusCompleted,
		service_order.SagaStatusIdle,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now(),
		time.Now(),
		nil,
	)
	if err != nil {
		t.Fatalf("ReconstructServiceOrder() error = %v", err)
	}
	return order
}
