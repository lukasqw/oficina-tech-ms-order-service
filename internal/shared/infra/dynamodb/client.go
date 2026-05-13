package dynamodb

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv2dynamodb "github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

const OrderHistoryTableName = "order_history"

type Client = awsv2dynamodb.Client

func NewClient(cfg aws.Config) *awsv2dynamodb.Client {
	return awsv2dynamodb.NewFromConfig(cfg)
}

func EnsureOrderHistoryTable(ctx context.Context, client *awsv2dynamodb.Client) error {
	_, err := client.DescribeTable(ctx, &awsv2dynamodb.DescribeTableInput{
		TableName: aws.String(OrderHistoryTableName),
	})
	if err == nil {
		return nil
	}

	var notFound *types.ResourceNotFoundException
	if !errors.As(err, &notFound) {
		return fmt.Errorf("describe DynamoDB table %s: %w", OrderHistoryTableName, err)
	}

	_, err = client.CreateTable(ctx, &awsv2dynamodb.CreateTableInput{
		TableName: aws.String(OrderHistoryTableName),
		AttributeDefinitions: []types.AttributeDefinition{
			{
				AttributeName: aws.String("order_id"),
				AttributeType: types.ScalarAttributeTypeS,
			},
			{
				AttributeName: aws.String("occurred_at"),
				AttributeType: types.ScalarAttributeTypeS,
			},
		},
		KeySchema: []types.KeySchemaElement{
			{
				AttributeName: aws.String("order_id"),
				KeyType:       types.KeyTypeHash,
			},
			{
				AttributeName: aws.String("occurred_at"),
				KeyType:       types.KeyTypeRange,
			},
		},
		BillingMode: types.BillingModePayPerRequest,
	})
	if err != nil {
		// ResourceInUseException means the table is already being created by another
		// process (e.g. the LocalStack init script). Just wait for it to become active.
		var inUse *types.ResourceInUseException
		if !errors.As(err, &inUse) {
			return fmt.Errorf("create DynamoDB table %s: %w", OrderHistoryTableName, err)
		}
		slog.Info("DynamoDB table already being created by another process, waiting", "table", OrderHistoryTableName)
	}

	waiter := awsv2dynamodb.NewTableExistsWaiter(client)
	if err := waiter.Wait(ctx, &awsv2dynamodb.DescribeTableInput{
		TableName: aws.String(OrderHistoryTableName),
	}, 30*time.Second); err != nil {
		return fmt.Errorf("wait DynamoDB table %s: %w", OrderHistoryTableName, err)
	}

	slog.Info("DynamoDB table ready", "table", OrderHistoryTableName)
	return nil
}
