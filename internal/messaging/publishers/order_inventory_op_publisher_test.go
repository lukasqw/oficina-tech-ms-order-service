package publishers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/shared/dto"
)

func TestPublishRequestSendsContractPayload(t *testing.T) {
	client := &fakeSendClient{}
	publisher := NewOrderInventoryOperationPublisher(client, "queue-url")

	err := publisher.PublishRequest(context.Background(),
		"11111111-1111-4111-8111-111111111111",
		"22222222-2222-4222-8222-222222222222",
		dto.StockOpReserve,
		[]events.InventoryItem{{ProductID: "33333333-3333-4333-8333-333333333333", Quantity: 2}},
	)
	if err != nil {
		t.Fatalf("PublishRequest() error = %v", err)
	}
	if client.input == nil || *client.input.QueueUrl != "queue-url" {
		t.Fatalf("unexpected SQS input: %+v", client.input)
	}

	var payload events.OrderInventoryOperationRequested
	if err := json.Unmarshal([]byte(*client.input.MessageBody), &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if payload.Event != events.EventOrderInventoryOperationRequested ||
		payload.Operation != string(dto.StockOpReserve) ||
		len(payload.Items) != 1 ||
		payload.Items[0].Quantity != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
}

func TestPublishRequestReturnsSQSError(t *testing.T) {
	client := &fakeSendClient{err: errors.New("sqs down")}
	publisher := NewOrderInventoryOperationPublisher(client, "queue-url")

	err := publisher.PublishRequest(context.Background(), "saga", "order", dto.StockOpReserve, nil)
	if err == nil {
		t.Fatalf("expected SQS error")
	}
}

type fakeSendClient struct {
	input *sqs.SendMessageInput
	err   error
}

func (c *fakeSendClient) SendMessage(_ context.Context, input *sqs.SendMessageInput, _ ...func(*sqs.Options)) (*sqs.SendMessageOutput, error) {
	if c.err != nil {
		return nil, c.err
	}
	c.input = input
	return &sqs.SendMessageOutput{}, nil
}
