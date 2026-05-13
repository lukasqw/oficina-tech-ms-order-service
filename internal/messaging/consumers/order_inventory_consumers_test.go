package consumers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/modules/service_order/domain/service_order"
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
