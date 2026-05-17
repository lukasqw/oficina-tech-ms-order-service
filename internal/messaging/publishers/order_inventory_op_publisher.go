package publishers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
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
	// Span producer criado antes da injeção — InjectTraceToSQS captura o
	// contexto deste span, que o consumer receberá via MessageAttributes.
	ctx, span := otel.Tracer("oficina-tech/messaging").Start(ctx,
		"publish "+events.EventOrderInventoryOperationRequested,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			semconv.MessagingSystemKey.String("aws_sqs"),
			semconv.MessagingDestinationName(p.queueURL),
			attribute.String("messaging.operation.name", "publish"),
		),
	)
	defer span.End()

	payload, err := json.Marshal(events.OrderInventoryOperationRequested{
		Event:      events.EventOrderInventoryOperationRequested,
		SagaID:     sagaID,
		OrderID:    orderID,
		Operation:  string(operation),
		Items:      items,
		OccurredAt: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal inventory operation request: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:          aws.String(p.queueURL),
		MessageBody:       aws.String(string(payload)),
		MessageAttributes: observability.InjectTraceToSQS(ctx),
	})
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("publish inventory operation request: %w", err)
	}
	return nil
}
