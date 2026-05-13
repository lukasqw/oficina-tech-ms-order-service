package consumers

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"oficina-tech/internal/messaging/events"
)

type SQSReceiveDeleteClient interface {
	ReceiveMessage(ctx context.Context, params *sqs.ReceiveMessageInput, optFns ...func(*sqs.Options)) (*sqs.ReceiveMessageOutput, error)
	DeleteMessage(ctx context.Context, params *sqs.DeleteMessageInput, optFns ...func(*sqs.Options)) (*sqs.DeleteMessageOutput, error)
}

type InventorySagaSucceededHandler interface {
	IsCurrentSaga(ctx context.Context, orderID, sagaID string) (bool, error)
	HandleSucceeded(ctx context.Context, event events.OrderInventoryOperationSucceeded) error
}

type OrderInventoryOperationSucceededConsumer struct {
	client   SQSReceiveDeleteClient
	queueURL string
	handler  InventorySagaSucceededHandler
}

func NewOrderInventoryOperationSucceededConsumer(client SQSReceiveDeleteClient, queueURL string, handler InventorySagaSucceededHandler) *OrderInventoryOperationSucceededConsumer {
	return &OrderInventoryOperationSucceededConsumer{client: client, queueURL: queueURL, handler: handler}
}

func (c *OrderInventoryOperationSucceededConsumer) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:            aws.String(c.queueURL),
			WaitTimeSeconds:     20,
			MaxNumberOfMessages: 10,
		})
		if err != nil {
			return fmt.Errorf("receive inventory success messages: %w", err)
		}
		for _, message := range output.Messages {
			if err := c.HandleMessage(ctx, message); err != nil {
				slog.Error("failed to process inventory success message", "error", err)
			}
		}
	}
}

func (c *OrderInventoryOperationSucceededConsumer) HandleMessage(ctx context.Context, message types.Message) error {
	event, err := decodeSucceeded(message)
	if err != nil {
		return err
	}

	current, err := c.handler.IsCurrentSaga(ctx, event.OrderID, event.SagaID)
	if err != nil {
		return err
	}
	if current {
		if err := c.handler.HandleSucceeded(ctx, event); err != nil {
			return err
		}
	}
	return c.deleteMessage(ctx, message)
}

func (c *OrderInventoryOperationSucceededConsumer) deleteMessage(ctx context.Context, message types.Message) error {
	if message.ReceiptHandle == nil {
		return fmt.Errorf("missing SQS receipt handle")
	}
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	})
	return err
}

func decodeSucceeded(message types.Message) (events.OrderInventoryOperationSucceeded, error) {
	if message.Body == nil {
		return events.OrderInventoryOperationSucceeded{}, fmt.Errorf("empty SQS message body")
	}
	var event events.OrderInventoryOperationSucceeded
	if err := json.Unmarshal([]byte(*message.Body), &event); err != nil {
		return event, fmt.Errorf("decode inventory success event: %w", err)
	}
	if event.Event != events.EventOrderInventoryOperationSucceeded {
		return event, fmt.Errorf("unexpected event %q", event.Event)
	}
	if _, err := time.Parse(time.RFC3339, event.OccurredAt); err != nil {
		return event, fmt.Errorf("invalid occurred_at: %w", err)
	}
	return event, nil
}
