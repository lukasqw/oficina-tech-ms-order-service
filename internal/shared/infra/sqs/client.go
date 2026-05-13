package sqsinfra

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsv2sqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

func NewClient(cfg aws.Config) *awsv2sqs.Client {
	return awsv2sqs.NewFromConfig(cfg)
}

type Client = awsv2sqs.Client

func ResolveQueueURL(ctx context.Context, client *awsv2sqs.Client, name string) (string, error) {
	if value := queueURLEnv(name); value != "" {
		return value, nil
	}

	output, err := client.GetQueueUrl(ctx, &awsv2sqs.GetQueueUrlInput{
		QueueName: aws.String(name),
	})
	if err == nil && output.QueueUrl != nil {
		return *output.QueueUrl, nil
	}

	var notFound *types.QueueDoesNotExist
	if !errors.As(err, &notFound) {
		return "", fmt.Errorf("get SQS queue %s: %w", name, err)
	}
	if os.Getenv("AWS_ENDPOINT_URL") == "" && os.Getenv("SQS_AUTO_CREATE_QUEUES") != "true" {
		return "", fmt.Errorf("SQS queue %s does not exist", name)
	}

	return createQueueWithDLQ(ctx, client, name)
}

func ResolveQueueURLs(ctx context.Context, client *awsv2sqs.Client, names ...string) (map[string]string, error) {
	urls := make(map[string]string, len(names))
	for _, name := range names {
		url, err := ResolveQueueURL(ctx, client, name)
		if err != nil {
			return nil, err
		}
		urls[name] = url
	}
	return urls, nil
}

func createQueueWithDLQ(ctx context.Context, client *awsv2sqs.Client, name string) (string, error) {
	dlqName := name + "-dlq"
	dlqOutput, err := client.CreateQueue(ctx, &awsv2sqs.CreateQueueInput{
		QueueName: aws.String(dlqName),
		Attributes: map[string]string{
			string(types.QueueAttributeNameMessageRetentionPeriod): "1209600",
		},
	})
	if err != nil {
		return "", fmt.Errorf("create SQS DLQ %s: %w", dlqName, err)
	}

	attrs, err := client.GetQueueAttributes(ctx, &awsv2sqs.GetQueueAttributesInput{
		QueueUrl:       dlqOutput.QueueUrl,
		AttributeNames: []types.QueueAttributeName{types.QueueAttributeNameQueueArn},
	})
	if err != nil {
		return "", fmt.Errorf("get SQS DLQ attributes %s: %w", dlqName, err)
	}

	redrive, err := json.Marshal(map[string]any{
		"deadLetterTargetArn": attrs.Attributes[string(types.QueueAttributeNameQueueArn)],
		"maxReceiveCount":     3,
	})
	if err != nil {
		return "", err
	}

	output, err := client.CreateQueue(ctx, &awsv2sqs.CreateQueueInput{
		QueueName: aws.String(name),
		Attributes: map[string]string{
			string(types.QueueAttributeNameVisibilityTimeout):      "30",
			string(types.QueueAttributeNameMessageRetentionPeriod): "1209600",
			string(types.QueueAttributeNameRedrivePolicy):          string(redrive),
		},
	})
	if err != nil {
		return "", fmt.Errorf("create SQS queue %s: %w", name, err)
	}
	return aws.ToString(output.QueueUrl), nil
}

func queueURLEnv(name string) string {
	key := strings.ToUpper(strings.NewReplacer("-", "_").Replace(name)) + "_QUEUE_URL"
	return os.Getenv(key)
}
