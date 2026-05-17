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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/shared/infra/observability"
)

type InventorySagaFailedHandler interface {
	IsCurrentSaga(ctx context.Context, orderID, sagaID string) (bool, error)
	HandleFailed(ctx context.Context, event events.OrderInventoryOperationFailed) error
}

type OrderInventoryOperationFailedConsumer struct {
	client   SQSReceiveDeleteClient
	queueURL string
	handler  InventorySagaFailedHandler
}

func NewOrderInventoryOperationFailedConsumer(client SQSReceiveDeleteClient, queueURL string, handler InventorySagaFailedHandler) *OrderInventoryOperationFailedConsumer {
	return &OrderInventoryOperationFailedConsumer{client: client, queueURL: queueURL, handler: handler}
}

func (c *OrderInventoryOperationFailedConsumer) Start(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		output, err := c.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
			QueueUrl:              aws.String(c.queueURL),
			WaitTimeSeconds:       20,
			MaxNumberOfMessages:   10,
			MessageAttributeNames: []string{"All"},
		})
		if err != nil {
			return fmt.Errorf("receive inventory failure messages: %w", err)
		}
		for _, message := range output.Messages {
			if err := c.HandleMessage(ctx, message); err != nil {
				slog.Error("failed to process inventory failure message", "error", err)
			}
		}
	}
}

func (c *OrderInventoryOperationFailedConsumer) HandleMessage(ctx context.Context, message types.Message) error {
	opts := []trace.SpanStartOption{trace.WithSpanKind(trace.SpanKindConsumer)}
	if link, ok := observability.ExtractSpanLinkFromSQS(message); ok {
		opts = append(opts, trace.WithLinks(link))
	}
	ctx, span := otel.Tracer("oficina-tech/messaging").Start(ctx,
		"consume "+events.EventOrderInventoryOperationFailed, opts...)
	defer span.End()

	event, err := decodeFailed(message)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	current, err := c.handler.IsCurrentSaga(ctx, event.OrderID, event.SagaID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if current {
		if err := c.handler.HandleFailed(ctx, event); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	return c.deleteMessage(ctx, message)
}

func (c *OrderInventoryOperationFailedConsumer) deleteMessage(ctx context.Context, message types.Message) error {
	if message.ReceiptHandle == nil {
		return fmt.Errorf("missing SQS receipt handle")
	}
	_, err := c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	})
	return err
}

func decodeFailed(message types.Message) (events.OrderInventoryOperationFailed, error) {
	if message.Body == nil {
		return events.OrderInventoryOperationFailed{}, fmt.Errorf("empty SQS message body")
	}
	var event events.OrderInventoryOperationFailed
	if err := json.Unmarshal([]byte(*message.Body), &event); err != nil {
		return event, fmt.Errorf("decode inventory failure event: %w", err)
	}
	if event.Event != events.EventOrderInventoryOperationFailed {
		return event, fmt.Errorf("unexpected event %q", event.Event)
	}
	if event.Reason == "" {
		return event, fmt.Errorf("missing failure reason")
	}
	if _, err := time.Parse(time.RFC3339, event.OccurredAt); err != nil {
		return event, fmt.Errorf("invalid occurred_at: %w", err)
	}
	return event, nil
}
