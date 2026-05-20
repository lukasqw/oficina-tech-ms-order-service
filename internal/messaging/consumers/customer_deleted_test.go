package consumers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

// customerDeletedMockRepo implements service_order.Repository for customer_deleted tests.
type customerDeletedMockRepo struct {
	orders  []*service_order.ServiceOrder
	findErr error
}

func (r *customerDeletedMockRepo) Save(_ context.Context, _ *service_order.ServiceOrder) error {
	return nil
}
func (r *customerDeletedMockRepo) SaveWithItems(_ context.Context, _ *service_order.ServiceOrder) error {
	return nil
}
func (r *customerDeletedMockRepo) FindByID(_ context.Context, _ string) (*service_order.ServiceOrder, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	if len(r.orders) > 0 {
		return r.orders[0], nil
	}
	return nil, service_order.ErrServiceOrderNotFound
}
func (r *customerDeletedMockRepo) FindByIDWithItems(_ context.Context, id string) (*service_order.ServiceOrder, error) {
	return r.FindByID(context.Background(), id)
}
func (r *customerDeletedMockRepo) FindAll(_ context.Context) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *customerDeletedMockRepo) FindAllWithFilters(_ context.Context, _ service_order.RepositoryFilters) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *customerDeletedMockRepo) FindByCustomerID(_ context.Context, _ string) ([]*service_order.ServiceOrder, error) {
	if r.findErr != nil {
		return nil, r.findErr
	}
	return r.orders, nil
}
func (r *customerDeletedMockRepo) FindByStatus(_ context.Context, _ service_order.OrderStatus) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *customerDeletedMockRepo) FindBySagaStatus(_ context.Context, _ string) ([]*service_order.ServiceOrder, error) {
	return r.orders, r.findErr
}
func (r *customerDeletedMockRepo) Delete(_ context.Context, _ string) error { return nil }
func (r *customerDeletedMockRepo) UpdateItemsHistoryID(_ context.Context, _ []string, _ string) error {
	return nil
}

func validCustomerDeletedBody() string {
	return `{"event":"CustomerDeleted","customer_id":"cust-abc","occurred_at":"2026-01-01T00:00:00Z"}`
}

// TestDecodeCustomerDeleted_NilBody exercises HandleMessage → decodeCustomerDeleted nil-body path.
func TestDecodeCustomerDeleted_NilBody(t *testing.T) {
	repo := &customerDeletedMockRepo{}
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "queue", repo, nil)

	msg := types.Message{Body: nil, ReceiptHandle: aws.String("handle")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestDecodeCustomerDeleted_InvalidJSON(t *testing.T) {
	repo := &customerDeletedMockRepo{}
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "queue", repo, nil)

	msg := types.Message{Body: aws.String("not-json"), ReceiptHandle: aws.String("handle")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecodeCustomerDeleted_WrongEventType(t *testing.T) {
	repo := &customerDeletedMockRepo{}
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "queue", repo, nil)

	body := `{"event":"OrderInventoryOperationSucceeded","customer_id":"cust-abc","occurred_at":"2026-01-01T00:00:00Z"}`
	msg := types.Message{Body: aws.String(body), ReceiptHandle: aws.String("handle")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for wrong event type")
	}
}

func TestDecodeCustomerDeleted_MissingCustomerID(t *testing.T) {
	repo := &customerDeletedMockRepo{}
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "queue", repo, nil)

	body := `{"event":"CustomerDeleted","customer_id":"","occurred_at":"2026-01-01T00:00:00Z"}`
	msg := types.Message{Body: aws.String(body), ReceiptHandle: aws.String("handle")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing customer_id")
	}
}

func TestDecodeCustomerDeleted_InvalidOccurredAt(t *testing.T) {
	repo := &customerDeletedMockRepo{}
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "queue", repo, nil)

	body := `{"event":"CustomerDeleted","customer_id":"cust-abc","occurred_at":"not-a-date"}`
	msg := types.Message{Body: aws.String(body), ReceiptHandle: aws.String("handle")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for invalid occurred_at")
	}
}

// TestHandleMessage_NoOrders: valid event, repo returns no orders → only SQS delete is called.
func TestHandleMessage_NoOrders_DeletesMessage(t *testing.T) {
	client := &fakeSQSClient{}
	repo := &customerDeletedMockRepo{orders: nil}
	consumer := NewCustomerDeletedConsumer(client, "queue", repo, nil)

	msg := types.Message{
		Body:          aws.String(validCustomerDeletedBody()),
		ReceiptHandle: aws.String("handle"),
	}
	if err := consumer.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.deleted != 1 {
		t.Errorf("expected 1 SQS delete call, got %d", client.deleted)
	}
}

// TestHandleMessage_RepoError: repo returns error → HandleMessage returns error.
func TestHandleMessage_RepoError(t *testing.T) {
	client := &fakeSQSClient{}
	repo := &customerDeletedMockRepo{findErr: errors.New("db down")}
	consumer := NewCustomerDeletedConsumer(client, "queue", repo, nil)

	msg := types.Message{
		Body:          aws.String(validCustomerDeletedBody()),
		ReceiptHandle: aws.String("handle"),
	}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error from repo")
	}
}

// TestHandleMessage_OrderNotOpenForDeletion: order in StatusCanceled → isOpenForCustomerDeletion=false
// → loop runs but deleteUC is never called → message deleted normally.
func TestHandleMessage_OrderNotOpenForDeletion(t *testing.T) {
	now := time.Now()
	canceledOrder, _ := service_order.ReconstructServiceOrder(
		"order-1", "cust-abc", "veh-1", "desc",
		service_order.StatusCanceled, service_order.SagaStatusIdle,
		nil, nil, nil, nil, nil, nil,
		nil, nil, now, now, nil,
	)
	client := &fakeSQSClient{}
	repo := &customerDeletedMockRepo{orders: []*service_order.ServiceOrder{canceledOrder}}
	consumer := NewCustomerDeletedConsumer(client, "queue", repo, nil)

	msg := types.Message{
		Body:          aws.String(validCustomerDeletedBody()),
		ReceiptHandle: aws.String("handle"),
	}
	if err := consumer.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.deleted != 1 {
		t.Errorf("expected 1 SQS delete, got %d", client.deleted)
	}
}

// TestHandleMessage_OrderAwaitingInventory_Skipped: order in SagaStatusAwaitingInventory
// → continue branch is taken → deleteUC never called → message deleted normally.
func TestHandleMessage_OrderAwaitingInventory_Skipped(t *testing.T) {
	now := time.Now()
	awaitingOrder, _ := service_order.ReconstructServiceOrder(
		"order-2", "cust-abc", "veh-1", "desc",
		service_order.StatusPendingAuthorization, service_order.SagaStatusAwaitingInventory,
		nil, nil, nil, nil, nil, nil,
		nil, nil, now, now, nil,
	)
	client := &fakeSQSClient{}
	repo := &customerDeletedMockRepo{orders: []*service_order.ServiceOrder{awaitingOrder}}
	consumer := NewCustomerDeletedConsumer(client, "queue", repo, nil)

	msg := types.Message{
		Body:          aws.String(validCustomerDeletedBody()),
		ReceiptHandle: aws.String("handle"),
	}
	if err := consumer.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.deleted != 1 {
		t.Errorf("expected 1 SQS delete, got %d", client.deleted)
	}
}

// --- Start function tests ---

func TestCustomerDeletedConsumerStart_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	consumer := NewCustomerDeletedConsumer(&fakeSQSClient{}, "q", &customerDeletedMockRepo{}, nil)
	if err := consumer.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestCustomerDeletedConsumerStart_ReceiveError(t *testing.T) {
	consumer := NewCustomerDeletedConsumer(&errorSQSClient{receiveErr: errors.New("net error")}, "q", &customerDeletedMockRepo{}, nil)
	if err := consumer.Start(context.Background()); err == nil {
		t.Fatal("expected error from ReceiveMessage failure")
	}
}

// TestHandleMessage_NoReceiptHandle: valid event, no orders, but no receipt handle → error.
func TestHandleMessage_NoReceiptHandle(t *testing.T) {
	client := &fakeSQSClient{}
	repo := &customerDeletedMockRepo{orders: nil}
	consumer := NewCustomerDeletedConsumer(client, "queue", repo, nil)

	msg := types.Message{
		Body:          aws.String(validCustomerDeletedBody()),
		ReceiptHandle: nil,
	}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for missing receipt handle")
	}
}
