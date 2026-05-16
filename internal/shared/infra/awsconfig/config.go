package awsconfig

import (
	"context"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// Load builds an AWS SDK config that also works with LocalStack.
func Load(ctx context.Context) (aws.Config, error) {
	region := firstNonEmpty(os.Getenv("AWS_REGION"), os.Getenv("AWS_DEFAULT_REGION"), "us-east-1")
	endpointURL := os.Getenv("AWS_ENDPOINT_URL")

	opts := []func(*config.LoadOptions) error{
		config.WithRegion(region),
	}

	if endpointURL != "" {
		opts = append(opts,
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
			config.WithBaseEndpoint(endpointURL),
		)
	}

	return config.LoadDefaultConfig(ctx, opts...)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
