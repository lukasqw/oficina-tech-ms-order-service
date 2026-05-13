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
	"oficina-tech/internal/modules/service_order/application/usecases"
	"oficina-tech/internal/modules/service_order/domain/service_order"
)

type CustomerDeletedConsumer struct {
	client   SQSReceiveDeleteClient
	queueURL string
	repo     service_order.Repository
	deleteUC *usecases.DeleteServiceOrder
}

func NewCustomerDeletedConsumer(client SQSReceiveDeleteClient, queueURL string, repo service_order.Repository, deleteUC *usecases.DeleteServiceOrder) *CustomerDeletedConsumer {
	return &CustomerDeletedConsumer{client: client, queueURL: queueURL, repo: repo, deleteUC: deleteUC}
}

func (c *CustomerDeletedConsumer) Start(ctx context.Context) error {
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
			return fmt.Errorf("receive customer deleted messages: %w", err)
		}
		for _, message := range output.Messages {
			if err := c.HandleMessage(ctx, message); err != nil {
				slog.Error("failed to process customer deleted message", "error", err)
			}
		}
	}
}

func (c *CustomerDeletedConsumer) HandleMessage(ctx context.Context, message types.Message) error {
	event, err := decodeCustomerDeleted(message)
	if err != nil {
		return err
	}

	orders, err := c.repo.FindByCustomerID(ctx, event.CustomerID)
	if err != nil {
		return err
	}
	note := "Cliente removido"
	for _, order := range orders {
		if order.SagaStatus() == service_order.SagaStatusAwaitingInventory {
			continue
		}
		if isOpenForCustomerDeletion(order.Status()) {
			if _, err := c.deleteUC.Execute(ctx, usecases.DeleteServiceOrderInput{ID: order.ID(), Notes: &note}); err != nil {
				return err
			}
		}
	}

	if message.ReceiptHandle == nil {
		return fmt.Errorf("missing SQS receipt handle")
	}
	_, err = c.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(c.queueURL),
		ReceiptHandle: message.ReceiptHandle,
	})
	return err
}

func decodeCustomerDeleted(message types.Message) (events.CustomerDeleted, error) {
	if message.Body == nil {
		return events.CustomerDeleted{}, fmt.Errorf("empty SQS message body")
	}
	var event events.CustomerDeleted
	if err := json.Unmarshal([]byte(*message.Body), &event); err != nil {
		return event, fmt.Errorf("decode customer deleted event: %w", err)
	}
	if event.Event != events.EventCustomerDeleted {
		return event, fmt.Errorf("unexpected event %q", event.Event)
	}
	if event.CustomerID == "" {
		return event, fmt.Errorf("missing customer_id")
	}
	if _, err := time.Parse(time.RFC3339, event.OccurredAt); err != nil {
		return event, fmt.Errorf("invalid occurred_at: %w", err)
	}
	return event, nil
}

func isOpenForCustomerDeletion(status service_order.OrderStatus) bool {
	switch status {
	case service_order.StatusReceived,
		service_order.StatusDiagnosing,
		service_order.StatusPendingAuthorization,
		service_order.StatusAuthorized,
		service_order.StatusInProgress:
		return true
	default:
		return false
	}
}
