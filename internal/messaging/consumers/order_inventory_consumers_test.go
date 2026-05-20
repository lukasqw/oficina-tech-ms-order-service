package consumers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"go.opentelemetry.io/otel"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	"oficina-tech/internal/shared/infra/observability"
)

func TestSucceededConsumerProcessesCurrentSagaAndDeletes(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "success-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationSucceeded",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if handler.succeededCalls != 1 || client.deleted != 1 {
		t.Fatalf("succeededCalls=%d deleted=%d", handler.succeededCalls, client.deleted)
	}
}

func TestSucceededConsumerDiscardsDuplicateSaga(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: false}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "success-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationSucceeded",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if handler.succeededCalls != 0 || client.deleted != 1 {
		t.Fatalf("duplicate should be acked only, calls=%d deleted=%d", handler.succeededCalls, client.deleted)
	}
}

func TestFailedConsumerLeavesMessageOnTransientError(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true, err: errors.New("database unavailable")}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "failed-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationFailed",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"reason":"insufficient stock",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err == nil {
		t.Fatalf("expected transient error")
	}
	if client.deleted != 0 {
		t.Fatalf("message should not be deleted on transient error")
	}
}

func TestFailedConsumerProcessesCurrentSagaAndDeletes(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "failed-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationFailed",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"reason":"insufficient stock",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err != nil {
		t.Fatalf("HandleMessage() error = %v", err)
	}
	if handler.failedCalls != 1 || client.deleted != 1 {
		t.Fatalf("failedCalls=%d deleted=%d", handler.failedCalls, client.deleted)
	}
}

func TestSucceededConsumerDoesNotDeleteWhenHandlerFails(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true, err: errors.New("database unavailable")}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "success-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationSucceeded",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err == nil {
		t.Fatalf("expected transient error")
	}
	if client.deleted != 0 {
		t.Fatalf("message should not be deleted on transient error")
	}
}

func TestFailedConsumerRejectsInvalidPayloadWithoutDeleting(t *testing.T) {
	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "failed-url", handler)

	err := consumer.HandleMessage(context.Background(), sqsMessage(`{"event":"WrongEvent"}`))
	if err == nil {
		t.Fatalf("expected decode error")
	}
	if client.deleted != 0 {
		t.Fatalf("invalid message should not be deleted")
	}
}

func TestSucceededConsumerRejectsWrongEvent(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{current: true})
	err := consumer.HandleMessage(context.Background(), sqsMessage(`{"event":"WrongEvent","occurred_at":"2026-04-30T19:21:04Z"}`))
	if err == nil {
		t.Fatal("expected decode error for wrong event type")
	}
	if client.deleted != 0 {
		t.Fatalf("should not delete on decode error")
	}
}

func TestSucceededConsumerRejectsNilBody(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{})
	msg := types.Message{ReceiptHandle: aws.String("rh-1")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestSucceededConsumerRejectsBadJSON(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{})
	if err := consumer.HandleMessage(context.Background(), sqsMessage(`{bad json}`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestSucceededConsumerRejectsInvalidOccurredAt(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{})
	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationSucceeded",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"occurred_at":"not-a-date"
	}`))
	if err == nil {
		t.Fatal("expected error for invalid occurred_at")
	}
}

func TestSucceededConsumerReturnsErrorWhenIsCurrentSagaFails(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeIsCurrentSagaErrHandler{})
	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationSucceeded",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err == nil {
		t.Fatal("expected error from IsCurrentSaga")
	}
}

func TestSucceededConsumerNilReceiptHandleErrors(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{current: false})
	msg := types.Message{
		Body:          aws.String(`{"event":"OrderInventoryOperationSucceeded","saga_id":"11111111-1111-4111-8111-111111111111","order_id":"22222222-2222-4222-8222-222222222222","operation":"RESERVE","occurred_at":"2026-04-30T19:21:04Z"}`),
		ReceiptHandle: nil,
	}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for nil receipt handle")
	}
}

func TestFailedConsumerRejectsNilBody(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{})
	msg := types.Message{ReceiptHandle: aws.String("rh-1")}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for nil body")
	}
}

func TestFailedConsumerRejectsBadJSON(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{})
	if err := consumer.HandleMessage(context.Background(), sqsMessage(`{bad json}`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestFailedConsumerRejectsEmptyReason(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{})
	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationFailed",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"reason":"",
		"occurred_at":"2026-04-30T19:21:04Z"
	}`))
	if err == nil {
		t.Fatal("expected error for empty reason")
	}
}

func TestFailedConsumerRejectsInvalidOccurredAt(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{})
	err := consumer.HandleMessage(context.Background(), sqsMessage(`{
		"event":"OrderInventoryOperationFailed",
		"saga_id":"11111111-1111-4111-8111-111111111111",
		"order_id":"22222222-2222-4222-8222-222222222222",
		"operation":"RESERVE",
		"reason":"insufficient stock",
		"occurred_at":"not-a-date"
	}`))
	if err == nil {
		t.Fatal("expected error for invalid occurred_at")
	}
}

func TestFailedConsumerNilReceiptHandleErrors(t *testing.T) {
	client := &fakeSQSClient{}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{current: false})
	msg := types.Message{
		Body:          aws.String(`{"event":"OrderInventoryOperationFailed","saga_id":"11111111-1111-4111-8111-111111111111","order_id":"22222222-2222-4222-8222-222222222222","operation":"RESERVE","reason":"gone","occurred_at":"2026-04-30T19:21:04Z"}`),
		ReceiptHandle: nil,
	}
	if err := consumer.HandleMessage(context.Background(), msg); err == nil {
		t.Fatal("expected error for nil receipt handle")
	}
}

// --- Start function tests ---

type errorSQSClient struct{ receiveErr error }

func (c *errorSQSClient) ReceiveMessage(_ context.Context, _ *sqs.ReceiveMessageInput, _ ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return nil, c.receiveErr
}
func (c *errorSQSClient) DeleteMessage(_ context.Context, _ *sqs.DeleteMessageInput, _ ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	return &sqs.DeleteMessageOutput{}, nil
}

func TestSucceededConsumerStart_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	consumer := NewOrderInventoryOperationSucceededConsumer(&fakeSQSClient{}, "q", &fakeSagaHandler{})
	if err := consumer.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestSucceededConsumerStart_ReceiveError(t *testing.T) {
	client := &errorSQSClient{receiveErr: errors.New("network error")}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "q", &fakeSagaHandler{})
	if err := consumer.Start(context.Background()); err == nil {
		t.Fatal("expected error from ReceiveMessage failure")
	}
}

func TestFailedConsumerStart_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	consumer := NewOrderInventoryOperationFailedConsumer(&fakeSQSClient{}, "q", &fakeSagaHandler{})
	if err := consumer.Start(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestFailedConsumerStart_ReceiveError(t *testing.T) {
	client := &errorSQSClient{receiveErr: errors.New("network error")}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "q", &fakeSagaHandler{})
	if err := consumer.Start(context.Background()); err == nil {
		t.Fatal("expected error from ReceiveMessage failure")
	}
}

type fakeIsCurrentSagaErrHandler struct{}

func (h *fakeIsCurrentSagaErrHandler) IsCurrentSaga(context.Context, string, string) (bool, error) {
	return false, errors.New("db unavailable")
}
func (h *fakeIsCurrentSagaErrHandler) HandleSucceeded(context.Context, events.OrderInventoryOperationSucceeded) error {
	return nil
}

func TestFailedConsumerWithTracePropagation(t *testing.T) {
	// Message with injected OTel trace context so ExtractSpanLinkFromSQS returns ok=true.
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "producer")
	defer span.End()

	attrs := observability.InjectTraceToSQS(ctx)
	msg := types.Message{
		Body: aws.String(`{
			"event":"OrderInventoryOperationFailed",
			"saga_id":"11111111-1111-4111-8111-111111111111",
			"order_id":"22222222-2222-4222-8222-222222222222",
			"operation":"RESERVE",
			"reason":"insufficient stock",
			"occurred_at":"2026-04-30T19:21:04Z"
		}`),
		ReceiptHandle:     aws.String("rh-trace"),
		MessageAttributes: attrs,
	}

	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true}
	consumer := NewOrderInventoryOperationFailedConsumer(client, "failed-url", handler)

	if err := consumer.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("HandleMessage() with trace propagation error = %v", err)
	}
}

func TestSucceededConsumerWithTracePropagation(t *testing.T) {
	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "producer")
	defer span.End()

	attrs := observability.InjectTraceToSQS(ctx)
	msg := types.Message{
		Body: aws.String(`{
			"event":"OrderInventoryOperationSucceeded",
			"saga_id":"11111111-1111-4111-8111-111111111111",
			"order_id":"22222222-2222-4222-8222-222222222222",
			"operation":"RESERVE",
			"occurred_at":"2026-04-30T19:21:04Z"
		}`),
		ReceiptHandle:     aws.String("rh-trace"),
		MessageAttributes: attrs,
	}

	client := &fakeSQSClient{}
	handler := &fakeSagaHandler{current: true}
	consumer := NewOrderInventoryOperationSucceededConsumer(client, "success-url", handler)

	if err := consumer.HandleMessage(context.Background(), msg); err != nil {
		t.Fatalf("HandleMessage() with trace propagation error = %v", err)
	}
}

func TestCustomerDeletedConsumerCancelsOpenOrdersOnly(t *testing.T) {
	if !isOpenForCustomerDeletion(service_order.StatusAuthorized) ||
		!isOpenForCustomerDeletion(service_order.StatusInProgress) ||
		isOpenForCustomerDeletion(service_order.StatusDelivered) ||
		isOpenForCustomerDeletion(service_order.StatusCanceled) {
		t.Fatalf("customer deletion status predicate is wrong")
	}
}

type fakeSQSClient struct {
	deleted int
}

func (c *fakeSQSClient) ReceiveMessage(context.Context, *sqs.ReceiveMessageInput, ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error) {
	return &sqs.ReceiveMessageOutput{}, nil
}

func (c *fakeSQSClient) DeleteMessage(context.Context, *sqs.DeleteMessageInput, ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error) {
	c.deleted++
	return &sqs.DeleteMessageOutput{}, nil
}

type fakeSagaHandler struct {
	current        bool
	err            error
	succeededCalls int
	failedCalls    int
}

func (h *fakeSagaHandler) IsCurrentSaga(context.Context, string, string) (bool, error) {
	return h.current, nil
}

func (h *fakeSagaHandler) HandleSucceeded(context.Context, events.OrderInventoryOperationSucceeded) error {
	h.succeededCalls++
	return h.err
}

func (h *fakeSagaHandler) HandleFailed(context.Context, events.OrderInventoryOperationFailed) error {
	h.failedCalls++
	return h.err
}

func sqsMessage(body string) types.Message {
	return types.Message{
		Body:          aws.String(body),
		ReceiptHandle: aws.String("receipt-" + time.Now().Format(time.RFC3339Nano)),
	}
}
