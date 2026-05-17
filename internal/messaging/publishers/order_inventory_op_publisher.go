package publishers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"oficina-tech/internal/messaging/events"
	"oficina-tech/internal/shared/dto"
	"oficina-tech/internal/shared/infra/observability"
)

type SQSSendMessageClient interface {
	SendMessage(ctx context.Context, params *sqs.SendMessageInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageOutput, error)
}

type OrderInventoryOperationPublisher struct {
	client   SQSSendMessageClient
	queueURL string
}

func NewOrderInventoryOperationPublisher(client SQSSendMessageClient, queueURL string) *OrderInventoryOperationPublisher {
	return &OrderInventoryOperationPublisher{
		client:   client,
		queueURL: queueURL,
	}
}

func (p *OrderInventoryOperationPublisher) PublishRequest(
	ctx context.Context,
	sagaID string,
	orderID string,
	operation dto.StockOperationType,
	items []events.InventoryItem,
) error {
	payload, err := json.Marshal(events.OrderInventoryOperationRequested{
		Event:      events.EventOrderInventoryOperationRequested,
		SagaID:     sagaID,
		OrderID:    orderID,
		Operation:  string(operation),
		Items:      items,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return fmt.Errorf("marshal inventory operation request: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(p.queueURL),
		MessageBody:       aws.String(string(payload)),
		MessageAttributes: observability.InjectTraceToSQS(ctx),
	})
	if err != nil {
		return fmt.Errorf("publish inventory operation request: %w", err)
	}
	return nil
}
