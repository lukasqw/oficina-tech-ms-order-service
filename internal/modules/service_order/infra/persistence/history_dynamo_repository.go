package persistence

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	awsv2dynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/google/uuid"
	"oficina-tech/internal/modules/service_order/domain/service_order"
	shareddynamo "oficina-tech/internal/shared/infra/dynamodb"
)

type DynamoHistoryRepository struct {
	client    *awsv2dynamodb.Client
	tableName string
}

type dynamoHistoryItem struct {
	OrderID    string         `dynamodbav:"order_id"`
	OccurredAt string         `dynamodbav:"occurred_at"`
	ID         string         `dynamodbav:"id"`
	Status     string         `dynamodbav:"status"`
	Metadata   map[string]any `dynamodbav:"metadata"`
	CreatedAt  string         `dynamodbav:"created_at"`
}

func NewDynamoHistoryRepository(client *awsv2dynamodb.Client) service_order.HistoryRepository {
	return &DynamoHistoryRepository{
		client:    client,
		tableName: shareddynamo.OrderHistoryTableName,
	}
}

func (r *DynamoHistoryRepository) Save(ctx context.Context, history *service_order.History) error {
	if history.ID() == "" {
		if err := history.SetID(uuid.Must(uuid.NewV7()).String()); err != nil {
			return err
		}
	}

	item, err := attributevalue.MarshalMap(toDynamoHistoryItem(history))
	if err != nil {
		return fmt.Errorf("marshal history item: %w", err)
	}

	_, err = r.client.PutItem(ctx, &awsv2dynamodb.PutItemInput{
		TableName: aws.String(r.tableName),
		Item:      item,
	})
	if err != nil {
		return fmt.Errorf("put history item: %w", err)
	}
	return nil
}

func (r *DynamoHistoryRepository) FindByServiceOrderID(ctx context.Context, serviceOrderID string) ([]*service_order.History, error) {
	output, err := r.client.Query(ctx, &awsv2dynamodb.QueryInput{
		TableName:              aws.String(r.tableName),
		KeyConditionExpression: aws.String("order_id = :order_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":order_id": &types.AttributeValueMemberS{Value: serviceOrderID},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, fmt.Errorf("query order history: %w", err)
	}

	histories := make([]*service_order.History, 0, len(output.Items))
	for _, raw := range output.Items {
		history, err := fromDynamoHistoryMap(raw)
		if err != nil {
			return nil, err
		}
		histories = append(histories, history)
	}
	return histories, nil
}

func (r *DynamoHistoryRepository) FindByID(ctx context.Context, id string) (*service_order.History, error) {
	output, err := r.client.Scan(ctx, &awsv2dynamodb.ScanInput{
		TableName:        aws.String(r.tableName),
		FilterExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, fmt.Errorf("scan order history by id: %w", err)
	}
	if len(output.Items) == 0 {
		return nil, service_order.ErrHistoryNotFound
	}
	return fromDynamoHistoryMap(output.Items[0])
}

func toDynamoHistoryItem(history *service_order.History) dynamoHistoryItem {
	occurredAt := history.CreatedAt().UTC().Format(time.RFC3339Nano)
	return dynamoHistoryItem{
		OrderID:    history.ServiceOrderID(),
		OccurredAt: occurredAt,
		ID:         history.ID(),
		Status:     history.Status().String(),
		Metadata:   history.Metadata(),
		CreatedAt:  occurredAt,
	}
}

func fromDynamoHistoryMap(raw map[string]types.AttributeValue) (*service_order.History, error) {
	var item dynamoHistoryItem
	if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
		return nil, fmt.Errorf("unmarshal history item: %w", err)
	}

	createdAt, err := time.Parse(time.RFC3339Nano, item.CreatedAt)
	if err != nil {
		createdAt, err = time.Parse(time.RFC3339, item.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse history timestamp: %w", err)
		}
	}

	status, err := service_order.NewOrderStatus(item.Status)
	if err != nil {
		return nil, err
	}

	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}

	return service_order.ReconstructHistory(
		item.ID,
		item.OrderID,
		item.Metadata,
		status,
		createdAt,
	), nil
}
